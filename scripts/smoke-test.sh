#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_API_BASE_URL EARLY_AGENT_BASE_URL EARLY_CA_FILE EARLY_INSECURE_TLS AUTH_TOKEN
EARLY_API_TOKEN="${LABTETHER_API_TOKEN-}"
EARLY_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"
EARLY_API_BASE_URL="${LABTETHER_API_BASE_URL-}"
EARLY_AGENT_BASE_URL="${LABTETHER_AGENT_BASE_URL-}"
EARLY_CA_FILE="${LABTETHER_CA_FILE-}"
EARLY_INSECURE_TLS="${LABTETHER_INSECURE_TLS-}"
unset LABTETHER_API_TOKEN LABTETHER_OWNER_TOKEN

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-${PROJECT_ROOT}/docker-compose.yml}"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-120}"
KEEP_UP=0
SKIP_COMPOSE=0
SKIP_BUILD=0
VERBOSE=0
ALLOW_MUTATIONS="${LABTETHER_SMOKE_ALLOW_MUTATIONS:-0}"
ALLOW_OUTBOUND="${LABTETHER_SMOKE_ALLOW_OUTBOUND:-0}"
ALLOW_REMOTE_EXEC="${LABTETHER_SMOKE_ALLOW_REMOTE_EXEC:-0}"
ALLOW_GLOBAL_SETTINGS="${LABTETHER_SMOKE_ALLOW_GLOBAL_SETTINGS:-0}"
ALLOW_DESTRUCTIVE_RETENTION="${LABTETHER_SMOKE_ALLOW_DESTRUCTIVE_RETENTION:-0}"
EPHEMERAL_STACK="${LABTETHER_SMOKE_EPHEMERAL_STACK:-1}"
REMOTE_EXEC_TARGET="${LABTETHER_SMOKE_TARGET_ASSET:-}"
SYNTHETIC_TARGET="${LABTETHER_SMOKE_SYNTHETIC_TARGET:-}"
WEBHOOK_TARGET="${LABTETHER_SMOKE_WEBHOOK_TARGET:-}"
SSH_TARGET="${LABTETHER_SMOKE_SSH_TARGET:-}"
CLI_CA_FILE=""
CLI_INSECURE_TLS=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/smoke-common.sh"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep)
      KEEP_UP=1
      ;;
    --skip-compose)
      SKIP_COMPOSE=1
      ;;
    --no-build)
      SKIP_BUILD=1
      ;;
    --verbose)
      VERBOSE=1
      ;;
    --allow-mutations)
      ALLOW_MUTATIONS=1
      ;;
    --allow-outbound)
      ALLOW_OUTBOUND=1
      ;;
    --allow-remote-exec)
      ALLOW_REMOTE_EXEC=1
      ;;
    --allow-global-settings)
      ALLOW_GLOBAL_SETTINGS=1
      ;;
    --allow-destructive-retention)
      ALLOW_DESTRUCTIVE_RETENTION=1
      ;;
    --ephemeral-stack)
      EPHEMERAL_STACK=1
      ;;
    --reuse-compose-stack)
      EPHEMERAL_STACK=0
      ;;
    --target-asset)
      REMOTE_EXEC_TARGET="${2:-}"
      shift
      ;;
    --synthetic-target)
      SYNTHETIC_TARGET="${2:-}"
      shift
      ;;
    --webhook-target)
      WEBHOOK_TARGET="${2:-}"
      shift
      ;;
    --ssh-host)
      SSH_TARGET="${2:-}"
      shift
      ;;
    --ca-file)
      CLI_CA_FILE="${2:-}"
      LABTETHER_CA_FILE="$CLI_CA_FILE"
      shift
      ;;
    --insecure-tls)
      CLI_INSECURE_TLS=1
      LABTETHER_INSECURE_TLS=1
      ;;
    --timeout)
      TIMEOUT_SECONDS="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

# Read dynamically by smoke_print_verbose_json from the sourced helper.
: "$VERBOSE"

PASS_COUNT=0
FAIL_COUNT=0

SMOKE_RUN_TOKEN="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
if [[ -z "$SMOKE_RUN_TOKEN" ]]; then
  echo "Failed to generate smoke run identifier" >&2
  exit 1
fi
SMOKE_NODE_ID="smoke-node-${SMOKE_RUN_TOKEN}"
SMOKE_COMMAND_TARGET_ID="smoke-unroutable-${SMOKE_RUN_TOKEN}"
SMOKE_GROUP_NODE_ID="smoke-group-node-${SMOKE_RUN_TOKEN}"
SMOKE_DEP_SRC_ID="smoke-dep-src-${SMOKE_RUN_TOKEN}"
SMOKE_DEP_TGT_ID="smoke-dep-tgt-${SMOKE_RUN_TOKEN}"
SMOKE_GROUP_NAME="Smoke Group ${SMOKE_RUN_TOKEN}"
SMOKE_BACKUP_GROUP_NAME="Smoke Backup Group ${SMOKE_RUN_TOKEN}"
retention_restore_payload=""
retention_settings_mutated=0
runtime_restore_payload=""
runtime_restore_mode=""
runtime_settings_mutated=0
cleanup_running=0
SMOKE_COMPOSE_PROJECT="labtether-smoke-${SMOKE_RUN_TOKEN}"

if labtether_value_is_true "$ALLOW_OUTBOUND"; then
  if [[ -z "$SYNTHETIC_TARGET" || -z "$WEBHOOK_TARGET" || -z "$SSH_TARGET" ]]; then
    echo "--allow-outbound requires --synthetic-target, --webhook-target, and --ssh-host" >&2
    exit 1
  fi
fi
if labtether_value_is_true "$ALLOW_REMOTE_EXEC" && [[ -z "$REMOTE_EXEC_TARGET" ]]; then
  echo "--allow-remote-exec requires --target-asset" >&2
  exit 1
fi
if labtether_value_is_true "$ALLOW_DESTRUCTIVE_RETENTION"; then
  if ! labtether_value_is_true "$ALLOW_GLOBAL_SETTINGS" || ! labtether_value_is_true "$EPHEMERAL_STACK" || [[ "$SKIP_COMPOSE" == "1" ]]; then
    echo "destructive retention coverage requires --allow-global-settings --ephemeral-stack and a script-managed compose stack" >&2
    exit 1
  fi
fi
labtether_validate_tls_options || exit 1

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing env file: $ENV_FILE" >&2
  exit 1
