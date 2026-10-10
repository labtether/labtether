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
# shellcheck disable=SC2034 # Consumed by sourced smoke helpers.
SKIP_COUNT=0

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
    smoke_print_summary
    log "RESULT: FAILED"
    exit 1
  fi
  smoke_print_summary
  log "RESULT: PASSED (read-only mode)"
  exit 0
fi

log "\nRunning API smoke checks"

# shellcheck source=scripts/smoke/sessions.sh
source "${PROJECT_ROOT}/scripts/smoke/sessions.sh"
smoke_check_sessions

# shellcheck source=scripts/smoke/inventory.sh
source "${PROJECT_ROOT}/scripts/smoke/inventory.sh"
smoke_check_inventory

# shellcheck source=scripts/smoke/alerting.sh
source "${PROJECT_ROOT}/scripts/smoke/alerting.sh"
smoke_check_alerting

# shellcheck source=scripts/smoke/relationships.sh
source "${PROJECT_ROOT}/scripts/smoke/relationships.sh"
smoke_check_relationships

# shellcheck source=scripts/smoke/automation.sh
source "${PROJECT_ROOT}/scripts/smoke/automation.sh"
smoke_check_automation

smoke_print_summary

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  log "RESULT: FAILED"
  exit 1
fi

log "RESULT: PASSED"
