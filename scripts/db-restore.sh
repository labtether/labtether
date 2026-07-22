#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
YES=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/db-restore.sh [options] <backup-file.sql.gz|backup-file.sql>

Restore LabTether Postgres data from a backup file.

Options:
  --yes, -y      Skip confirmation prompt.
  -h, --help     Show this help.

Environment:
  ENV_FILE       Path to .env used to resolve DATABASE_URL (default: ./\.env)
USAGE
}

BACKUP_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes|-y)
      YES=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      if [[ -z "${BACKUP_FILE}" ]]; then
        BACKUP_FILE="$1"
      else
        echo "unexpected argument: $1" >&2
        usage
        exit 1
      fi
      ;;
  esac
  shift
done

if [[ -z "${BACKUP_FILE}" ]]; then
  echo "missing backup file argument" >&2
  usage
  exit 1
fi

if [[ ! -f "${BACKUP_FILE}" || -L "${BACKUP_FILE}" ]]; then
  echo "backup file must be a non-symlink regular file: ${BACKUP_FILE}" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql is required for restore" >&2
  exit 1
fi

if [[ "${BACKUP_FILE}" == *.gz ]]; then
  if ! command -v gzip >/dev/null 2>&1; then
    echo "gzip is required to restore .gz backups" >&2
    exit 1
  fi
fi

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if [[ -z "$database_url" && ( -e "${ENV_FILE}" || -L "${ENV_FILE}" ) ]]; then
  labtether_require_private_env_file "${ENV_FILE}" || exit 1
  labtether_read_env_value database_url "${ENV_FILE}" DATABASE_URL || exit 1
fi
database_url="${database_url:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

echo "Target database: configured DATABASE_URL (credentials hidden)"
echo "Backup file: ${BACKUP_FILE}"
echo ""
echo "WARNING: this will overwrite data in the target database."

if [[ "${YES}" -ne 1 ]]; then
  read -r -p "Continue restore? (yes/no): " confirm
  if [[ "${confirm}" != "yes" ]]; then
    echo "Restore cancelled."
    exit 1
  fi
fi

echo "Restoring..."
if [[ "${BACKUP_FILE}" == *.gz ]]; then
  gzip -dc "${BACKUP_FILE}" | PGDATABASE="$database_url" psql --no-psqlrc --set=ON_ERROR_STOP=on
else
  PGDATABASE="$database_url" psql --no-psqlrc --set=ON_ERROR_STOP=on < "${BACKUP_FILE}"
fi
unset database_url

echo "Restore complete."
