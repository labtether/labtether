#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
ENV_TEMPLATE="${ENV_TEMPLATE:-${PROJECT_ROOT}/.env.example}"

FAST_MODE=0
SKIP_SMOKE=0
SKIP_DOCTOR=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/bootstrap.sh [options]

One-command LabTether bootstrap for first-time local installs:
1) creates .env from .env.example when missing,
2) generates secure required secrets when placeholders are present,
3) runs migrations + compose startup,
4) runs smoke test + setup doctor checks (unless skipped).

Options:
  --fast           Use existing images (make compose-up-fast) instead of rebuilding.
  --skip-smoke     Skip smoke validation.
  --skip-doctor    Skip setup doctor run.
  -h, --help       Show this help.

Environment:
  ENV_FILE         Path to .env (default: ./\.env)
  ENV_TEMPLATE     Path to env template (default: ./\.env.example)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --fast)
      FAST_MODE=1
      ;;
    --skip-smoke)
      SKIP_SMOKE=1
      ;;
    --skip-doctor)
      SKIP_DOCTOR=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log_fail "unknown option: $1"
      usage
      exit 1
      ;;
  esac
  shift
done

if ! require_command make; then
  exit 1
fi
if ! require_command openssl; then
  log_info "Install OpenSSL to generate secure bootstrap values."
  exit 1
fi

ensure_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    return 0
  fi
  if [[ ! -f "${ENV_TEMPLATE}" ]]; then
    log_fail "missing env template: ${ENV_TEMPLATE}"
    return 1
  fi
  cp "${ENV_TEMPLATE}" "${ENV_FILE}"
  chmod 600 "${ENV_FILE}"
  log_pass "created ${ENV_FILE} from ${ENV_TEMPLATE}"
}

read_env_value() {
  local key=$1
  local line
  line=$(grep -E "^${key}=" "${ENV_FILE}" | head -n 1 || true)
  printf '%s' "${line#*=}"
}

upsert_env_value() {
  local key=$1
  local value=$2
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v k="$key" -v v="$value" -F= '
    BEGIN { written = 0 }
    $1 == k { print k "=" v; written = 1; next }
    { print }
    END { if (!written) print k "=" v }
  ' "${ENV_FILE}" > "${tmp_file}"
  mv "${tmp_file}" "${ENV_FILE}"
  chmod 600 "${ENV_FILE}"
}

is_placeholder() {
  local value=$1
  [[ -z "${value// }" || "${value}" == REPLACE_WITH_* ]]
}

generated_keys=()

set_or_generate() {
  local key=$1
  local generator=$2
  local current
  current="$(read_env_value "${key}")"
  if is_placeholder "${current}"; then
    local next
    next="$(${generator})"
    upsert_env_value "${key}" "${next}"
    generated_keys+=("${key}")
  fi
}

generate_hex_token() {
  openssl rand -hex 32
}

generate_password() {
  openssl rand -base64 24 | tr -d '\n' | tr '+/' '-_'
}

generate_base64_key() {
  openssl rand -base64 32 | tr -d '\n'
}

refresh_database_url_if_placeholder() {
  local db_url
  db_url="$(read_env_value "DATABASE_URL")"
  if ! is_placeholder "${db_url}" && [[ "${db_url}" != *"REPLACE_WITH_STRONG_POSTGRES_PASSWORD"* ]]; then
    return 0
  fi

  local user pass db
  user="$(read_env_value "POSTGRES_USER")"
  pass="$(read_env_value "POSTGRES_PASSWORD")"
  db="$(read_env_value "POSTGRES_DB")"
  [[ -z "${user}" ]] && user="labtether"
  [[ -z "${db}" ]] && db="labtether"

  upsert_env_value "DATABASE_URL" "postgres://${user}:${pass}@localhost:5432/${db}?sslmode=disable"
  generated_keys+=("DATABASE_URL")
}

default_if_empty() {
  local key=$1
  local fallback=$2
  local current
  current="$(read_env_value "${key}")"
  if [[ -z "${current// }" ]]; then
    upsert_env_value "${key}" "${fallback}"
    generated_keys+=("${key}")
  fi
}

ensure_env_file

default_if_empty "POSTGRES_USER" "labtether"
default_if_empty "POSTGRES_DB" "labtether"
set_or_generate "POSTGRES_PASSWORD" generate_password
set_or_generate "LABTETHER_OWNER_TOKEN" generate_hex_token

api_token_value="$(read_env_value "LABTETHER_API_TOKEN")"
if is_placeholder "${api_token_value}"; then
  upsert_env_value "LABTETHER_API_TOKEN" "$(generate_hex_token)"
  generated_keys+=("LABTETHER_API_TOKEN")
fi

set_or_generate "LABTETHER_ENCRYPTION_KEY" generate_base64_key
set_or_generate "LABTETHER_ADMIN_PASSWORD" generate_password
refresh_database_url_if_placeholder

log_info "Prepared ${ENV_FILE}"
if [[ ${#generated_keys[@]} -gt 0 ]]; then
  log_pass "generated secure values for: ${generated_keys[*]}"
else
  log_pass "existing secure values detected; no secret regeneration needed"
fi

cd "${PROJECT_ROOT}"

log_info "Running migrations..."
make db-migrate

if [[ "${FAST_MODE}" -eq 1 ]]; then
  log_info "Starting stack (fast mode)..."
  make compose-up-fast
else
  log_info "Starting stack..."
  make compose-up
fi

if [[ "${SKIP_SMOKE}" -eq 0 ]]; then
  log_info "Running smoke validation..."
  ./scripts/smoke-test.sh --skip-compose --no-build
else
  log_info "Skipping smoke validation (--skip-smoke)."
fi

if [[ "${SKIP_DOCTOR}" -eq 0 ]]; then
  log_info "Running setup doctor..."
  ./scripts/setup-doctor.sh
else
  log_info "Skipping setup doctor (--skip-doctor)."
fi

admin_password="$(read_env_value "LABTETHER_ADMIN_PASSWORD")"

cat <<EOF

LabTether bootstrap complete.

- Console: https://localhost:3000
- API health: http://localhost:8080/healthz
- Default user: admin
- Admin password: ${admin_password}

If the browser warns about a self-signed cert, install the hub CA:
https://localhost:8443/api/v1/tls/ca.crt
EOF
