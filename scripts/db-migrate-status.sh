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

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if [[ -z "$database_url" && ( -e "${ENV_FILE}" || -L "${ENV_FILE}" ) ]]; then
  labtether_require_private_env_file "${ENV_FILE}" || exit 1
  labtether_read_env_value database_url "${ENV_FILE}" DATABASE_URL || exit 1
fi
database_url="${database_url:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"

echo "Migration status (applied migrations):"
echo ""
PGDATABASE="$database_url" psql --no-psqlrc --set=ON_ERROR_STOP=on -c "
SELECT
  version,
  name,
  applied_at AT TIME ZONE 'UTC' AS applied_at_utc,
  CASE WHEN checksum IS NOT NULL THEN 'yes' ELSE 'no (pre-checksum)' END AS checksum_verified
FROM schema_migrations
ORDER BY version ASC;
"
unset database_url
