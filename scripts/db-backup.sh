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

for command_name in cut date du find gzip mktemp pg_dump tr wc; do
  require_command "$command_name" || exit 1
done
if [[ ! "$KEEP_DAYS" =~ ^[0-9]+$ ]] || ((KEEP_DAYS > 36500)); then
  log_fail "KEEP_DAYS must be an integer between 0 and 36500"
  exit 1
fi

labtether_prepare_owned_output_dir "$BACKUP_DIR" "backup directory" || exit 1
chmod 700 "$BACKUP_DIR"

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if [[ -z "$database_url" && ( -e "$ENV_FILE" || -L "$ENV_FILE" ) ]]; then
  labtether_require_private_env_file "$ENV_FILE" || exit 1
  labtether_read_env_value database_url "$ENV_FILE" DATABASE_URL || exit 1
fi
database_url="${database_url:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

timestamp=$(date +%Y%m%d_%H%M%S)
temporary_backup=$(mktemp "${BACKUP_DIR}/.labtether-${timestamp}.XXXXXX")
backup_path="${BACKUP_DIR}/labtether_${timestamp}_${temporary_backup##*.}.sql.gz"
cleanup_partial() {
  [[ -z "$temporary_backup" ]] || rm -f -- "$temporary_backup"
}
trap cleanup_partial EXIT INT TERM HUP

log_info "Backing up database to ${backup_path}..."
PGDATABASE="$database_url" pg_dump --no-password | gzip >"$temporary_backup"
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
