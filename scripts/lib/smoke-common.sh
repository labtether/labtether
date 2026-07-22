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
  --allow-mutations Permit fixture writes with --skip-compose
  --allow-outbound  Enable network probes (also requires all explicit targets)
  --allow-remote-exec --target-asset <id>
                    Permit commands against one explicit real asset
  --allow-global-settings
                    Permit reversible runtime-setting coverage
  --allow-destructive-retention --ephemeral-stack
                    Permit retention mutation only in an isolated compose project
  --reuse-compose-stack
                    Explicitly reuse the normal compose project/volumes (unsafe for isolation)
  --synthetic-target <url> --webhook-target <url> --ssh-host <host>
                    Explicit outbound destinations; there are no live defaults
  --ca-file <path>  Verify HTTPS with this CA bundle
  --insecure-tls    Rejected for authenticated smoke runs; use setup-doctor for diagnostics
  -h, --help       Show this help

Environment:
  ENV_FILE=/path/to/.env
  COMPOSE_FILE=/path/to/docker-compose.yml
  TIMEOUT_SECONDS=120
  LABTETHER_CA_FILE=/path/to/ca.crt
  LABTETHER_INSECURE_TLS=1  # rejected by authenticated smoke scripts
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

smoke_print_verbose_json() {
  local label=$1
  local json=$2
  [[ "${VERBOSE:-0}" == "1" ]] || return 0
  printf '%s: %s\n' "$label" "$(labtether_redact_json_for_log "$json")"
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

  local -a args=("-sS" "--connect-timeout" "${SMOKE_CONNECT_TIMEOUT_SECONDS:-5}" "--max-time" "${TIMEOUT_SECONDS:-120}" "-w" $'\n%{http_code}' -X "$method" "$url")
  if ! labtether_build_curl_request_args "$url" "$with_auth"; then
    printf -v "$__body_var" '%s' ''
    printf -v "$__status_var" '%s' '000'
    return 1
  fi
  args=("${LABTETHER_CURL_REQUEST_ARGS[@]}" "${args[@]}")
  if [[ -n "$payload" ]]; then
    args+=(-H "Content-Type: application/json" --data-binary @-)
  fi

  local response
  if [[ -n "$payload" ]]; then
    if ! response=$(labtether_curl "${args[@]}" <<<"$payload"); then
      printf -v "$__body_var" '%s' "$response"
      printf -v "$__status_var" '%s' '000'
      return 1
    fi
  elif ! response=$(labtether_curl "${args[@]}"); then
    printf -v "$__body_var" '%s' "$response"
    printf -v "$__status_var" '%s' '000'
    return 1
  fi

  local parsed_status
  parsed_status=$(printf '%s' "$response" | tail -n 1)
  local parsed_body
  parsed_body=$(printf '%s' "$response" | sed '$d')

  printf -v "$__body_var" '%s' "$parsed_body"
  printf -v "$__status_var" '%s' "$parsed_status"
}

smoke_cleanup_tasks=()
SMOKE_CLEANUP_FAILURES=0

smoke_init_cleanup() {
  smoke_cleanup_tasks=()
  SMOKE_CLEANUP_FAILURES=0
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
  # run_request writes through the variable name passed as its first argument.
  # shellcheck disable=SC2034
  local body
  local status

  for ((index=${#smoke_cleanup_tasks[@]} - 1; index >= 0; index--)); do
    cleanup_entry=${smoke_cleanup_tasks[$index]}
    IFS='|' read -r method path label <<< "$cleanup_entry"
    if ! run_request body status "$method" "$API_BASE${path}" "" 1; then
      status=${status:-000}
    fi
    status=${status:-000}
    if [[ "$status" == "200" || "$status" == "202" || "$status" == "204" || "$status" == "404" ]]; then
      continue
    fi
    log "  [CLEANUP] ${label} cleanup failed with status ${status}"
    SMOKE_CLEANUP_FAILURES=$((SMOKE_CLEANUP_FAILURES + 1))
  done

  smoke_cleanup_tasks=()
  [[ "$SMOKE_CLEANUP_FAILURES" -eq 0 ]]
}
