#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT_DIR}/.env}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/scripts/lib/script-common.sh"

if ! require_command psql; then
  echo "Error: psql is required for migrate-status. Install PostgreSQL client tools." >&2
  exit 1
fi

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "${ENV_FILE}"
  set +a
fi

DB_URL="${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

echo "Migration status (applied migrations):"
echo ""
psql "$DB_URL" --no-psqlrc -c "
SELECT
  version,
  name,
  applied_at AT TIME ZONE 'UTC' AS applied_at_utc,
  CASE WHEN checksum IS NOT NULL THEN 'yes' ELSE 'no (pre-checksum)' END AS checksum_verified
FROM schema_migrations
ORDER BY version ASC;
"
