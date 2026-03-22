#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups}"
KEEP_DAYS="${KEEP_DAYS:-7}"
mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
FILENAME="labtether_${TIMESTAMP}.sql.gz"

# Load .env for DATABASE_URL if available
# shellcheck disable=SC1091
[ -f .env ] && set -a && source .env && set +a

DB_URL="${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

echo "Backing up database to ${BACKUP_DIR}/${FILENAME}..."
pg_dump "$DB_URL" | gzip > "${BACKUP_DIR}/${FILENAME}"

# Prune old backups
PRUNED=$(find "$BACKUP_DIR" -name "labtether_*.sql.gz" -mtime +"$KEEP_DAYS" -print -delete 2>/dev/null | wc -l | tr -d ' ')
if [ "$PRUNED" -gt 0 ]; then
    echo "Pruned ${PRUNED} backup(s) older than ${KEEP_DAYS} days."
fi

SIZE=$(du -sh "${BACKUP_DIR}/${FILENAME}" | cut -f1)
echo "Backup complete: ${BACKUP_DIR}/${FILENAME} (${SIZE})"