fi
labtether_require_private_env_file "$ENV_FILE" || exit 1

unset SOURCED_API_TOKEN SOURCED_OWNER_TOKEN SOURCED_API_BASE_URL SOURCED_AGENT_BASE_URL SOURCED_CA_FILE SOURCED_INSECURE_TLS AUTH_TOKEN
labtether_read_env_value SOURCED_API_TOKEN "$ENV_FILE" LABTETHER_API_TOKEN || exit 1
labtether_read_env_value SOURCED_OWNER_TOKEN "$ENV_FILE" LABTETHER_OWNER_TOKEN || exit 1
labtether_read_env_value SOURCED_API_BASE_URL "$ENV_FILE" LABTETHER_API_BASE_URL || exit 1
labtether_read_env_value SOURCED_AGENT_BASE_URL "$ENV_FILE" LABTETHER_AGENT_BASE_URL || exit 1
labtether_read_env_value SOURCED_CA_FILE "$ENV_FILE" LABTETHER_CA_FILE || exit 1
labtether_read_env_value SOURCED_INSECURE_TLS "$ENV_FILE" LABTETHER_INSECURE_TLS || exit 1

LABTETHER_CA_FILE="${EARLY_CA_FILE:-${SOURCED_CA_FILE:-}}"
LABTETHER_INSECURE_TLS="${EARLY_INSECURE_TLS:-${SOURCED_INSECURE_TLS:-0}}"

if [[ -n "$CLI_CA_FILE" ]]; then
  LABTETHER_CA_FILE="$CLI_CA_FILE"
fi
if [[ "$CLI_INSECURE_TLS" == "1" ]]; then
  LABTETHER_INSECURE_TLS=1
fi
labtether_validate_tls_options || exit 1
if labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}"; then
  echo "Authenticated smoke tests refuse --insecure-tls; provide --ca-file or use OS trust" >&2
  exit 1
fi

API_BASE="${EARLY_API_BASE_URL:-${SOURCED_API_BASE_URL:-http://localhost:8080}}"
AGENT_BASE="${EARLY_AGENT_BASE_URL:-${SOURCED_AGENT_BASE_URL:-http://localhost:8090}}"
AUTH_TOKEN="${SOURCED_API_TOKEN:-${SOURCED_OWNER_TOKEN:-${EARLY_API_TOKEN:-${EARLY_OWNER_TOKEN:-}}}}"

if [[ -z "$AUTH_TOKEN" ]]; then
  echo "No LABTETHER_API_TOKEN or LABTETHER_OWNER_TOKEN found in $ENV_FILE" >&2
  exit 1
fi

labtether_prepare_curl_auth "$AUTH_TOKEN" || exit 1
labtether_clear_token_environment
unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_API_BASE_URL EARLY_AGENT_BASE_URL EARLY_CA_FILE EARLY_INSECURE_TLS
unset SOURCED_API_TOKEN SOURCED_OWNER_TOKEN SOURCED_API_BASE_URL SOURCED_AGENT_BASE_URL SOURCED_CA_FILE SOURCED_INSECURE_TLS
trap labtether_cleanup_curl_security EXIT

# TLS redirect detection is now handled after compose starts (see below).

smoke_init_cleanup

if [[ ! -f "$COMPOSE_FILE" ]]; then
  echo "Missing compose file: $COMPOSE_FILE" >&2
  exit 1
fi

if ! require_command curl || ! require_command jq || ! require_command od; then
  exit 1
fi

compose_cmd=()

