#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_API_BASE_URL EARLY_CA_FILE EARLY_INSECURE_TLS AUTH_TOKEN
EARLY_API_TOKEN="${LABTETHER_API_TOKEN-}"
EARLY_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"
EARLY_API_BASE_URL="${LABTETHER_API_BASE_URL-}"
EARLY_CA_FILE="${LABTETHER_CA_FILE-}"
EARLY_INSECURE_TLS="${LABTETHER_INSECURE_TLS-}"
unset LABTETHER_API_TOKEN LABTETHER_OWNER_TOKEN

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-45}"
INTEGRATION_MANAGE_STACK="${INTEGRATION_MANAGE_STACK:-auto}"
INTEGRATION_COMPOSE_UP_ARGS="${INTEGRATION_COMPOSE_UP_ARGS:--d}"
INTEGRATION_TARGET="${LABTETHER_INTEGRATION_TARGET:-}"
INTEGRATION_ALLOW_REMOTE_EXEC="${LABTETHER_INTEGRATION_ALLOW_REMOTE_EXEC:-0}"
CLI_CA_FILE=""
CLI_INSECURE_TLS=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      INTEGRATION_TARGET="${2:-}"
      shift
      ;;
    --allow-remote-exec)
      INTEGRATION_ALLOW_REMOTE_EXEC=1
      ;;
    --ca-file)
      CLI_CA_FILE="${2:-}"
      shift
      ;;
    --insecure-tls)
      CLI_INSECURE_TLS=1
      ;;
    -h|--help)
      cat <<'USAGE'
Usage: scripts/integration-queue-flow.sh --allow-remote-exec --target <asset-id> [--ca-file <path>]

This test executes commands and dry-run update jobs against a real managed
asset, so both an explicit target and explicit execution opt-in are required.
Authenticated insecure TLS is rejected.
USAGE
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
  shift
done

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/smoke-common.sh"
unset SOURCED_API_TOKEN SOURCED_OWNER_TOKEN SOURCED_API_BASE_URL SOURCED_CA_FILE SOURCED_INSECURE_TLS
if [[ -f "${ENV_FILE}" ]]; then
  labtether_require_private_env_file "$ENV_FILE" || exit 1
  labtether_read_env_value SOURCED_API_TOKEN "$ENV_FILE" LABTETHER_API_TOKEN || exit 1
  labtether_read_env_value SOURCED_OWNER_TOKEN "$ENV_FILE" LABTETHER_OWNER_TOKEN || exit 1
  labtether_read_env_value SOURCED_API_BASE_URL "$ENV_FILE" LABTETHER_API_BASE_URL || exit 1
  labtether_read_env_value SOURCED_CA_FILE "$ENV_FILE" LABTETHER_CA_FILE || exit 1
  labtether_read_env_value SOURCED_INSECURE_TLS "$ENV_FILE" LABTETHER_INSECURE_TLS || exit 1
fi
unset AUTH_TOKEN

LABTETHER_CA_FILE="${EARLY_CA_FILE:-${SOURCED_CA_FILE:-}}"
LABTETHER_INSECURE_TLS="${EARLY_INSECURE_TLS:-${SOURCED_INSECURE_TLS:-0}}"

if [[ -n "$CLI_CA_FILE" ]]; then
  LABTETHER_CA_FILE="$CLI_CA_FILE"
fi
if [[ "$CLI_INSECURE_TLS" == "1" ]]; then
  LABTETHER_INSECURE_TLS=1
fi

API_BASE="${EARLY_API_BASE_URL:-${SOURCED_API_BASE_URL:-http://localhost:8080}}"
AUTH_TOKEN="${SOURCED_API_TOKEN:-${SOURCED_OWNER_TOKEN:-${EARLY_API_TOKEN:-${EARLY_OWNER_TOKEN:-}}}}"
INTEGRATION_RUN_TOKEN="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
INTEGRATION_COMPOSE_PROJECT="labtether-integration-${INTEGRATION_RUN_TOKEN}"

