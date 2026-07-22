#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT_DIR}/.env}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/scripts/lib/script-common.sh"

if ! require_command go; then
  exit 1
fi

database_url="${DATABASE_URL-}"
export -n database_url 2>/dev/null || true
unset DATABASE_URL
if [[ -z "$database_url" && ( -e "${ENV_FILE}" || -L "${ENV_FILE}" ) ]]; then
  labtether_require_private_env_file "${ENV_FILE}" || exit 1
  labtether_read_env_value database_url "${ENV_FILE}" DATABASE_URL || exit 1
fi
if [[ -n "$database_url" ]]; then
  export DATABASE_URL="$database_url"
fi
unset database_url

cd "${ROOT_DIR}"
go run ./services/migrator