resolve_compose_cmd() {
  compose_cmd=()
  if has_command docker; then
    if docker compose version >/dev/null 2>&1; then
      compose_cmd=(docker compose)
    elif has_command docker-compose; then
      compose_cmd=(docker-compose)
    fi
  fi
  if [[ ${#compose_cmd[@]} -gt 0 ]]; then
    compose_cmd+=(--env-file "$ENV_FILE")
    if labtether_value_is_true "$EPHEMERAL_STACK"; then
      compose_cmd+=(-p "$SMOKE_COMPOSE_PROJECT")
    fi
    compose_cmd+=(-f "$COMPOSE_FILE")
  fi
}

if [[ "$SKIP_COMPOSE" == "0" ]]; then
  resolve_compose_cmd

  if [[ ${#compose_cmd[@]} -eq 0 ]]; then
    echo "Docker compose not available (install docker with compose plugin or docker-compose)" >&2
    exit 1
  fi
fi

STARTED_COMPOSE=0
cleanup_stack() {
  if [[ "$SKIP_COMPOSE" == "0" && "$KEEP_UP" == "0" && "$STARTED_COMPOSE" == "1" ]]; then
    log "Shutting down docker compose"
    if labtether_value_is_true "$EPHEMERAL_STACK"; then
      "${compose_cmd[@]}" down -v
    else
      "${compose_cmd[@]}" down
    fi
  fi
}

restore_retention_settings() {
  if [[ "$retention_settings_mutated" != "1" || -z "${retention_restore_payload}" ]]; then
    return 0
  fi

  body=""
  status=""
  run_request body status POST "$API_BASE/settings/retention" "${retention_restore_payload}"
  if [[ "$status" != "200" ]]; then
    log "  [CLEANUP] retention settings restore failed with status ${status:-000}"
    return 1
  fi
  if ! printf '%s' "$body" | jq -e --argjson expected "$retention_restore_payload" '.settings == $expected' >/dev/null 2>&1; then
    log "  [CLEANUP] retention settings restore verification failed"
    return 1
  fi
  retention_settings_mutated=0
}

restore_runtime_settings() {
  if [[ "$runtime_settings_mutated" != "1" || -z "$runtime_restore_mode" ]]; then
    return 0
  fi

  local body=""
  local status=""
  if [[ "$runtime_restore_mode" == "patch" ]]; then
    run_request body status PATCH "$API_BASE/settings/runtime" "$runtime_restore_payload"
  else
    run_request body status POST "$API_BASE/settings/runtime/reset" '{"keys":["console.poll_interval_seconds"]}'
  fi
  if [[ "$status" != "200" ]]; then
    log "  [CLEANUP] runtime settings restore failed with status ${status:-000}"
    return 1
  fi

  if [[ "$runtime_restore_mode" == "patch" ]]; then
    local expected
    local actual
    expected=$(printf '%s' "$runtime_restore_payload" | jq -r '.values["console.poll_interval_seconds"]')
    actual=$(printf '%s' "$body" | jq -r '.overrides["console.poll_interval_seconds"] // empty')
    [[ "$actual" == "$expected" ]] || {
      log "  [CLEANUP] runtime settings restore verification failed"
      return 1
    }
  elif printf '%s' "$body" | jq -e '.overrides | has("console.poll_interval_seconds")' >/dev/null 2>&1; then
    log "  [CLEANUP] runtime settings reset verification failed"
    return 1
  fi
  runtime_settings_mutated=0
}

prepare_owned_compose_ca() {
  if [[ "$STARTED_COMPOSE" != "1" || "$API_BASE" != https://* || -n "${LABTETHER_CA_FILE:-}" ]] || labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}"; then
    return 0
  fi
  labtether_ensure_secure_curl_dir || return 1
  local container_id=""
  local attempt
  for ((attempt = 0; attempt < 20; attempt++)); do
    container_id=$("${compose_cmd[@]}" ps -q labtether 2>/dev/null || true)
    [[ -n "$container_id" ]] && break
    sleep 1
  done
  if [[ -z "$container_id" ]]; then
    log "Unable to resolve the owned LabTether container for CA acquisition"
    return 1
  fi
  local owned_ca="${LABTETHER_SECURE_CURL_DIR}/owned-compose-ca.crt"
  local copied=0
  for ((attempt = 0; attempt < 20; attempt++)); do
    if docker cp "${container_id}:/ca/ca.crt" "$owned_ca" >/dev/null 2>&1 && [[ -s "$owned_ca" ]]; then
      copied=1
      break
    fi
    sleep 1
  done
  if [[ "$copied" != "1" ]]; then
    log "Unable to copy the owned compose CA; provide --ca-file or install the CA in the system trust store"
    return 1
  fi
  chmod 600 "$owned_ca"
  LABTETHER_CA_FILE="$owned_ca"
  log "Using CA copied from the script-owned LabTether service"
}

verify_no_smoke_residue() {
  # Assigned in the sourced smoke-common helper.
  # shellcheck disable=SC2154
  if [[ ${#smoke_cleanup_tasks[@]} -ne 0 ]]; then
    # The caller must run registered cleanup first.
    return 1
  fi
  local -a residue_paths=(
    "/assets"
    "/groups"
    "/logs/views?limit=200"
    "/actions/runs?limit=200"
    "/updates/plans?limit=200"
    "/updates/runs?limit=200"
    "/alerts/silences?limit=200"
    "/notifications/channels?limit=200"
    "/alerts/routes?limit=200"
    "/incidents?limit=200"
    "/synthetic-checks?limit=200"
    "/group-profiles?limit=200"
    "/group-failover-pairs?limit=200"
    "/hub-collectors?limit=200"
  )
  local path
  local body=""
  local status=""
  local failures=0
  for path in "${residue_paths[@]}"; do
    body=""
    status=""
    run_request body status GET "$API_BASE${path}" "" 1 || true
    if [[ "$status" != "200" ]]; then
      log "  [CLEANUP] residue verification failed for ${path} with status ${status:-000}"
      failures=$((failures + 1))
      continue
    fi
    if [[ "$body" == *"$SMOKE_RUN_TOKEN"* ]]; then
      log "  [CLEANUP] smoke residue remains visible at ${path}"
      failures=$((failures + 1))
    fi
  done
  [[ "$failures" -eq 0 ]]
}

cleanup() {
  local original_rc=$?
  if [[ "$cleanup_running" == "1" ]]; then
    return
  fi
  cleanup_running=1
  trap - EXIT INT TERM HUP
  set +e
  local cleanup_failed=0
  restore_runtime_settings || cleanup_failed=1
  restore_retention_settings || cleanup_failed=1
  smoke_run_cleanup || cleanup_failed=1
  if [[ "$SKIP_COMPOSE" == "0" ]] || labtether_value_is_true "$ALLOW_MUTATIONS"; then
    verify_no_smoke_residue || cleanup_failed=1
  fi
  cleanup_stack || cleanup_failed=1
  labtether_cleanup_curl_security || cleanup_failed=1
  if [[ "$cleanup_failed" == "1" ]]; then
    log "RESULT: FAILED (cleanup incomplete)"
    original_rc=1
  fi
  exit "$original_rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 129' HUP
trap 'exit 143' TERM

log "Project root: $PROJECT_ROOT"
log "Env file   : $ENV_FILE"

if [[ "$SKIP_COMPOSE" == "0" ]]; then
  if [[ "$SKIP_BUILD" == "1" ]]; then
    log "Bringing up compose without build"
    "${compose_cmd[@]}" up -d
  else
    log "Bringing up compose with build"
    "${compose_cmd[@]}" up -d --build
  fi
  STARTED_COMPOSE=1

  # Hub in TLS auto mode returns a redirect hint on HTTP /healthz.
  # Detect this and switch to HTTPS before waiting for health.
  if [[ "${API_BASE}" == http://* ]]; then
    log "Probing for TLS redirect on ${API_BASE}/healthz"
    for _probe_i in $(seq 1 30); do
      labtether_build_curl_request_args "${API_BASE}/healthz" 0 || exit 1
      health_probe="$(labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sS --connect-timeout 2 --max-time 2 "${API_BASE}/healthz" 2>/dev/null || true)"
      if [[ "${health_probe}" == *'"status":"redirect_active"'* ]]; then
        base_no_scheme="${API_BASE#http://}"
        host_port="${base_no_scheme%%/*}"
        host="${host_port%%:*}"
        redirect_port="$(printf '%s' "${health_probe}" | sed -n 's/.*https on port \([0-9][0-9]*\).*/\1/p')"
        if [[ -z "${redirect_port}" ]]; then
          redirect_port=8443
        fi
        API_BASE="https://${host}:${redirect_port}"
        log "  Detected TLS redirect; switching API base to ${API_BASE}"
        break
      fi
      sleep 2
    done
  fi

  prepare_owned_compose_ca || exit 1

  wait_for_http "LabTether health" "$API_BASE/healthz" || exit 1
  AGENT_AVAILABLE=0
  if wait_for_http "Agent health" "$AGENT_BASE/healthz" 30; then
    AGENT_AVAILABLE=1
  else
    log "  Agent not reachable (may need more time for TLS cert propagation) — continuing with hub-only smoke tests"
  fi
else
  log "Skipping compose startup (assuming services already running)"
  wait_for_http "API health" "$API_BASE/healthz" || exit 1
fi

if [[ "$SKIP_COMPOSE" == "1" ]] && ! labtether_value_is_true "$ALLOW_MUTATIONS"; then
  log "\nRunning safe read-only smoke checks (--allow-mutations is required for fixture writes with --skip-compose)"
  body=""
  status=""
  run_request body status GET "$API_BASE/healthz" "" 0
  assert_equal "GET /healthz" "200" "$status"
  run_request body status GET "$API_BASE/auth/me"
  assert_equal "GET /auth/me" "200" "$status"
  run_request body status GET "$API_BASE/assets"
  assert_equal "GET /assets" "200" "$status"
  run_request body status GET "$API_BASE/connectors"
  assert_equal "GET /connectors" "200" "$status"
  run_request body status GET "$API_BASE/settings/runtime"
  assert_equal "GET /settings/runtime" "200" "$status"
  run_request body status GET "$API_BASE/settings/retention"
  assert_equal "GET /settings/retention" "200" "$status"
  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    log "RESULT: FAILED"
    exit 1
  fi
  log "RESULT: PASSED (read-only mode)"
  exit 0
fi

log "\nRunning API smoke checks"

# 1) health (public)
body=""
status=""
run_request body status GET "$API_BASE/healthz" "" 0
assert_equal "GET /healthz" "200" "$status"
smoke_print_verbose_json "body" "$body"

# 2) auth required check on protected endpoint
body=""
status=""
run_request body status POST "$API_BASE/terminal/sessions" '{"actor_id":"owner","target":"'"$SMOKE_COMMAND_TARGET_ID"'","mode":"interactive"}' 0
assert_equal "POST /terminal/sessions without auth rejected" "401" "$status"

# 3) create session
body=""
status=""
terminal_target="$SMOKE_COMMAND_TARGET_ID"
if labtether_value_is_true "$ALLOW_REMOTE_EXEC"; then
  terminal_target="$REMOTE_EXEC_TARGET"
fi
terminal_payload=$(jq -cn --arg target "$terminal_target" '{actor_id:"owner",target:$target,mode:"interactive"}')
run_request body status POST "$API_BASE/terminal/sessions" "$terminal_payload"
assert_equal "POST /terminal/sessions" "201" "$status"
smoke_print_verbose_json "session response" "$body"
session_id=$(extract_json_string "id" "$body")
if [[ -z "$session_id" ]]; then
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse session id from response\n'
  session_id=""
else
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created session %s\n' "$session_id"
  smoke_register_cleanup DELETE "/terminal/sessions/${session_id}" "terminal session ${session_id}"
fi

# 4) create command
if [[ -n "$session_id" ]]; then
  body=""
  status=""
  run_request body status POST "$API_BASE/terminal/sessions/${session_id}/commands" '{"actor_id":"owner","command":"uname -a"}'
  assert_equal "POST /terminal/sessions/{id}/commands" "202" "$status"
  smoke_print_verbose_json "command response" "$body"

  command_id=$(extract_json_string "id" "$body")
  if [[ -z "$command_id" ]]; then
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse command id\n'
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] command id parsed %s\n' "$command_id"
  fi

  # 5) list commands for session (auth)
  body=""
  status=""
  run_request body status GET "$API_BASE/terminal/sessions/${session_id}/commands"
  assert_equal "GET /terminal/sessions/{id}/commands" "200" "$status"
  if [[ -n "$command_id" && "$body" == *"$command_id"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] command id found in session command list\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] command id not found in session command list\n'
  fi

  # 6) recent commands endpoint (auth)
  body=""
  status=""
  run_request body status GET "$API_BASE/terminal/commands/recent?limit=12"
  assert_equal "GET /terminal/commands/recent" "200" "$status"
  if [[ "$body" == *"$session_id"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] recent commands includes session id\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] recent commands does not include session id\n'
  fi

  # 7) audit events
  body=""
  status=""
  run_request body status GET "$API_BASE/audit/events?limit=20"
  assert_equal "GET /audit/events" "200" "$status"
  if [[ -n "$body" && "$body" == *"terminal.command.queued"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] audit contains queued event\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] audit did not include terminal.command.queued\n'
  fi

  # 8) worker stats
  body=""
  status=""
  run_request body status GET "$API_BASE/worker/stats"  '' 0
  assert_not_equal "GET /worker/stats" "000" "$status"
  if [[ "$status" == "200" ]]; then
    if contains "processed_jobs" "$body"; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] worker stats payload returned\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] worker stats missing processed_jobs\n'
    fi
  fi
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] Skipping command flow because session id missing\n'
fi

# 9) connector list and agent health checks
body=""
status=""
run_request body status GET "$API_BASE/connectors"
assert_equal "GET /connectors" "200" "$status"

if [[ "$SKIP_COMPOSE" == "0" && "${AGENT_AVAILABLE:-0}" == "1" ]]; then
  body=""
  status=""
  run_request body status GET "$AGENT_BASE/healthz" "" 0
  assert_equal "GET /agent/healthz" "200" "$status"
elif [[ "$SKIP_COMPOSE" == "0" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] GET /agent/healthz skipped (agent not reachable during startup)\n'
else
  if check_http_status "$AGENT_BASE/healthz" 200 2; then
    body=""
    status=""
    run_request body status GET "$AGENT_BASE/healthz" "" 0
    assert_equal "GET /agent/healthz" "200" "$status"
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] GET /agent/healthz skipped (non-compose mode; agent service not running)\n'
  fi
fi

# 10) asset heartbeat and inventory endpoints
body=""
status=""
run_request body status POST "$API_BASE/assets/heartbeat" '{"asset_id":"'"$SMOKE_NODE_ID"'","type":"host","name":"Smoke Node '"$SMOKE_RUN_TOKEN"'","source":"smoke-test","status":"online","platform":"linux"}'
assert_equal "POST /assets/heartbeat" "202" "$status"
if [[ "$status" == "202" ]]; then
  smoke_register_cleanup DELETE "/assets/${SMOKE_NODE_ID}" "asset ${SMOKE_NODE_ID}"
fi

body=""
status=""
smoke_group_code="SMK$(date +%s)$((RANDOM % 1000))"
run_request body status POST "$API_BASE/groups" "{\"name\":\"${SMOKE_GROUP_NAME}\",\"slug\":\"${smoke_group_code}\",\"location\":\"Austin\",\"latitude\":30.2672,\"longitude\":-97.7431,\"geo_label\":\"Austin, TX\",\"status\":\"active\"}"
assert_equal "POST /groups" "201" "$status"
smoke_group_id=$(extract_json_string "id" "$body")
if [[ -z "$smoke_group_id" ]]; then
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse smoke group id from response\n'
else
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created smoke group %s\n' "$smoke_group_id"
  smoke_register_cleanup DELETE "/groups/${smoke_group_id}" "group ${smoke_group_id}"
fi

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"$SMOKE_GROUP_NODE_ID\",\"type\":\"host\",\"name\":\"Smoke Group Node ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"group_id\":\"${smoke_group_id}\",\"status\":\"online\",\"platform\":\"linux\",\"metadata\":{\"cpu_percent\":\"15.3\",\"memory_percent\":\"44.1\"}}"
  assert_equal "POST /assets/heartbeat (group scoped)" "202" "$status"
  if [[ "$status" == "202" ]]; then
    smoke_register_cleanup DELETE "/assets/${SMOKE_GROUP_NODE_ID}" "asset ${SMOKE_GROUP_NODE_ID}"
  fi
fi

body=""
status=""
run_request body status GET "$API_BASE/assets"
assert_equal "GET /assets" "200" "$status"
if [[ "$body" == *"${SMOKE_NODE_ID}"* ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] assets list includes smoke heartbeat asset\n'
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] assets list missing smoke heartbeat asset\n'
fi

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/assets?group_id=${smoke_group_id}"
  assert_equal "GET /assets?group_id" "200" "$status"
  if [[ "$body" == *"${SMOKE_GROUP_NODE_ID}"* && "$body" != *"${SMOKE_NODE_ID}"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] group filtered assets returned scoped node only\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] group filtered assets did not return expected scoped list\n'
  fi
fi

body=""
status=""
run_request body status GET "$API_BASE/metrics/overview?window=15m"
assert_equal "GET /metrics/overview" "200" "$status"

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/metrics/overview?window=15m&group_id=${smoke_group_id}"
  assert_equal "GET /metrics/overview?group_id" "200" "$status"
  if [[ "$body" == *"${SMOKE_GROUP_NODE_ID}"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] group filtered telemetry overview returned scoped node\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] group filtered telemetry overview missing scoped node\n'
  fi
fi

body=""
status=""
run_request body status GET "$API_BASE/metrics/assets/${SMOKE_NODE_ID}?window=15m&step=30s"
assert_equal "GET /metrics/assets/{id}" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/logs/sources?limit=10"
assert_equal "GET /logs/sources" "200" "$status"

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/logs/sources?limit=10&group_id=${smoke_group_id}"
  assert_equal "GET /logs/sources?group_id" "200" "$status"
fi

body=""
status=""
run_request body status GET "$API_BASE/logs/query?window=1h&limit=20"
assert_equal "GET /logs/query" "200" "$status"

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/logs/query?window=1h&limit=20&group_id=${smoke_group_id}"
  assert_equal "GET /logs/query?group_id" "200" "$status"
fi

body=""
status=""
run_request body status POST "$API_BASE/logs/views" "{\"name\":\"Smoke Logs ${SMOKE_RUN_TOKEN}\",\"window\":\"1h\",\"level\":\"info\"}"
assert_equal "POST /logs/views" "201" "$status"
smoke_log_view_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_log_view_id" ]]; then
  smoke_register_cleanup DELETE "/logs/views/${smoke_log_view_id}" "log view ${smoke_log_view_id}"
fi

body=""
status=""
run_request body status GET "$API_BASE/queue/dead-letters?window=24h&limit=20"
assert_equal "GET /queue/dead-letters" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/connectors/proxmox/actions"
assert_equal "GET /connectors/{id}/actions" "200" "$status"

body=""
status=""
smoke_action_target="$SMOKE_NODE_ID"
if labtether_value_is_true "$ALLOW_REMOTE_EXEC"; then
  smoke_action_target="$REMOTE_EXEC_TARGET"
fi
action_payload=$(jq -cn --arg target "$smoke_action_target" '{type:"command",actor_id:"owner",target:$target,command:"uptime"}')
run_request body status POST "$API_BASE/actions/execute" "$action_payload"
assert_equal "POST /actions/execute" "202" "$status"
action_run_id=$(extract_json_string "id" "$body")
if [[ -z "$action_run_id" ]]; then
  action_run_id=$(extract_json_string "run_id" "$body")
fi
if [[ -z "$action_run_id" ]]; then
  action_run_id=$(extract_nested_json_id "run" "$body")
fi
if [[ -n "$action_run_id" ]]; then
  smoke_register_cleanup DELETE "/actions/runs/${action_run_id}" "action run ${action_run_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse action run id for cleanup\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/actions/runs?limit=10"
assert_equal "GET /actions/runs" "200" "$status"

body=""
status=""
update_plan_payload=$(jq -cn \
  --arg name "Smoke Plan ${SMOKE_RUN_TOKEN}" \
  --arg target "$smoke_action_target" \
  '{name:$name,targets:[$target],scopes:["os_packages"],default_dry_run:true}')
run_request body status POST "$API_BASE/updates/plans" "$update_plan_payload"
assert_equal "POST /updates/plans" "201" "$status"
update_plan_id=$(extract_json_string "id" "$body")
if [[ -z "$update_plan_id" ]]; then
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse update plan id from response\n'
else
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created update plan %s\n' "$update_plan_id"
  smoke_register_cleanup DELETE "/updates/plans/${update_plan_id}" "update plan ${update_plan_id}"
fi

body=""
status=""
run_request body status GET "$API_BASE/updates/plans?limit=10"
assert_equal "GET /updates/plans" "200" "$status"

if [[ -n "$update_plan_id" ]]; then
  body=""
  status=""
  run_request body status POST "$API_BASE/updates/plans/${update_plan_id}/execute" '{"actor_id":"owner","dry_run":true}'
  assert_equal "POST /updates/plans/{id}/execute" "202" "$status"
  update_run_id=$(extract_json_string "id" "$body")
  if [[ -z "$update_run_id" ]]; then
    update_run_id=$(extract_nested_json_id "run" "$body")
  fi
  if [[ -n "$update_run_id" ]]; then
    smoke_register_cleanup DELETE "/updates/runs/${update_run_id}" "update run ${update_run_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse update run id for cleanup\n'
  fi
fi

body=""
status=""
run_request body status GET "$API_BASE/updates/runs?limit=10"
assert_equal "GET /updates/runs" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/groups/reliability?window=24h"
assert_equal "GET /groups/reliability" "200" "$status"

if [[ -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/groups/${smoke_group_id}/timeline?window=24h&limit=20"
  assert_equal "GET /groups/{id}/timeline" "200" "$status"

  body=""
  status=""
  run_request body status POST "$API_BASE/groups/${smoke_group_id}/maintenance-windows" '{"name":"Smoke Guardrail Window","start_at":"2000-01-01T00:00:00Z","end_at":"2100-01-01T00:00:00Z","suppress_alerts":true,"block_actions":true,"block_updates":true}'
  assert_equal "POST /groups/{id}/maintenance-windows" "201" "$status"
  smoke_maintenance_window_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_maintenance_window_id" ]]; then
    smoke_register_cleanup DELETE "/groups/${smoke_group_id}/maintenance-windows/${smoke_maintenance_window_id}" "maintenance window ${smoke_maintenance_window_id}"
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/groups/${smoke_group_id}/maintenance-windows?active=true&limit=10"
  assert_equal "GET /groups/{id}/maintenance-windows" "200" "$status"

  body=""
  status=""
  run_request body status POST "$API_BASE/actions/execute" "{\"type\":\"command\",\"actor_id\":\"owner\",\"target\":\"${SMOKE_GROUP_NODE_ID}\",\"command\":\"echo blocked by maintenance\"}"
  assert_equal "POST /actions/execute blocked by maintenance" "423" "$status"

  body=""
  status=""
  run_request body status POST "$API_BASE/updates/plans" "{\"name\":\"Smoke Maintenance Plan ${SMOKE_RUN_TOKEN}\",\"targets\":[\"${SMOKE_GROUP_NODE_ID}\"],\"scopes\":[\"os_packages\"],\"default_dry_run\":true}"
  assert_equal "POST /updates/plans (maintenance target)" "201" "$status"
  maintenance_plan_id=$(extract_json_string "id" "$body")
  if [[ -n "$maintenance_plan_id" ]]; then
    smoke_register_cleanup DELETE "/updates/plans/${maintenance_plan_id}" "update plan ${maintenance_plan_id}"
    body=""
    status=""
    run_request body status POST "$API_BASE/updates/plans/${maintenance_plan_id}/execute" '{"actor_id":"owner","dry_run":true}'
    assert_equal "POST /updates/plans/{id}/execute blocked by maintenance" "423" "$status"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse maintenance plan id from response\n'
  fi
fi

body=""
status=""
run_request body status GET "$API_BASE/settings/runtime"
assert_equal "GET /settings/runtime" "200" "$status"
if [[ "$body" == *"console.poll_interval_seconds"* ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] runtime settings payload includes console.poll_interval_seconds\n'
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] runtime settings payload missing console.poll_interval_seconds\n'
fi

if labtether_value_is_true "$ALLOW_GLOBAL_SETTINGS"; then
  if printf '%s' "$body" | jq -e '.overrides | has("console.poll_interval_seconds")' >/dev/null 2>&1; then
    runtime_restore_mode="patch"
    prior_poll_override=$(printf '%s' "$body" | jq -er '.overrides["console.poll_interval_seconds"]') || {
      log "Unable to snapshot runtime setting; refusing to mutate it"
      exit 1
    }
    runtime_restore_payload=$(jq -cn --arg value "$prior_poll_override" '{values:{"console.poll_interval_seconds":$value}}')
  else
    runtime_restore_mode="reset"
    runtime_restore_payload='{}'
  fi

  runtime_settings_mutated=1
  body=""
  status=""
  run_request body status PATCH "$API_BASE/settings/runtime" '{"values":{"console.poll_interval_seconds":"7"}}'
  assert_equal "PATCH /settings/runtime" "200" "$status"
  if [[ "$body" == *"\"override_value\":\"7\""* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] runtime setting override persisted\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] runtime setting override not reflected\n'
  fi
  if restore_runtime_settings; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] runtime setting restored exactly\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] runtime setting restoration failed\n'
  fi
else
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] runtime setting mutation skipped (safe default)\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/settings/retention"
assert_equal "GET /settings/retention" "200" "$status"
if labtether_value_is_true "$ALLOW_DESTRUCTIVE_RETENTION"; then
  retention_restore_payload=$(printf '%s' "$body" | jq -cer '.settings') || {
    log "Unable to snapshot retention settings; refusing destructive mutation"
    exit 1
  }
  retention_settings_mutated=1
  body=""
  status=""
  run_request body status POST "$API_BASE/settings/retention" '{"preset":"balanced"}'
  assert_equal "POST /settings/retention" "200" "$status"
  if restore_retention_settings; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] retention settings restored exactly\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] retention settings restoration failed\n'
  fi
else
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] retention mutation skipped (safe default)\n'
fi

# ── Stream 1: MVP Authentication ──

log "\nAuth smoke checks"

body=""
status=""
run_request body status POST "$API_BASE/auth/login" '{"username":"admin","password":"wrong-password"}' 0
assert_equal "POST /auth/login bad creds" "401" "$status"

body=""
status=""
run_request body status GET "$API_BASE/auth/me" '' 0
assert_equal "GET /auth/me no auth" "401" "$status"

body=""
status=""
run_request body status GET "$API_BASE/auth/me"
assert_equal "GET /auth/me bearer token" "200" "$status"

body=""
status=""
run_request body status POST "$API_BASE/auth/logout" '' 0
assert_equal "POST /auth/logout" "200" "$status"

# ── Stream 2: Alert Instances, Silences, Notifications, Routes ──

log "\nAlert instances & silences smoke checks"

body=""
status=""
run_request body status GET "$API_BASE/alerts/instances?limit=10"
assert_equal "GET /alerts/instances" "200" "$status"

body=""
status=""
run_request body status POST "$API_BASE/alerts/silences" "{\"matchers\":{\"asset_id\":\"${SMOKE_NODE_ID}\"},\"reason\":\"smoke test ${SMOKE_RUN_TOKEN}\",\"starts_at\":\"2000-01-01T00:00:00Z\",\"ends_at\":\"2100-01-01T00:00:00Z\"}"
assert_equal "POST /alerts/silences" "201" "$status"
smoke_silence_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_silence_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created silence %s\n' "$smoke_silence_id"
  smoke_register_cleanup DELETE "/alerts/silences/${smoke_silence_id}" "alert silence ${smoke_silence_id}"

  body=""
  status=""
  run_request body status GET "$API_BASE/alerts/silences?limit=10"
  assert_equal "GET /alerts/silences" "200" "$status"

  body=""
  status=""
  run_request body status DELETE "$API_BASE/alerts/silences/${smoke_silence_id}"
  assert_equal "DELETE /alerts/silences/{id}" "200" "$status"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse silence id\n'
fi

log "\nNotification channels & routes smoke checks"

body=""
status=""
outbound_enabled=false
smoke_webhook_target="${WEBHOOK_TARGET:-https://smoke.invalid/hook}"
if labtether_value_is_true "$ALLOW_OUTBOUND"; then
  outbound_enabled=true
fi
channel_payload=$(jq -cn \
  --arg name "Smoke Webhook ${SMOKE_RUN_TOKEN}" \
  --arg url "$smoke_webhook_target" \
  --argjson enabled "$outbound_enabled" \
  '{name:$name,type:"webhook",config:{url:$url},enabled:$enabled}')
run_request body status POST "$API_BASE/notifications/channels" "$channel_payload"
assert_equal "POST /notifications/channels" "201" "$status"
smoke_channel_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_channel_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created channel %s\n' "$smoke_channel_id"
  smoke_register_cleanup DELETE "/notifications/channels/${smoke_channel_id}" "notification channel ${smoke_channel_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse channel id\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/notifications/channels?limit=10"
assert_equal "GET /notifications/channels" "200" "$status"

body=""
status=""
route_payload=$(jq -cn \
  --arg name "Smoke Route ${SMOKE_RUN_TOKEN}" \
  --arg channel "${smoke_channel_id:-none}" \
  --arg asset "$SMOKE_NODE_ID" \
  --argjson enabled "$outbound_enabled" \
  '{name:$name,matchers:{asset_id:$asset},channel_ids:[$channel],enabled:$enabled}')
run_request body status POST "$API_BASE/alerts/routes" "$route_payload"
assert_equal "POST /alerts/routes" "201" "$status"
smoke_route_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_route_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created route %s\n' "$smoke_route_id"
  smoke_register_cleanup DELETE "/alerts/routes/${smoke_route_id}" "alert route ${smoke_route_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse route id\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/alerts/routes?limit=10"
assert_equal "GET /alerts/routes" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/notifications/history?limit=10"
assert_equal "GET /notifications/history" "200" "$status"

# ── Stream 3: Dependency Graph & Incident Asset Linking ──

log "\nDependency graph smoke checks"

# Ensure we have two assets for dependency testing
body=""
status=""
run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"type\":\"host\",\"name\":\"Dep Source ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"status\":\"online\",\"platform\":\"linux\"}"
assert_equal "POST /assets/heartbeat (dep src)" "202" "$status"
if [[ "$status" == "202" ]]; then
  smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_SRC_ID}" "asset ${SMOKE_DEP_SRC_ID}"
fi

body=""
status=""
run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"${SMOKE_DEP_TGT_ID}\",\"type\":\"host\",\"name\":\"Dep Target ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"status\":\"online\",\"platform\":\"linux\"}"
assert_equal "POST /assets/heartbeat (dep tgt)" "202" "$status"
if [[ "$status" == "202" ]]; then
  smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_TGT_ID}" "asset ${SMOKE_DEP_TGT_ID}"
fi

body=""
status=""
run_request body status POST "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies" "{\"source_asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"target_asset_id\":\"${SMOKE_DEP_TGT_ID}\",\"relationship_type\":\"depends_on\",\"direction\":\"downstream\",\"criticality\":\"high\"}"
assert_equal "POST /assets/{id}/dependencies" "201" "$status"
smoke_dep_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_dep_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created dependency %s\n' "$smoke_dep_id"
  smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_SRC_ID}/dependencies/${smoke_dep_id}" "dependency ${smoke_dep_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse dependency id\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies?limit=10"
assert_equal "GET /assets/{id}/dependencies" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/blast-radius?max_depth=3"
assert_equal "GET /assets/{id}/blast-radius" "200" "$status"

body=""
status=""
run_request body status GET "$API_BASE/assets/${SMOKE_DEP_TGT_ID}/upstream?max_depth=3"
assert_equal "GET /assets/{id}/upstream" "200" "$status"

if [[ -n "$smoke_dep_id" ]]; then
  body=""
  status=""
  run_request body status DELETE "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies/${smoke_dep_id}"
  assert_equal "DELETE /assets/{id}/dependencies/{depId}" "200" "$status"
fi

log "\nIncident asset linking smoke checks"

body=""
status=""
run_request body status POST "$API_BASE/incidents" "{\"title\":\"Smoke Asset Link Test ${SMOKE_RUN_TOKEN}\",\"severity\":\"low\"}"
assert_equal "POST /incidents (asset link)" "201" "$status"
smoke_link_inc_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_link_inc_id" ]]; then
  smoke_register_cleanup DELETE "/incidents/${smoke_link_inc_id}" "incident ${smoke_link_inc_id}"
  body=""
  status=""
  run_request body status POST "$API_BASE/incidents/${smoke_link_inc_id}/link-asset" "{\"asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"role\":\"primary\"}"
  assert_equal "POST /incidents/{id}/link-asset" "201" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/incidents/${smoke_link_inc_id}/assets"
  assert_equal "GET /incidents/{id}/assets" "200" "$status"
  if [[ "$body" == *"${SMOKE_DEP_SRC_ID}"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] incident asset list includes linked asset\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] incident asset list missing linked asset\n'
  fi
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse incident id for asset linking\n'
fi

# ── Stream 4: Synthetic Checks, Group Profiles, Failover Pairs, Hub Collectors ──

log "\nSynthetic checks smoke checks"

# Create synthetic check
body=""
status=""
smoke_synthetic_target="${SYNTHETIC_TARGET:-https://smoke.invalid/healthz}"
synthetic_payload=$(jq -cn \
  --arg name "Smoke HTTP Check ${SMOKE_RUN_TOKEN}" \
  --arg target "$smoke_synthetic_target" \
  --argjson enabled "$outbound_enabled" \
  '{name:$name,check_type:"http",target:$target,interval_seconds:60,enabled:$enabled}')
run_request body status POST "$API_BASE/synthetic-checks" "$synthetic_payload"
assert_equal "POST /synthetic-checks" "201" "$status"
smoke_check_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_check_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created synthetic check %s\n' "$smoke_check_id"
  smoke_register_cleanup DELETE "/synthetic-checks/${smoke_check_id}" "synthetic check ${smoke_check_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse synthetic check id\n'
fi

# List synthetic checks
body=""
status=""
run_request body status GET "$API_BASE/synthetic-checks?limit=10"
assert_equal "GET /synthetic-checks" "200" "$status"

# Get synthetic check results
if [[ -n "${smoke_check_id:-}" ]]; then
  body=""
  status=""
  run_request body status GET "$API_BASE/synthetic-checks/${smoke_check_id}/results?limit=10"
  assert_equal "GET /synthetic-checks/{id}/results" "200" "$status"

  # Delete synthetic check
  body=""
  status=""
  run_request body status DELETE "$API_BASE/synthetic-checks/${smoke_check_id}"
  assert_equal "DELETE /synthetic-checks/{id}" "200" "$status"
fi

log "\nSite profiles smoke checks"

body=""
status=""
run_request body status POST "$API_BASE/group-profiles" "{\"name\":\"Smoke Profile ${SMOKE_RUN_TOKEN}\",\"description\":\"Smoke test profile\",\"config\":{\"expected_asset_count\":2,\"required_platforms\":[\"linux\"],\"min_online_percent\":80}}"
assert_equal "POST /group-profiles" "201" "$status"
smoke_profile_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_profile_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created group profile %s\n' "$smoke_profile_id"
  smoke_register_cleanup DELETE "/group-profiles/${smoke_profile_id}" "group profile ${smoke_profile_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse group profile id\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/group-profiles?limit=10"
assert_equal "GET /group-profiles" "200" "$status"

if [[ -n "${smoke_profile_id:-}" && -n "${smoke_group_id:-}" ]]; then
  body=""
  status=""
  run_request body status POST "$API_BASE/group-profiles/${smoke_profile_id}/assign" "{\"group_id\":\"${smoke_group_id}\"}"
  assert_equal "POST /group-profiles/{id}/assign" "201" "$status"
fi

if [[ -n "${smoke_profile_id:-}" ]]; then
  body=""
  status=""
  run_request body status DELETE "$API_BASE/group-profiles/${smoke_profile_id}"
  assert_equal "DELETE /group-profiles/{id}" "200" "$status"
fi

log "\nSite failover pairs smoke checks"

if [[ -n "${smoke_group_id:-}" ]]; then
  # Create a second group for failover pairing
  body=""
  status=""
  smoke_backup_site_code="SMB$(date +%s)$((RANDOM % 1000))"
  run_request body status POST "$API_BASE/groups" "{\"name\":\"${SMOKE_BACKUP_GROUP_NAME}\",\"slug\":\"${smoke_backup_site_code}\",\"location\":\"Denver\",\"latitude\":39.7392,\"longitude\":-104.9903,\"geo_label\":\"Denver, CO\",\"status\":\"active\"}"
  assert_equal "POST /groups (backup)" "201" "$status"
  smoke_backup_group_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_backup_group_id" ]]; then
    smoke_register_cleanup DELETE "/groups/${smoke_backup_group_id}" "group ${smoke_backup_group_id}"
  fi

  if [[ -n "$smoke_backup_group_id" ]]; then
    body=""
    status=""
    run_request body status POST "$API_BASE/group-failover-pairs" "{\"primary_group_id\":\"${smoke_group_id}\",\"backup_group_id\":\"${smoke_backup_group_id}\"}"
    assert_equal "POST /group-failover-pairs" "201" "$status"
    smoke_failover_id=$(extract_json_string "id" "$body")
    if [[ -n "$smoke_failover_id" ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] created failover pair %s\n' "$smoke_failover_id"
      smoke_register_cleanup DELETE "/group-failover-pairs/${smoke_failover_id}" "failover pair ${smoke_failover_id}"
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] could not parse failover pair id\n'
    fi

    body=""
    status=""
    run_request body status GET "$API_BASE/group-failover-pairs?limit=10"
    assert_equal "GET /group-failover-pairs" "200" "$status"

    if [[ -n "${smoke_failover_id:-}" ]]; then
      body=""
      status=""
      run_request body status POST "$API_BASE/group-failover-pairs/${smoke_failover_id}/check-readiness"
      assert_equal "POST /group-failover-pairs/{id}/check-readiness" "200" "$status"

      body=""
      status=""
      run_request body status DELETE "$API_BASE/group-failover-pairs/${smoke_failover_id}"
      assert_equal "DELETE /group-failover-pairs/{id}" "200" "$status"
    fi
  fi
fi

log "\nHub collectors smoke checks"

body=""
status=""
smoke_ssh_target="${SSH_TARGET:-smoke.invalid}"
collector_payload=$(jq -cn \
  --arg asset "$SMOKE_NODE_ID" \
  --arg host "$smoke_ssh_target" \
  --argjson enabled "$outbound_enabled" \
  '{asset_id:$asset,collector_type:"ssh",config:{host:$host,user:"labtether-smoke",script:"uname -a"},interval_seconds:300,enabled:$enabled}')
run_request body status POST "$API_BASE/hub-collectors" "$collector_payload"
assert_equal "POST /hub-collectors" "201" "$status"
smoke_collector_id=$(extract_json_string "id" "$body")
if [[ -n "$smoke_collector_id" ]]; then
  PASS_COUNT=$((PASS_COUNT + 1))
  printf '  [PASS] created hub collector %s\n' "$smoke_collector_id"
  smoke_register_cleanup DELETE "/hub-collectors/${smoke_collector_id}" "hub collector ${smoke_collector_id}"
else
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] could not parse hub collector id\n'
fi

body=""
status=""
run_request body status GET "$API_BASE/hub-collectors?limit=10"
assert_equal "GET /hub-collectors" "200" "$status"

if [[ -n "${smoke_collector_id:-}" ]]; then
  body=""
  status=""
  run_request body status DELETE "$API_BASE/hub-collectors/${smoke_collector_id}"
  assert_equal "DELETE /hub-collectors/{id}" "200" "$status"
fi

log "\nSmoke test summary"
TOTAL_COUNT=$((PASS_COUNT + FAIL_COUNT))
printf '  Passed: %s\n' "$PASS_COUNT"
printf '  Failed: %s\n' "$FAIL_COUNT"
printf '  Total : %s\n' "$TOTAL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  log "RESULT: FAILED"
  exit 1
fi

log "RESULT: PASSED"
