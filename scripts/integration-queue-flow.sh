#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-45}"
INTEGRATION_MANAGE_STACK="${INTEGRATION_MANAGE_STACK:-auto}"
INTEGRATION_COMPOSE_UP_ARGS="${INTEGRATION_COMPOSE_UP_ARGS:--d}"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/smoke-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

if [[ -f "${ENV_FILE}" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "${ENV_FILE}"
  set +a
fi

API_BASE="${LABTETHER_API_BASE_URL:-http://localhost:8080}"
AUTH_TOKEN="${LABTETHER_API_TOKEN:-${LABTETHER_OWNER_TOKEN:-}}"
INTEGRATION_RUN_TOKEN="$(date +%s)-$RANDOM"

if [[ -z "${AUTH_TOKEN}" ]]; then
  echo "missing LABTETHER_API_TOKEN or LABTETHER_OWNER_TOKEN" >&2
  exit 1
fi
if ! require_command curl; then
  exit 1
fi

# dev-backend defaults to TLS auto mode and returns a redirect hint on HTTP /healthz.
if [[ "${API_BASE}" == http://* ]]; then
  health_probe="$(curl -sS --max-time 5 "${API_BASE}/healthz" 2>/dev/null || true)"
  if [[ "${health_probe}" == *'"status":"redirect_active"'* ]]; then
    base_no_scheme="${API_BASE#http://}"
    host_port="${base_no_scheme%%/*}"
    host="${host_port%%:*}"
    redirect_port="$(printf '%s' "${health_probe}" | sed -n 's/.*https on port \([0-9][0-9]*\).*/\1/p')"
    if [[ -z "${redirect_port}" ]]; then
      redirect_port=8443
    fi
    API_BASE="https://${host}:${redirect_port}"
    echo "Detected API HTTP redirect mode; switching integration API base to ${API_BASE}"
  fi
fi

compose_cmd=()
compose_args=()
resolve_compose_cmd() {
  compose_cmd=()
  if has_command docker; then
    if docker compose version >/dev/null 2>&1; then
      compose_cmd=(docker compose)
    elif has_command docker-compose; then
      compose_cmd=(docker-compose)
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
    (cd "${PROJECT_ROOT}" && "${compose_cmd[@]}" down >/dev/null 2>&1 || true)
  fi
}

cleanup() {
  smoke_run_cleanup
  cleanup_stack
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
  fi
fi

if ! wait_for_http "${API_BASE}/healthz" "${TIMEOUT_SECONDS}"; then
  echo "API not reachable at ${API_BASE} within ${TIMEOUT_SECONDS}s" >&2
  exit 1
fi

run_request() {
  local __body_var=$1
  local __status_var=$2
  local method=$3
  local url=$4
  local payload=${5:-}

  local -a args=("-sS" "-w" $'\n%{http_code}' -X "$method" "$url" -H "Authorization: Bearer ${AUTH_TOKEN}")
  if [[ -n "${payload}" ]]; then
    args+=( -H "Content-Type: application/json" --data "${payload}" )
  fi

  local response
  if [[ "${url}" == https://* ]]; then
    args=("-k" "${args[@]}")
  fi
  response=$(curl "${args[@]}" || true)
  local resp_status
  resp_status=$(printf '%s' "${response}" | tail -n 1)
  local resp_body
  resp_body=$(printf '%s' "${response}" | sed '$d')
  if [[ -z "${resp_status}" ]]; then
    resp_status="000"
  fi

  printf -v "$__body_var" '%s' "$resp_body"
  printf -v "$__status_var" '%s' "$resp_status"
}

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

trap cleanup EXIT

body=""
status=""
run_request body status POST "${API_BASE}/terminal/sessions" '{"actor_id":"owner","target":"lab-host-01","mode":"interactive"}'
[[ "${status}" == "201" ]] || { echo "session create failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
session_id=$(extract_json_string "id" "${body}")
smoke_register_cleanup DELETE "/terminal/sessions/${session_id}" "terminal session ${session_id}"

run_request body status POST "${API_BASE}/terminal/sessions/${session_id}/commands" '{"actor_id":"owner","command":"uname -a"}'
[[ "${status}" == "202" ]] || { echo "command enqueue failed: ${status}" >&2; echo "${body}" >&2; exit 1; }
command_id=$(extract_json_string "id" "${body}")
wait_for_command_status "${session_id}" "${command_id}" "succeeded" || exit 1

run_request body status POST "${API_BASE}/actions/execute" '{"type":"command","actor_id":"owner","target":"lab-host-01","command":"uptime"}'
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

run_request body status POST "${API_BASE}/updates/plans" "{\"name\":\"Integration Plan ${INTEGRATION_RUN_TOKEN}\",\"targets\":[\"lab-host-01\"],\"scopes\":[\"os_packages\",\"docker_images\"],\"default_dry_run\":true}"
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
