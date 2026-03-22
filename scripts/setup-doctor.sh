#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"

RUN_PREFLIGHT=1
RUN_RUNTIME=1

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/setup-doctor.sh [options]

Validate LabTether setup readiness and runtime health.

Options:
  --preflight    Run dependency/env checks only.
  --runtime      Run runtime/health checks only.
  -h, --help     Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --preflight)
      RUN_RUNTIME=0
      ;;
    --runtime)
      RUN_PREFLIGHT=0
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

FAILURES=0
WARNINGS=0

pass() { log_pass "$1"; }
warn() { log_info "WARN: $1"; WARNINGS=$((WARNINGS + 1)); }
fail() { log_fail "$1"; FAILURES=$((FAILURES + 1)); }

compose_cmd=()
resolve_compose_cmd() {
  compose_cmd=()
  if has_command docker; then
    if docker compose version >/dev/null 2>&1; then
      compose_cmd=(docker compose -f "${PROJECT_ROOT}/docker-compose.yml")
    elif has_command docker-compose; then
      compose_cmd=(docker-compose -f "${PROJECT_ROOT}/docker-compose.yml")
    fi
  fi
}

read_env_value() {
  local key=$1
  local line
  line=$(grep -E "^${key}=" "${ENV_FILE}" | head -n 1 || true)
  printf '%s' "${line#*=}"
}

is_placeholder() {
  local value=$1
  [[ -z "${value// }" || "${value}" == REPLACE_WITH_* ]]
}

http_code() {
  local url=$1
  if [[ "${url}" == https://* ]]; then
    curl -k -sS -o /dev/null -w "%{http_code}" --max-time 6 "${url}" || true
  else
    curl -sS -o /dev/null -w "%{http_code}" --max-time 6 "${url}" || true
  fi
}

check_port_conflict() {
  local port=$1
  if ! nc -z localhost "${port}" 2>/dev/null; then
    pass "port ${port} is available"
    return 0
  fi

  local conflicts
  conflicts=$(pgrep -a -f "labtether|next-server|next dev|node.*${port}" 2>/dev/null | while read -r pid cmd_line; do
    local cmd
    cmd=$(ps -o comm= -p "${pid}" 2>/dev/null || true)
    if [[ "${cmd}" != "docker" && "${cmd}" != "com.docker.b" && "${cmd}" != "vpnkit-bridg" && "${cmd}" != "ssh" ]]; then
      echo "${pid}:${cmd}"
    fi
  done)

  if [[ -n "${conflicts}" ]]; then
    fail "port ${port} is in use by non-Docker process(es): ${conflicts}"
  else
    pass "port ${port} is available or Docker-owned"
  fi
}

if [[ "${RUN_PREFLIGHT}" -eq 1 ]]; then
  log_info "=== LabTether Setup Doctor: preflight ==="

  if require_command docker; then
    pass "docker binary found"
  else
    fail "docker is required"
  fi

  if require_command curl; then
    pass "curl binary found"
  else
    fail "curl is required"
  fi
  if require_command openssl; then
    pass "openssl binary found"
  else
    fail "openssl is required"
  fi

  resolve_compose_cmd
  if [[ ${#compose_cmd[@]} -eq 0 ]]; then
    fail "docker compose command not found"
  else
    pass "docker compose command available"
  fi

  if docker info >/dev/null 2>&1; then
    pass "docker daemon is reachable"
  else
    fail "docker daemon is not reachable"
  fi

  if [[ -f "${ENV_FILE}" ]]; then
    pass "env file found at ${ENV_FILE}"
  else
    warn "env file not found at ${ENV_FILE}; release deploy artifacts may not need one"
  fi

  if [[ -f "${ENV_FILE}" ]]; then
    admin_password="$(read_env_value "LABTETHER_ADMIN_PASSWORD")"
    if [[ -z "${admin_password// }" ]]; then
      pass "LABTETHER_ADMIN_PASSWORD omitted; website setup flow will create the first admin"
    elif is_placeholder "${admin_password}" ; then
      fail "LABTETHER_ADMIN_PASSWORD is still placeholder"
    else
      pass "LABTETHER_ADMIN_PASSWORD is configured"
    fi
  fi

  check_port_conflict 3000
  check_port_conflict 8080
  check_port_conflict 8443
fi

if [[ "${RUN_RUNTIME}" -eq 1 ]]; then
  log_info "=== LabTether Setup Doctor: runtime ==="

  resolve_compose_cmd
  if [[ ${#compose_cmd[@]} -gt 0 ]]; then
    running_services=$("${compose_cmd[@]}" ps --services --filter status=running 2>/dev/null || true)
    if [[ -n "${running_services}" ]]; then
      pass "compose services running: $(echo "${running_services}" | tr '\n' ' ')"
    else
      warn "no running compose services detected"
    fi
  else
    warn "docker compose command unavailable; skipping compose status check"
  fi

  api_health_code="$(http_code "http://localhost:8080/healthz")"
  if [[ "${api_health_code}" == "200" ]]; then
    pass "API health endpoint reachable over HTTP"
  else
    # Auto-TLS mode may return redirect metadata; check HTTPS as fallback.
    api_tls_health_code="$(http_code "https://localhost:8443/healthz")"
    if [[ "${api_tls_health_code}" == "200" ]]; then
      pass "API health endpoint reachable over HTTPS"
    else
      fail "API health check failed (http:${api_health_code} https:${api_tls_health_code})"
    fi
  fi

  console_https_code="$(http_code "https://localhost:3000")"
  console_http_code="$(http_code "http://localhost:3000")"
  if [[ "${console_https_code}" == "200" || "${console_http_code}" == "200" ]]; then
    pass "web console endpoint reachable"
  else
    fail "web console check failed (https:${console_https_code} http:${console_http_code})"
  fi

  smoke_script="${PROJECT_ROOT}/scripts/smoke-test.sh"
  if [[ -x "${smoke_script}" ]]; then
    pass "smoke test script available"
  else
    warn "smoke test script is not executable: ${smoke_script}"
  fi
fi

log_info ""
if [[ "${FAILURES}" -eq 0 ]]; then
  pass "setup doctor finished with no failures"
  if [[ "${WARNINGS}" -gt 0 ]]; then
    warn "${WARNINGS} warning(s) detected"
  fi
  exit 0
fi

fail "${FAILURES} failure(s) detected"
if [[ "${WARNINGS}" -gt 0 ]]; then
  warn "${WARNINGS} warning(s) detected"
fi
exit 1
