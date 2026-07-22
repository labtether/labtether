#!/usr/bin/env bash
# LabTether backend profile capture.
#
# Works against the current auth-protected DEV_MODE pprof setup by supporting:
# - bearer token auth, or
# - session login to the backend
#
# Usage:
#   ./scripts/perf/profile.sh
#   ./scripts/perf/profile.sh 20 --api-base https://127.0.0.1:8443

set -euo pipefail
set +x
set +a
umask 077

unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_ADMIN_PASSWORD AUTH_TOKEN LOGIN_PASSWORD
EARLY_API_TOKEN="${LABTETHER_API_TOKEN-}"
EARLY_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"
EARLY_ADMIN_PASSWORD="${LABTETHER_ADMIN_PASSWORD-}"
unset LABTETHER_API_TOKEN LABTETHER_OWNER_TOKEN LABTETHER_ADMIN_PASSWORD

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

for cmd in curl date grep ls mktemp ps sed; do
  if ! require_command "${cmd}"; then
    exit 1
  fi
done

usage() {
  cat <<'USAGE'
LabTether backend profile capture

Usage:
  ./scripts/perf/profile.sh [duration_seconds] [options]

Options:
  --api-base URL         Backend API origin (auto-detect by default)
  --token-file PATH      Read bearer token from a mode-0600 file
  --username USER        Login username when bearer token is not provided (default: admin)
  --password-file PATH   Read login password from a mode-0600 file
  --output-root DIR      Artifact root (default: the per-user temporary directory)
  --ca-file PATH         Verify HTTPS with this CA bundle
  -h, --help             Show help

Environment fallbacks:
  LABTETHER_API_BASE_URL
  LABTETHER_API_TOKEN / LABTETHER_OWNER_TOKEN
  LABTETHER_ADMIN_USERNAME / LABTETHER_ADMIN_PASSWORD
USAGE
}

DURATION="${1:-30}"
if (($# > 0)) && [[ "${1:-}" != --* ]]; then
  shift
fi

API_BASE="${LABTETHER_API_BASE_URL:-}"
AUTH_TOKEN="${EARLY_API_TOKEN:-${EARLY_OWNER_TOKEN:-}}"
LOGIN_USERNAME="${LABTETHER_ADMIN_USERNAME:-admin}"
LOGIN_PASSWORD="${EARLY_ADMIN_PASSWORD:-password}"
unset EARLY_API_TOKEN EARLY_OWNER_TOKEN EARLY_ADMIN_PASSWORD
OUTPUT_ROOT="${TMPDIR:-/tmp}"
CLI_CA_FILE=""
TOKEN_FILE=""
PASSWORD_FILE=""

while (($# > 0)); do
  case "$1" in
    --api-base)
      API_BASE="${2:-}"
      shift 2
      ;;
    --token-file)
      TOKEN_FILE="${2:-}"
      shift 2
      ;;
    --username)
      LOGIN_USERNAME="${2:-}"
      shift 2
      ;;
    --password-file)
      PASSWORD_FILE="${2:-}"
      shift 2
      ;;
    --token|--password)
      log_fail "$1 is disabled because secret values must not be passed in process arguments; use the corresponding -file option or environment fallback"
      exit 1
      ;;
    --output-root)
      OUTPUT_ROOT="${2:-}"
      shift 2
      ;;
    --ca-file)
      CLI_CA_FILE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log_fail "unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -n "$TOKEN_FILE" ]]; then
  labtether_read_private_secret_file AUTH_TOKEN "$TOKEN_FILE" "bearer token file" || exit 1
fi
if [[ -n "$PASSWORD_FILE" ]]; then
  labtether_read_private_secret_file LOGIN_PASSWORD "$PASSWORD_FILE" "login password file" || exit 1
fi

if [[ -n "$CLI_CA_FILE" ]]; then
  # Read by the sourced script-common request helper.
  # shellcheck disable=SC2034
  LABTETHER_CA_FILE="$CLI_CA_FILE"
fi
labtether_validate_tls_options || exit 1
if labtether_value_is_true "${LABTETHER_INSECURE_TLS:-0}"; then
  log_fail "authenticated profiling refuses disabled TLS verification; use --ca-file or OS trust"
  exit 1
fi
if [[ -n "$AUTH_TOKEN" ]]; then
  labtether_prepare_curl_auth "$AUTH_TOKEN" || exit 1
fi
labtether_clear_token_environment
export -n LOGIN_PASSWORD 2>/dev/null || true
unset LABTETHER_ADMIN_PASSWORD
trap labtether_cleanup_curl_security EXIT

if ! [[ "${DURATION}" =~ ^[0-9]+$ ]] || [[ "${DURATION}" -le 0 ]]; then
  log_fail "duration must be a positive integer"
  exit 1
fi

trimmed_or_empty() {
  local value=${1:-}
  printf '%s' "${value#"${value%%[![:space:]]*}"}" | sed 's/[[:space:]]*$//'
}

is_backend_reachable() {
  local url=$1
  local status
  labtether_build_curl_request_args "${url}/healthz" 0 || return 1
  status="$(labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sS --connect-timeout 3 --max-time 8 -o /dev/null -w '%{http_code}' "${url}/healthz" || true)"
  [[ "${status}" == "200" ]]
}

auto_pick_backend_base() {
  local candidate=""
  for candidate in "$@"; do
    if [[ -z "${candidate}" ]]; then
      continue
    fi
    if is_backend_reachable "${candidate}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 1
}

