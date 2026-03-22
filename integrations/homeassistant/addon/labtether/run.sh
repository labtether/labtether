#!/usr/bin/env bash
set -Eeuo pipefail

readonly OPTIONS_FILE="/data/options.json"
readonly STATE_DIR="/data/labtether-addon"
readonly STATE_ENV_FILE="${STATE_DIR}/runtime.env"
readonly GENERATED_FILE="${STATE_DIR}/generated-credentials.txt"

mkdir -p "${STATE_DIR}"
chmod 700 "${STATE_DIR}"

log() {
  echo "[labtether-addon] $*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

load_state_env() {
  if [[ -f "${STATE_ENV_FILE}" ]]; then
    # shellcheck disable=SC1090
    source "${STATE_ENV_FILE}"
  fi
}

persist_state_value() {
  local key="$1"
  local value="$2"
  local tmp
  tmp="$(mktemp)"
  if [[ -f "${STATE_ENV_FILE}" ]]; then
    grep -v "^${key}=" "${STATE_ENV_FILE}" > "${tmp}" || true
  fi
  printf '%s=%q\n' "${key}" "${value}" >> "${tmp}"
  mv "${tmp}" "${STATE_ENV_FILE}"
  chmod 600 "${STATE_ENV_FILE}"
}

read_option_string() {
  local key="$1"
  jq -r --arg key "${key}" '.[$key] // ""' "${OPTIONS_FILE}"
}

read_option_bool() {
  local key="$1"
  local fallback="$2"
  if [[ ! -f "${OPTIONS_FILE}" ]]; then
    echo "${fallback}"
    return
  fi
  jq -r --arg key "${key}" --arg fallback "${fallback}" '
    if has($key) then
      if .[$key] == true then "true" else "false" end
    else
      $fallback
    end
  ' "${OPTIONS_FILE}"
}

generate_hex_token() {
  od -An -tx1 -N32 /dev/urandom | tr -d ' \n'
}

generate_base64_key() {
  head -c 32 /dev/urandom | base64 | tr -d '\n'
}

generate_password() {
  LC_ALL=C tr -dc 'A-Za-z0-9@#%+_-' < /dev/urandom | head -c 24
}

require_options_file() {
  if [[ ! -f "${OPTIONS_FILE}" ]]; then
    fail "missing ${OPTIONS_FILE}; install via Home Assistant add-on options"
  fi
}

require_or_generate() {
  local key="$1"
  local option_value="$2"
  local existing_value="$3"
  local auto_generate="$4"
  local generator="$5"

  local value
  value="${option_value}"
  if [[ -z "${value}" ]]; then
    value="${existing_value}"
  fi

  if [[ -z "${value}" ]]; then
    if [[ "${auto_generate}" != "true" ]]; then
      fail "${key} is required when auto_generate_credentials=false"
    fi
    value="$(${generator})"
    GENERATED_KEYS+=("${key}")
  fi

  persist_state_value "${key}" "${value}"
  printf '%s' "${value}"
}

start_local_postgres() {
  local pgdata="/data/postgres"
  mkdir -p "${pgdata}"
  chown -R postgres:postgres "${pgdata}"

  if [[ ! -f "${pgdata}/PG_VERSION" ]]; then
    log "initializing local postgres data directory"
    su-exec postgres initdb -D "${pgdata}" -U labtether --auth=trust >/dev/null
    echo "listen_addresses = '127.0.0.1'" >> "${pgdata}/postgresql.conf"
    echo "port = 5432" >> "${pgdata}/postgresql.conf"
  fi

  log "starting local postgres"
  su-exec postgres postgres -D "${pgdata}" -h 127.0.0.1 -p 5432 >/tmp/labtether-addon-postgres.log 2>&1 &
  PG_PID=$!

  local tries=0
  until pg_isready -h 127.0.0.1 -p 5432 -U labtether >/dev/null 2>&1; do
    tries=$((tries + 1))
    if (( tries > 40 )); then
      fail "local postgres did not become ready"
    fi
    sleep 1
  done

  su-exec postgres createdb -h 127.0.0.1 -p 5432 -U labtether labtether >/dev/null 2>&1 || true
}

write_generated_summary() {
  if (( ${#GENERATED_KEYS[@]} == 0 )); then
    return
  fi
  {
    echo "Generated on: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo ""
    for key in "${GENERATED_KEYS[@]}"; do
      echo "${key}=${!key}"
    done
    echo ""
    echo "Store these values securely."
  } > "${GENERATED_FILE}"
  chmod 600 "${GENERATED_FILE}"
  log "generated credentials saved to ${GENERATED_FILE}"
}

cleanup() {
  if [[ -n "${PG_PID:-}" ]] && kill -0 "${PG_PID}" >/dev/null 2>&1; then
    kill "${PG_PID}" >/dev/null 2>&1 || true
    wait "${PG_PID}" || true
  fi
}

trap cleanup EXIT

require_options_file
load_state_env

AUTO_GENERATE="$(read_option_bool "auto_generate_credentials" "true")"
OWNER_TOKEN_OPT="$(read_option_string "labtether_owner_token")"
ADMIN_PASSWORD_OPT="$(read_option_string "labtether_admin_password")"
ENCRYPTION_KEY_OPT="$(read_option_string "encryption_key")"
DATABASE_URL_OPT="$(read_option_string "database_url")"
TLS_MODE_OPT="$(read_option_string "tls_mode")"

GENERATED_KEYS=()

LABTETHER_OWNER_TOKEN="$(require_or_generate "LABTETHER_OWNER_TOKEN" "${OWNER_TOKEN_OPT}" "${LABTETHER_OWNER_TOKEN:-}" "${AUTO_GENERATE}" generate_hex_token)"
LABTETHER_ADMIN_PASSWORD="$(require_or_generate "LABTETHER_ADMIN_PASSWORD" "${ADMIN_PASSWORD_OPT}" "${LABTETHER_ADMIN_PASSWORD:-}" "${AUTO_GENERATE}" generate_password)"
LABTETHER_ENCRYPTION_KEY="$(require_or_generate "LABTETHER_ENCRYPTION_KEY" "${ENCRYPTION_KEY_OPT}" "${LABTETHER_ENCRYPTION_KEY:-}" "${AUTO_GENERATE}" generate_base64_key)"

if [[ -n "${DATABASE_URL_OPT}" ]]; then
  DATABASE_URL="${DATABASE_URL_OPT}"
  persist_state_value "DATABASE_URL" "${DATABASE_URL}"
else
  DATABASE_URL="${DATABASE_URL:-}"
fi

if [[ -z "${DATABASE_URL}" ]]; then
  start_local_postgres
  DATABASE_URL="postgres://labtether@127.0.0.1:5432/labtether?sslmode=disable"
  persist_state_value "DATABASE_URL" "${DATABASE_URL}"
fi

if [[ -z "${TLS_MODE_OPT}" ]]; then
  TLS_MODE_OPT="auto"
fi

# Export runtime env expected by LabTether hub.
export API_PORT=8080
export LABTETHER_HTTP_PORT=8080
export LABTETHER_HTTPS_PORT=8443
export LABTETHER_TLS_MODE="${TLS_MODE_OPT}"
export LABTETHER_DATA_DIR=/data
export LABTETHER_ENV=production
export LABTETHER_OWNER_TOKEN
export LABTETHER_API_TOKEN="${LABTETHER_OWNER_TOKEN}"
export LABTETHER_ADMIN_PASSWORD
export LABTETHER_ENCRYPTION_KEY
export DATABASE_URL

# Ensure base64 key decodes to 32 bytes.
if ! decoded_len=$(printf '%s' "${LABTETHER_ENCRYPTION_KEY}" | base64 -d 2>/dev/null | wc -c | tr -d ' '); then
  fail "LABTETHER_ENCRYPTION_KEY must be valid base64"
fi
if [[ "${decoded_len}" != "32" ]]; then
  fail "LABTETHER_ENCRYPTION_KEY must decode to 32 bytes"
fi

write_generated_summary

log "starting LabTether hub (tls_mode=${LABTETHER_TLS_MODE})"
exec /usr/local/bin/labtether
