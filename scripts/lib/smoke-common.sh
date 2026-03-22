#!/usr/bin/env bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/smoke-test.sh [options]

Options:
  --keep            Leave docker compose stack running after tests
  --skip-compose    Do not run docker compose up/down (assumes services already running)
  --no-build        Skip docker build when bringing stack up
  --verbose         Print response bodies for easier debugging
  --timeout <secs>  Service wait timeout in seconds (default: 120)
  -h, --help       Show this help

Environment:
  ENV_FILE=/path/to/.env
  COMPOSE_FILE=/path/to/docker-compose.yml
  TIMEOUT_SECONDS=120
USAGE
}

log() {
  log_info "$*"
}

assert_equal() {
  local label=$1
  local expected=$2
  local actual=$3
  if [[ "$expected" == "$actual" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] %s (%s)\n' "$label" "$actual"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] %s expected=%s actual=%s\n' "$label" "$expected" "$actual"
  fi
}

assert_not_equal() {
  local label=$1
  local forbidden=$2
  local actual=$3
  if [[ "$forbidden" != "$actual" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] %s (%s)\n' "$label" "$actual"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] %s should not be %s\n' "$label" "$forbidden"
  fi
}

contains() {
  local needle=$1
  local haystack=$2
  [[ "$haystack" == *"$needle"* ]]
}

extract_json_string() {
  local key=$1
  local json=$2
  printf '%s' "$json" | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" | head -n 1
}

extract_nested_json_id() {
  local key=$1
  local json=$2
  printf '%s' "$json" | sed -n "s/.*\"${key}\"[^{]*{[^}]*\"id\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" | head -n 1
}

wait_for_http() {
  local label=$1
  local url=$2
  local timeout=${3:-$TIMEOUT_SECONDS}
  local delay=2
  local deadline=$((SECONDS + timeout))

  log "Waiting for $label at $url"

  while [[ $SECONDS -lt $deadline ]]; do
    if check_http_status "$url" 200 2; then
      log "  available: $label"
      return 0
    fi
    sleep "$delay"
  done

  log "  timeout waiting for $label"
  return 1
}

run_request() {
  local __body_var=$1
  local __status_var=$2
  local method=$3
  local url=$4
  local payload=${5:-}
  local with_auth=${6:-1}

  local -a args=("-sS" "-w" $'\n%{http_code}' -X "$method" "$url")
  if [[ "${url}" == https://* ]]; then
    # Local dev commonly uses self-signed hub certs.
    args+=("-k")
  fi
  if [[ "$with_auth" == "1" ]]; then
    args+=(-H "Authorization: Bearer ${AUTH_TOKEN}")
  fi
  if [[ -n "$payload" ]]; then
    args+=(-H "Content-Type: application/json" --data "$payload")
  fi

  local response
  response=$(curl "${args[@]}" || true)

  local parsed_status
  parsed_status=$(printf '%s' "$response" | tail -n 1)
  local parsed_body
  parsed_body=$(printf '%s' "$response" | sed '$d')

  printf -v "$__body_var" '%s' "$parsed_body"
  printf -v "$__status_var" '%s' "$parsed_status"
}

smoke_cleanup_tasks=()

smoke_init_cleanup() {
  smoke_cleanup_tasks=()
}

smoke_register_cleanup() {
  local method=$1
  local path=$2
  local label=$3

  if [[ -z "$method" || -z "$path" ]]; then
    return 0
  fi

  smoke_cleanup_tasks+=("${method}|${path}|${label:-$path}")
}

smoke_run_cleanup() {
  local index
  local cleanup_entry
  local method
  local path
  local label
  local body
  local status

  for ((index=${#smoke_cleanup_tasks[@]} - 1; index >= 0; index--)); do
    cleanup_entry=${smoke_cleanup_tasks[$index]}
    IFS='|' read -r method path label <<< "$cleanup_entry"
    run_request body status "$method" "$API_BASE${path}" "" 1 || true
    status=${status:-000}
    if [[ "$status" == "200" || "$status" == "204" || "$status" == "404" ]]; then
      continue
    fi
    log "  [CLEANUP] ${label} cleanup failed with status ${status}"
  done

  smoke_cleanup_tasks=()
}