if [[ -z "${AUTH_TOKEN}" ]]; then
  echo "missing LABTETHER_API_TOKEN or LABTETHER_OWNER_TOKEN" >&2
  exit 1
fi
if ! labtether_value_is_true "$INTEGRATION_ALLOW_REMOTE_EXEC" || [[ -z "$INTEGRATION_TARGET" ]]; then
  echo "integration queue flow requires --allow-remote-exec and --target <asset-id>" >&2
  exit 1
fi
labtether_validate_tls_options || exit 1
if labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}"; then
  echo "Authenticated integration tests refuse --insecure-tls; provide --ca-file or use OS trust" >&2
  exit 1
fi
labtether_prepare_curl_auth "$AUTH_TOKEN" || exit 1
labtether_clear_token_environment
unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_API_BASE_URL EARLY_CA_FILE EARLY_INSECURE_TLS
unset SOURCED_API_TOKEN SOURCED_OWNER_TOKEN SOURCED_API_BASE_URL SOURCED_CA_FILE SOURCED_INSECURE_TLS
trap labtether_cleanup_curl_security EXIT

if ! require_command curl || ! require_command jq || ! require_command od; then
  exit 1
fi

detect_tls_redirect() {
  [[ "${API_BASE}" == http://* ]] || return 0
  local health_probe=""
  labtether_build_curl_request_args "${API_BASE}/healthz" 0 || return 1
  health_probe="$(labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sS --connect-timeout 2 --max-time 5 "${API_BASE}/healthz" 2>/dev/null || true)"
  if [[ "${health_probe}" == *'"status":"redirect_active"'* ]]; then
    local base_no_scheme="${API_BASE#http://}"
    local host_port="${base_no_scheme%%/*}"
    local host="${host_port%%:*}"
    local redirect_port=""
    redirect_port="$(printf '%s' "${health_probe}" | sed -n 's/.*https on port \([0-9][0-9]*\).*/\1/p')"
    if [[ -z "${redirect_port}" ]]; then
      redirect_port=8443
    fi
    API_BASE="https://${host}:${redirect_port}"
    echo "Detected API HTTP redirect mode; switching integration API base to ${API_BASE}"
  fi
}

detect_tls_redirect

compose_cmd=()
compose_args=()
resolve_compose_cmd() {
  compose_cmd=()
  if has_command docker; then
    if docker compose version >/dev/null 2>&1; then
      compose_cmd=(docker compose --env-file "$ENV_FILE" -p "$INTEGRATION_COMPOSE_PROJECT")
    elif has_command docker-compose; then
      compose_cmd=(docker-compose --env-file "$ENV_FILE" -p "$INTEGRATION_COMPOSE_PROJECT")
    fi
  fi
}

smoke_init_cleanup

wait_for_http() {
  local url=$1
  local timeout=${2:-$TIMEOUT_SECONDS}
  local deadline=$((SECONDS + timeout))

  while [[ ${SECONDS} -lt ${deadline} ]]; do
    if check_http_status "${url}" 200 2; then
      return 0
    fi
    sleep 1
  done

  return 1
}

should_manage_stack="false"
case "$(printf '%s' "${INTEGRATION_MANAGE_STACK}" | tr '[:upper:]' '[:lower:]')" in
  true|1|yes)
    should_manage_stack="true"
    ;;
  auto|"")
    if [[ "${API_BASE}" =~ ^https?://(localhost|127\.0\.0\.1)(:[0-9]+)?(/|$) ]]; then
      should_manage_stack="true"
    fi
    ;;
esac

stack_started="false"

cleanup_stack() {
  if [[ "${stack_started}" == "true" ]]; then
    (cd "${PROJECT_ROOT}" && "${compose_cmd[@]}" down -v >/dev/null 2>&1)
  fi
}

cleanup() {
  local original_rc=$?
  trap - EXIT INT TERM HUP
  set +e
  local cleanup_failed=0
  smoke_run_cleanup || cleanup_failed=1
  cleanup_stack || cleanup_failed=1
  labtether_cleanup_curl_security || cleanup_failed=1
  if [[ "$cleanup_failed" == "1" ]]; then
    original_rc=1
  fi
  exit "$original_rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 129' HUP
trap 'exit 143' TERM

prepare_owned_ca() {
  if [[ "$stack_started" != "true" || "$API_BASE" != https://* || -n "${LABTETHER_CA_FILE:-}" ]]; then
    return 0
  fi
  local container_id=""
  local attempt
  for ((attempt = 0; attempt < 20; attempt++)); do
    container_id=$(cd "$PROJECT_ROOT" && "${compose_cmd[@]}" ps -q labtether 2>/dev/null || true)
    [[ -n "$container_id" ]] && break
    sleep 1
  done
  if [[ -z "$container_id" ]]; then
    echo "unable to resolve owned LabTether container for CA acquisition" >&2
    return 1
  fi
  labtether_ensure_secure_curl_dir || return 1
  local owned_ca="${LABTETHER_SECURE_CURL_DIR}/owned-compose-ca.crt"
  local copied=0
  for ((attempt = 0; attempt < 20; attempt++)); do
    if docker cp "${container_id}:/ca/ca.crt" "$owned_ca" >/dev/null 2>&1 && [[ -s "$owned_ca" ]]; then
      copied=1
      break
    fi
    sleep 1
  done
  [[ "$copied" == "1" ]] || {
    echo "unable to acquire owned compose CA; provide --ca-file" >&2
    return 1
  }
  chmod 600 "$owned_ca"
  LABTETHER_CA_FILE="$owned_ca"
}

if ! wait_for_http "${API_BASE}/healthz" 8; then
  if [[ "${should_manage_stack}" == "true" ]]; then
    resolve_compose_cmd
    if [[ ${#compose_cmd[@]} -eq 0 ]]; then
      echo "Docker compose not available (install docker with compose plugin or docker-compose)" >&2
      exit 1
    fi

    read -r -a compose_args <<< "${INTEGRATION_COMPOSE_UP_ARGS}"
    echo "API not reachable at ${API_BASE}; starting compose stack"
    (cd "${PROJECT_ROOT}" && "${compose_cmd[@]}" up "${compose_args[@]}")
    stack_started="true"
    for _redirect_attempt in $(seq 1 30); do
      detect_tls_redirect
      [[ "$API_BASE" == https://* ]] && break
      sleep 1
    done
    prepare_owned_ca || exit 1
  fi
fi

if ! wait_for_http "${API_BASE}/healthz" "${TIMEOUT_SECONDS}"; then
  echo "API not reachable at ${API_BASE} within ${TIMEOUT_SECONDS}s" >&2
  exit 1
fi

extract_json_string() {
  local key=$1
  local json=$2
  printf '%s' "${json}" | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" | head -n 1
}

extract_command_status() {
  local command_id=$1
  local json=$2
  printf '%s' "${json}" | tr -d '\n' | sed -n "s/.*\"id\"[[:space:]]*:[[:space:]]*\"${command_id}\"[^}]*\"status\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" | head -n 1
}

wait_for_status() {
  local url=$1
  local expect=${2:-succeeded}
  local deadline=$((SECONDS + TIMEOUT_SECONDS))

  while [[ ${SECONDS} -lt ${deadline} ]]; do
    local body status current
    run_request body status GET "$url"
    if [[ "${status}" == "200" ]]; then
      current=$(extract_json_string "status" "${body}")
      if [[ "${current}" == "${expect}" ]]; then
        return 0
      fi
      if [[ "${current}" == "failed" ]]; then
        echo "run failed at ${url}" >&2
        echo "${body}" >&2
        return 1
      fi
    fi
    sleep 1
  done

  echo "timeout waiting for ${url}" >&2
  return 1
}

wait_for_command_status() {
  local session_id=$1
  local command_id=$2
  local expect=${3:-succeeded}
  local url="${API_BASE}/terminal/sessions/${session_id}/commands"
  local deadline=$((SECONDS + TIMEOUT_SECONDS))

  while [[ ${SECONDS} -lt ${deadline} ]]; do
    local body status current
    run_request body status GET "${url}"
    if [[ "${status}" == "200" ]]; then
      current=$(extract_command_status "${command_id}" "${body}")
      if [[ "${current}" == "${expect}" ]]; then
        return 0
      fi
      if [[ "${current}" == "failed" ]]; then
        echo "command failed for ${command_id}" >&2
        echo "${body}" >&2
        return 1
      fi
    fi
    sleep 1
  done

  echo "timeout waiting for command ${command_id} in session ${session_id}" >&2
  return 1
}

body=""
status=""
session_payload=$(jq -cn --arg target "$INTEGRATION_TARGET" '{actor_id:"owner",target:$target,mode:"interactive"}')
run_request body status POST "${API_BASE}/terminal/sessions" "$session_payload"
[[ "${status}" == "201" ]] || { echo "session create failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
session_id=$(extract_json_string "id" "${body}")
smoke_register_cleanup DELETE "/terminal/sessions/${session_id}" "terminal session ${session_id}"

run_request body status POST "${API_BASE}/terminal/sessions/${session_id}/commands" '{"actor_id":"owner","command":"uname -a"}'
[[ "${status}" == "202" ]] || { echo "command enqueue failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
command_id=$(extract_json_string "id" "${body}")
wait_for_command_status "${session_id}" "${command_id}" "succeeded" || exit 1

action_payload=$(jq -cn --arg target "$INTEGRATION_TARGET" '{type:"command",actor_id:"owner",target:$target,command:"uptime"}')
run_request body status POST "${API_BASE}/actions/execute" "$action_payload"
[[ "${status}" == "202" ]] || { echo "action enqueue failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
action_run_id=$(extract_json_string "id" "${body}")
if [[ -z "${action_run_id}" ]]; then
  action_run_id=$(extract_json_string "run_id" "${body}")
fi
if [[ -z "${action_run_id}" ]]; then
  action_run_id=$(extract_nested_json_id "run" "${body}")
fi
if [[ -n "${action_run_id}" ]]; then
  smoke_register_cleanup DELETE "/actions/runs/${action_run_id}" "action run ${action_run_id}"
else
  echo "action run id missing from response" >&2
  echo "${body}" >&2
  exit 1
fi
wait_for_status "${API_BASE}/actions/runs/${action_run_id}" "succeeded" || exit 1

plan_payload=$(jq -cn \
  --arg name "Integration Plan ${INTEGRATION_RUN_TOKEN}" \
  --arg target "$INTEGRATION_TARGET" \
  '{name:$name,targets:[$target],scopes:["os_packages"],default_dry_run:true}')
run_request body status POST "${API_BASE}/updates/plans" "$plan_payload"
[[ "${status}" == "201" ]] || { echo "update plan create failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
plan_id=$(extract_json_string "id" "${body}")
smoke_register_cleanup DELETE "/updates/plans/${plan_id}" "update plan ${plan_id}"

run_request body status POST "${API_BASE}/updates/plans/${plan_id}/execute" '{"actor_id":"owner","dry_run":true}'
[[ "${status}" == "202" ]] || { echo "update execute failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
update_run_id=$(extract_json_string "id" "${body}")
if [[ -z "${update_run_id}" ]]; then
  update_run_id=$(extract_nested_json_id "run" "${body}")
fi
if [[ -n "${update_run_id}" ]]; then
  smoke_register_cleanup DELETE "/updates/runs/${update_run_id}" "update run ${update_run_id}"
else
  echo "update run id missing from response" >&2
  echo "${body}" >&2
  exit 1
fi
wait_for_status "${API_BASE}/updates/runs/${update_run_id}" "succeeded" || exit 1

echo "integration queue flow passed"
