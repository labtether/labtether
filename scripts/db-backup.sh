#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
BACKUP_DIR="${BACKUP_DIR:-${PROJECT_ROOT}/backups}"
KEEP_DAYS="${KEEP_DAYS:-7}"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/compose-db.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/db-backup.sh [--install-dir DIR | --compose]

Write a compressed SQL dump of the LabTether database to BACKUP_DIR
(default: ./backups).

Options:
  --install-dir DIR  Back up the release install in DIR, the folder that holds
                     docker-compose.deploy.yml and .env.deploy.
  --compose          Back up the Compose stack named by COMPOSE_FILE and
                     COMPOSE_ENV_FILE (default: this checkout's deploy stack).
  -h, --help         Show this help.

Without an option it connects with DATABASE_URL (from the environment or
ENV_FILE, default ./.env). When none is set and ./.env.deploy exists, it backs
up this checkout's deploy stack. In Compose mode pg_dump runs inside the
stack's postgres container, so Postgres needs no host port.

Environment: BACKUP_DIR, KEEP_DAYS (default 7), DATABASE_URL, ENV_FILE,
COMPOSE_FILE, COMPOSE_ENV_FILE.
USAGE
}

compose_mode=0
compose_file="${COMPOSE_FILE:-${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml}"
compose_env_file="${COMPOSE_ENV_FILE:-${PROJECT_ROOT}/.env.deploy}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      [[ $# -ge 2 && -n "$2" ]] || { usage >&2; exit 1; }
      compose_mode=1
      compose_file="${2%/}/docker-compose.deploy.yml"
      compose_env_file="${2%/}/.env.deploy"
      shift
      ;;
    --compose) compose_mode=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unexpected argument: $1" >&2; usage >&2; exit 1 ;;
  esac
  shift
done

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if ((compose_mode == 0)) && [[ -z "$database_url" && ( -e "$ENV_FILE" || -L "$ENV_FILE" ) ]]; then
  labtether_require_private_env_file "$ENV_FILE" || exit 1
  labtether_read_env_value database_url "$ENV_FILE" DATABASE_URL || exit 1
fi
# No connection string anywhere, but this checkout runs the deploy stack.
if ((compose_mode == 0)) && [[ -z "$database_url" && -f "$compose_env_file" ]]; then
  compose_mode=1
fi
database_url="${database_url:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

for command_name in cut date du find gzip mktemp tr wc; do
  require_command "$command_name" || exit 1
done
if ((compose_mode)); then
  unset database_url
  labtether_compose_db_prepare "$compose_file" "$compose_env_file" || exit 1
else
  require_command pg_dump || exit 1
  require_command python3 || exit 1
fi
if [[ ! "$KEEP_DAYS" =~ ^[0-9]+$ ]] || ((KEEP_DAYS > 36500)); then
  log_fail "KEEP_DAYS must be an integer between 0 and 36500"
  exit 1
fi

labtether_prepare_owned_output_dir "$BACKUP_DIR" "backup directory" || exit 1
chmod 700 "$BACKUP_DIR"

timestamp=$(date +%Y%m%d_%H%M%S)
temporary_backup=$(mktemp "${BACKUP_DIR}/.labtether-${timestamp}.XXXXXX")
backup_path="${BACKUP_DIR}/labtether_${timestamp}_${temporary_backup##*.}.sql.gz"
cleanup_partial() {
  [[ -z "$temporary_backup" ]] || rm -f -- "$temporary_backup"
}
trap cleanup_partial EXIT INT TERM HUP

log_info "Backing up database to ${backup_path}..."
if ((compose_mode)); then
  labtether_compose_db_dump | gzip >"$temporary_backup"
else
  python3 "${PROJECT_ROOT}/scripts/lib/db-client.py" pg_dump --no-password 3<<<"$database_url" | gzip >"$temporary_backup"
fi
chmod 600 "$temporary_backup"
mv "$temporary_backup" "$backup_path"
temporary_backup=""
unset database_url

# Keep retention scoped to regular LabTether backup files in the private
# backup directory; symlinks and unrelated files are never deleted.
pruned=$(find "$BACKUP_DIR" -type f -name 'labtether_*.sql.gz' -mtime +"$KEEP_DAYS" -print -delete 2>/dev/null | wc -l | tr -d ' ')
if [[ "$pruned" -gt 0 ]]; then
  log_info "Pruned ${pruned} backup(s) older than ${KEEP_DAYS} days."
fi

size=$(du -sh "$backup_path" | cut -f1)
log_pass "Backup complete: ${backup_path} (${size})"
# The dump holds saved credentials, webhook secrets and 2FA enrolments only in
# encrypted form. Without the Hub's encryption key a restore cannot read them.
log_warn "This file holds the database only. Also back up the Hub's encryption key, and keep it secret:"
log_warn "  /data/install/secrets.json in the Hub data volume (the encryption_key value), or"
log_warn "  LABTETHER_ENCRYPTION_KEY if you set it in your env file."
log_warn "Restore it together with this file; without it the restored secrets cannot be read."
