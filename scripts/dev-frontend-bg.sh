#!/usr/bin/env bash
set -euo pipefail
set +x
set +a
umask 077

SESSION_NAME="${LABTETHER_FRONTEND_TMUX_SESSION:-labtether-frontend}"
LOG_FILE="${LABTETHER_FRONTEND_LOG_FILE:-${TMPDIR:-/tmp}/labtether-dev-frontend.log}"
RESTART_SESSION=0
STOP_SESSION=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/dev-runtime-warnings.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/dev-frontend-bg.sh [options]

Start the LabTether web console in a tmux session.

Options:
  --restart     Restart the tmux session if already running.
  --stop        Stop the tmux session and exit.
  -h, --help    Show this help.

Environment:
  LABTETHER_FRONTEND_TMUX_SESSION  Session name (default: labtether-frontend)
  LABTETHER_FRONTEND_LOG_FILE      Private log path (default: the per-user temporary directory)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --restart)
      RESTART_SESSION=1
      ;;
    --stop)
      STOP_SESSION=1
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

if ! require_command tmux; then
  log_info "Install tmux, or run: cd web/console && npm run dev"
  exit 1
fi

emit_dev_runtime_warnings

if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
  if [[ "${STOP_SESSION}" -eq 1 || "${RESTART_SESSION}" -eq 1 ]]; then
    tmux kill-session -t "${SESSION_NAME}"
    log_info "Stopped tmux session '${SESSION_NAME}'."
    if [[ "${STOP_SESSION}" -eq 1 ]]; then
      exit 0
    fi
  else
    log_info "tmux session '${SESSION_NAME}' already exists."
    log_info "Attach: tmux attach -t ${SESSION_NAME}"
    exit 0
  fi
elif [[ "${STOP_SESSION}" -eq 1 ]]; then
  log_info "tmux session '${SESSION_NAME}' is not running."
  exit 0
fi

if [[ "${RESTART_SESSION}" -eq 1 ]]; then
  log_info "Restarting frontend in tmux session '${SESSION_NAME}'..."
else
  log_info "Starting frontend in new tmux session '${SESSION_NAME}'..."
fi

if [[ -e "$LOG_FILE" || -L "$LOG_FILE" ]]; then
  labtether_lock_down_private_file "$LOG_FILE" "frontend log file" || exit 1
else
  labtether_prepare_private_output_file "$LOG_FILE" || exit 1
fi

# --- TLS auto-detection ---
# When LABTETHER_API_BASE_URL is already set, skip probing (explicit override wins).
DATA_DIR="${LABTETHER_DATA_DIR:-data}"
CA_CERT="${PROJECT_ROOT}/${DATA_DIR}/certs/ca.crt"
SERVER_CERT="${PROJECT_ROOT}/${DATA_DIR}/certs/server.crt"
SERVER_KEY="${PROJECT_ROOT}/${DATA_DIR}/certs/server.key"
NEXT_HTTPS_FLAGS=()

PROBE_HTTP_PORT="${LABTETHER_HTTP_PORT:-8080}"
if [[ ! "$PROBE_HTTP_PORT" =~ ^[0-9]+$ ]] || ((PROBE_HTTP_PORT < 1 || PROBE_HTTP_PORT > 65535)); then
  log_fail "LABTETHER_HTTP_PORT must be an integer between 1 and 65535"
  exit 1
fi

if [ -n "${LABTETHER_API_BASE_URL:-}" ]; then
  log_info "LABTETHER_API_BASE_URL already set: ${LABTETHER_API_BASE_URL} (skipping probe)"
