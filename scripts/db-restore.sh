#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
YES=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/compose-db.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/db-restore.sh [options] <backup-file.sql.gz|backup-file.sql>

Restore LabTether Postgres data from a backup file.

Options:
  --install-dir DIR  Restore into the release install in DIR, the folder that
                     holds docker-compose.deploy.yml and .env.deploy.
  --compose          Restore into the Compose stack named by COMPOSE_FILE and
                     COMPOSE_ENV_FILE (default: this checkout's deploy stack).
  --yes, -y          Skip confirmation prompt.
  -h, --help         Show this help.

In Compose mode the restore stops the Hub, replaces the database with an
empty one, loads the backup inside the postgres container and starts the Hub
again, so it works on a stack that already has data. Without an option it
loads into DATABASE_URL (from the environment or ENV_FILE, default ./.env),
which must be an empty database; when none is set and ./.env.deploy exists,
it restores this checkout's deploy stack.

Environment: DATABASE_URL, ENV_FILE, COMPOSE_FILE, COMPOSE_ENV_FILE.
USAGE
}

compose_mode=0
compose_file="${COMPOSE_FILE:-${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml}"
compose_env_file="${COMPOSE_ENV_FILE:-${PROJECT_ROOT}/.env.deploy}"
BACKUP_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      [[ $# -ge 2 && -n "$2" ]] || { usage >&2; exit 1; }
      compose_mode=1
      compose_file="${2%/}/docker-compose.deploy.yml"
      compose_env_file="${2%/}/.env.deploy"
      shift
      ;;
    --compose)
      compose_mode=1
      ;;
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

if [[ "${BACKUP_FILE}" == *.gz ]]; then
  if ! command -v gzip >/dev/null 2>&1; then
    echo "gzip is required to restore .gz backups" >&2
    exit 1
  fi
fi

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if ((compose_mode == 0)) && [[ -z "$database_url" && ( -e "${ENV_FILE}" || -L "${ENV_FILE}" ) ]]; then
  labtether_require_private_env_file "${ENV_FILE}" || exit 1
  labtether_read_env_value database_url "${ENV_FILE}" DATABASE_URL || exit 1
fi
# No connection string anywhere, but this checkout runs the deploy stack.
if ((compose_mode == 0)) && [[ -z "$database_url" && -f "$compose_env_file" ]]; then
  compose_mode=1
fi
database_url="${database_url:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

if ((compose_mode)); then
  unset database_url
  labtether_compose_db_prepare "$compose_file" "$compose_env_file" || exit 1
  echo "Target: the database of the Compose stack in $(dirname "$compose_file")"
else
  if ! command -v psql >/dev/null 2>&1; then
    echo "psql is required for restore" >&2
    exit 1
  fi
  require_command python3 || exit 1
  echo "Target database: configured DATABASE_URL (credentials hidden)"
fi
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
if ((compose_mode)); then
  if [[ "${BACKUP_FILE}" == *.gz ]]; then
    gzip -dc "${BACKUP_FILE}" | labtether_compose_db_restore
  else
    labtether_compose_db_restore < "${BACKUP_FILE}"
  fi
elif [[ "${BACKUP_FILE}" == *.gz ]]; then
  gzip -dc "${BACKUP_FILE}" | python3 "${PROJECT_ROOT}/scripts/lib/db-client.py" psql --no-password --no-psqlrc --set=ON_ERROR_STOP=on 3<<<"$database_url"
else
  python3 "${PROJECT_ROOT}/scripts/lib/db-client.py" psql --no-password --no-psqlrc --set=ON_ERROR_STOP=on 3<<<"$database_url" < "${BACKUP_FILE}"
fi
unset database_url

echo "Restore complete."
