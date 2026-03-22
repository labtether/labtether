#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
YES=0

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

if [[ ! -f "${BACKUP_FILE}" ]]; then
  echo "backup file not found: ${BACKUP_FILE}" >&2
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

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "${ENV_FILE}"
  set +a
fi

DB_URL="${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

echo "Target database: ${DB_URL}"
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
  gzip -dc "${BACKUP_FILE}" | psql "${DB_URL}"
else
  psql "${DB_URL}" < "${BACKUP_FILE}"
fi

echo "Restore complete."