else
  # Probe the backend's TLS info endpoint over plain HTTP.
  PROBE_URL="http://localhost:${PROBE_HTTP_PORT}/api/v1/tls/info"
  PROBE_JSON=""
  if command -v curl >/dev/null 2>&1; then
    labtether_build_curl_request_args "$PROBE_URL" 0 || exit 1
    PROBE_JSON=$(labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" -sf --max-filesize 1048576 --connect-timeout 2 --max-time 2 "${PROBE_URL}" 2>/dev/null || true)
  fi

  if [ -n "${PROBE_JSON}" ]; then
    # Parse JSON fields. Prefer jq, fall back to python3.
    _parse_json_field() {
      local json="$1" field="$2"
      local key="${field#.}"  # strip leading dot: .tls_source -> tls_source
      if command -v jq >/dev/null 2>&1; then
        echo "${json}" | jq -r "${field} // empty" 2>/dev/null
      elif command -v python3 >/dev/null 2>&1; then
        echo "${json}" | python3 -c "import sys,json; d=json.load(sys.stdin); v=d.get('${key}',''); print(v if v else '')" 2>/dev/null || true
      fi
    }

    TLS_SOURCE=$(_parse_json_field "${PROBE_JSON}" ".tls_source")
    PROBE_HTTPS_PORT=$(_parse_json_field "${PROBE_JSON}" ".https_port")
    PROBE_HTTP_PORT_VAL=$(_parse_json_field "${PROBE_JSON}" ".http_port")
    # cert_dns_names is an array; grab the first entry.
    CERT_HOSTNAME=""
    if command -v jq >/dev/null 2>&1; then
      CERT_HOSTNAME=$(echo "${PROBE_JSON}" | jq -r '.cert_dns_names[0] // empty' 2>/dev/null)
    elif command -v python3 >/dev/null 2>&1; then
      CERT_HOSTNAME=$(echo "${PROBE_JSON}" | python3 -c "import sys,json; d=json.load(sys.stdin); n=d.get('cert_dns_names',[]); print(n[0] if n else '')" 2>/dev/null || true)
    fi

    case "${TLS_SOURCE}" in
      tailscale|deployment_external|ui_uploaded|disabled|built_in) ;;
      *) TLS_SOURCE="built_in" ;;
    esac
    for candidate_port_name in PROBE_HTTPS_PORT PROBE_HTTP_PORT_VAL; do
      candidate_port=${!candidate_port_name:-}
      if [[ -n "$candidate_port" ]] && { [[ ! "$candidate_port" =~ ^[0-9]+$ ]] || ((candidate_port < 1 || candidate_port > 65535)); }; then
        log_warn "Ignoring invalid ${candidate_port_name} from the local TLS probe"
        printf -v "$candidate_port_name" '%s' ''
      fi
    done
    if [[ -n "$CERT_HOSTNAME" && ! "$CERT_HOSTNAME" =~ ^[A-Za-z0-9._:-]+$ ]]; then
      log_warn "Ignoring invalid certificate hostname from the local TLS probe"
      CERT_HOSTNAME=""
    fi

    log_info "Backend TLS probe: source=${TLS_SOURCE:-unknown}, hostname=${CERT_HOSTNAME:-localhost}, https_port=${PROBE_HTTPS_PORT:-?}, http_port=${PROBE_HTTP_PORT_VAL:-?}"

    case "${TLS_SOURCE}" in
      tailscale)
        unset NODE_EXTRA_CA_CERTS
        export LABTETHER_API_BASE_URL="https://${CERT_HOSTNAME}:${PROBE_HTTPS_PORT}"
        export NEXT_PUBLIC_HUB_API_PORT="${PROBE_HTTPS_PORT}"
        log_info "TLS source: Tailscale (${CERT_HOSTNAME}) — no CA trust needed"
        ;;
      deployment_external|ui_uploaded)
        unset NODE_EXTRA_CA_CERTS
        CERT_HOST="${CERT_HOSTNAME:-localhost}"
        export LABTETHER_API_BASE_URL="https://${CERT_HOST}:${PROBE_HTTPS_PORT}"
        export NEXT_PUBLIC_HUB_API_PORT="${PROBE_HTTPS_PORT}"
        log_info "TLS source: ${TLS_SOURCE} (${CERT_HOST}) — assuming OS-trusted"
        ;;
      disabled)
        unset NODE_EXTRA_CA_CERTS
        HTTP_PORT_VAL="${PROBE_HTTP_PORT_VAL:-${PROBE_HTTP_PORT}}"
        export LABTETHER_API_BASE_URL="http://localhost:${HTTP_PORT_VAL}"
        export NEXT_PUBLIC_HUB_API_PORT="${HTTP_PORT_VAL}"
        log_info "TLS source: disabled — using plain HTTP"
        ;;
      built_in|*)
        # built_in or unknown: use localhost with built-in CA trust.
        HTTPS_PORT_VAL="${PROBE_HTTPS_PORT:-8443}"
        export LABTETHER_API_BASE_URL="https://localhost:${HTTPS_PORT_VAL}"
        export NEXT_PUBLIC_HUB_API_PORT="${HTTPS_PORT_VAL}"
        if [ -f "${CA_CERT}" ]; then
          export NODE_EXTRA_CA_CERTS="${CA_CERT}"
          log_info "TLS source: ${TLS_SOURCE:-built_in} — trusting CA at ${CA_CERT}"
        fi
        ;;
    esac
  else
    # Probe failed: backend not running or unreachable. Fall back to built-in CA behavior.
    log_warn "Could not probe backend TLS info at ${PROBE_URL} — assuming built-in CA mode"
    log_warn "Set LABTETHER_API_BASE_URL to override"
    if [ -f "${CA_CERT}" ]; then
      export NODE_EXTRA_CA_CERTS="${CA_CERT}"
      export LABTETHER_API_BASE_URL="https://localhost:8443"
      export NEXT_PUBLIC_HUB_API_PORT="8443"
    else
      export LABTETHER_API_BASE_URL="http://localhost:8080"
    fi
  fi