API_BASE="$(trimmed_or_empty "${API_BASE}")"
if [[ -z "${API_BASE}" ]]; then
  API_BASE="$(auto_pick_backend_base \
    "http://127.0.0.1:8080" \
    "http://localhost:8080" \
    "https://127.0.0.1:8443" \
    "https://localhost:8443" || true)"
fi
if [[ -z "${API_BASE}" ]]; then
  log_fail "could not auto-detect a reachable backend base; pass --api-base"
  exit 1
fi

OUTDIR=""
labtether_make_private_run_dir OUTDIR "$OUTPUT_ROOT" "labtether-profile-$(date +%Y%m%d-%H%M%S)" || exit 1

COOKIE_JAR=""
cleanup() {
  if [[ -n "${COOKIE_JAR}" && -f "${COOKIE_JAR}" ]]; then
    rm -f "${COOKIE_JAR}"
  fi
  labtether_cleanup_curl_security
}
trap cleanup EXIT

login_backend() {
  local cookie_jar=$1
  local response_body
  response_body="$(mktemp)"
  local payload=""
  labtether_build_login_json payload "${LOGIN_USERNAME}" "${LOGIN_PASSWORD}" || exit 1
  local code
  labtether_build_curl_request_args "${API_BASE}/auth/login" 0 1 || exit 1
  code="$(labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sS --connect-timeout 5 --max-time 30 \
    -c "${cookie_jar}" \
    -o "${response_body}" \
    -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -X POST \
    --data-binary @- \
    "${API_BASE}/auth/login" <<<"${payload}" || true)"
  if [[ "${code}" != "200" ]]; then
    log_fail "backend login failed (${API_BASE}/auth/login -> ${code})"
    cat "${response_body}" > "${OUTDIR}/login-error.json"
    rm -f "${response_body}"
    exit 1
  fi
  if grep -q '"requires_2fa":true' "${response_body}" 2>/dev/null; then
    log_fail "backend login requires 2FA; use --token-file or a local non-2FA baseline account"
    cat "${response_body}" > "${OUTDIR}/login-error.json"
    rm -f "${response_body}"
    exit 1
  fi
  rm -f "${response_body}"
}

AUTH_ARGS=()
if [[ -n "${AUTH_TOKEN}" ]]; then
  labtether_build_curl_request_args "${API_BASE}/healthz" 1 || exit 1
  AUTH_ARGS=("${LABTETHER_CURL_REQUEST_ARGS[@]}")
  log_info "Auth mode: bearer token"
else
  COOKIE_JAR="$(mktemp)"
  login_backend "${COOKIE_JAR}"
  labtether_build_curl_request_args "${API_BASE}/healthz" 0 || exit 1
  AUTH_ARGS=("${LABTETHER_CURL_REQUEST_ARGS[@]}" -b "${COOKIE_JAR}")
  log_info "Auth mode: session login (${LOGIN_USERNAME})"
fi

log_info "=== LabTether Profile Capture ==="
log_info "Duration: ${DURATION}s"
log_info "API base: ${API_BASE}"
log_info "Output: ${OUTDIR}"
log_info ""

health_code="$(labtether_curl "${AUTH_ARGS[@]}" -sS --connect-timeout 5 --max-time 30 -o /dev/null -w '%{http_code}' "${API_BASE}/healthz" || true)"
if [[ "${health_code}" != "200" ]]; then
  log_fail "health check failed (${API_BASE}/healthz -> ${health_code})"
  exit 1
fi

pprof_code="$(labtether_curl "${AUTH_ARGS[@]}" -sS --connect-timeout 5 --max-time 30 -o /dev/null -w '%{http_code}' "${API_BASE}/debug/pprof/" || true)"
if [[ "${pprof_code}" != "200" ]]; then
  log_fail "pprof endpoint unavailable (${API_BASE}/debug/pprof/ -> ${pprof_code})"
  log_fail "ensure the backend is running in DEV_MODE and that auth is valid"
  exit 1
fi

log_info "pprof available; capturing profiles..."
labtether_curl "${AUTH_ARGS[@]}" -sS --connect-timeout 5 --max-time 60 "${API_BASE}/debug/pprof/goroutine?debug=1" > "${OUTDIR}/goroutines.txt" &
labtether_curl "${AUTH_ARGS[@]}" -sS --connect-timeout 5 --max-time 60 "${API_BASE}/debug/pprof/heap" > "${OUTDIR}/heap.pb.gz" &
labtether_curl "${AUTH_ARGS[@]}" -sS --connect-timeout 5 --max-time "$((DURATION + 30))" "${API_BASE}/debug/pprof/profile?seconds=${DURATION}" > "${OUTDIR}/cpu.pb.gz" &
log_info "CPU profile will take ${DURATION}s..."
wait
log_pass "Profiles saved."

{
  echo "=== Sample at $(date -Iseconds) ==="
  # ps provides the full resource snapshot needed in the profiling artifact.
  # shellcheck disable=SC2009
  ps aux | grep -E '(labtether|node.*next)' | grep -v grep || true
} > "${OUTDIR}/process-sample.txt"

log_info ""
log_info "=== Output files ==="
ls -la "${OUTDIR}"/
log_info ""
log_info "Done. View with:"
log_info "  cat ${OUTDIR}/goroutines.txt        # goroutine dump"
log_info "  go tool pprof ${OUTDIR}/cpu.pb.gz   # CPU profile"
log_info "  go tool pprof ${OUTDIR}/heap.pb.gz  # heap profile"