fi

# Next.js HTTPS for the browser-facing hop.
# When Tailscale Serve is the TLS terminator, Next.js stays HTTP — Tailscale
# Serve proxies https://*.ts.net:443 → http://127.0.0.1:3000.
# Only enable --experimental-https for non-Tailscale TLS (built-in CA, etc.)
# where the browser hits Next.js directly without a reverse proxy.
if [ "${TLS_SOURCE:-}" = "tailscale" ]; then
  log_info "Tailscale Serve handles TLS — Next.js serving plain HTTP on :3000"
  log_info "Browser access: https://${CERT_HOSTNAME} (via Tailscale Serve → :3000)"
elif [ -f "${CA_CERT}" ] && [ -f "${SERVER_CERT}" ] && [ -f "${SERVER_KEY}" ]; then
  NEXT_HTTPS_FLAGS=(--experimental-https --experimental-https-ca "$CA_CERT" --experimental-https-cert "$SERVER_CERT" --experimental-https-key "$SERVER_KEY")
  log_info "Next.js serving HTTPS on :3000 with built-in certs"
  log_info "WebSocket transport: same-origin via Next.js proxy routes"
  log_info "Cert trust: trust https://localhost:3000 (or install CA system-wide: ${CA_CERT})"
else
  log_info "No TLS certs found: Next.js serving HTTP on :3000"
fi

log_info "LABTETHER_API_BASE_URL=${LABTETHER_API_BASE_URL:-http://localhost:8080}"

shell_join_quoted() {
  local __result_var=$1
  shift
  local result=""
  local quoted=""
  local arg
  for arg in "$@"; do
    printf -v quoted '%q' "$arg"
    result+="${result:+ }${quoted}"
  done
  printf -v "$__result_var" '%s' "$result"
}

FRONTEND_BIND="${LABTETHER_FRONTEND_BIND:-127.0.0.1}"
if [[ ! "$FRONTEND_BIND" =~ ^[A-Za-z0-9._:-]+$ ]]; then
  log_fail "LABTETHER_FRONTEND_BIND contains unsupported characters"
  exit 1
fi
frontend_command=(
  env
  -u LABTETHER_API_TOKEN
  -u LABTETHER_OWNER_TOKEN
  -u LABTETHER_ADMIN_PASSWORD
  -u LABTETHER_ENCRYPTION_KEY
  -u LABTETHER_SETUP_TOKEN
  -u DATABASE_URL
  -u POSTGRES_PASSWORD
  "NODE_EXTRA_CA_CERTS=${NODE_EXTRA_CA_CERTS:-}"
  "LABTETHER_API_BASE_URL=${LABTETHER_API_BASE_URL:-http://localhost:8080}"
  "NEXT_PUBLIC_HUB_API_PORT=${NEXT_PUBLIC_HUB_API_PORT:-}"
  npm run dev -- --hostname "$FRONTEND_BIND" --port 3000
)
frontend_command+=("${NEXT_HTTPS_FLAGS[@]}")
frontend_shell_command=""
shell_join_quoted frontend_shell_command "${frontend_command[@]}"
printf -v quoted_log_file '%q' "$LOG_FILE"
frontend_shell_command+=" 2>&1 | tee -a ${quoted_log_file}"
HTTP_REDIRECT_PORT=""
redirect_command=""
if [[ ${#NEXT_HTTPS_FLAGS[@]} -gt 0 ]]; then
  HTTP_REDIRECT_PORT="${LABTETHER_HTTP_REDIRECT_PORT:-3080}"
  if [[ ! "$HTTP_REDIRECT_PORT" =~ ^[0-9]+$ ]] || ((HTTP_REDIRECT_PORT < 1 || HTTP_REDIRECT_PORT > 65535)); then
    log_fail "LABTETHER_HTTP_REDIRECT_PORT must be an integer between 1 and 65535"
    exit 1
  fi
  shell_join_quoted redirect_command node "${PROJECT_ROOT}/scripts/lib/http-redirect.js" "$HTTP_REDIRECT_PORT" 3000
fi
tmux new-session -d -s "${SESSION_NAME}" -c "${PROJECT_ROOT}/web/console" "$frontend_shell_command"

if [[ ${#NEXT_HTTPS_FLAGS[@]} -gt 0 ]]; then
  tmux new-window -t "${SESSION_NAME}" -n redirect \
    "$redirect_command"
  log_info "HTTP redirect: http://localhost:${HTTP_REDIRECT_PORT} -> https://localhost:3000"
fi

log_info "Frontend started."
log_info "Session: ${SESSION_NAME}"
log_info "Attach: tmux attach -t ${SESSION_NAME}"
log_info "Logs: ${LOG_FILE}"
