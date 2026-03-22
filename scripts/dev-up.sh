#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESTART_SESSIONS=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/dev-runtime-warnings.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/dev-up.sh [options]

Start both LabTether dev tmux sessions (backend + frontend), then print a
compatible Linux agent install command for quick copy/paste.

Options:
  --restart     Restart backend/frontend sessions before starting.
  -h, --help    Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --restart)
      RESTART_SESSIONS=1
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

if ! require_command curl; then
  exit 1
fi

emit_dev_runtime_warnings

backend_args=()
frontend_args=()
log_info "Launching backend dev session..."
if [[ "${RESTART_SESSIONS}" -eq 1 ]]; then
  "${PROJECT_ROOT}/scripts/dev-backend-bg.sh" --restart
else
  "${PROJECT_ROOT}/scripts/dev-backend-bg.sh"
fi

log_info "Launching frontend dev session..."
if [[ "${RESTART_SESSIONS}" -eq 1 ]]; then
  "${PROJECT_ROOT}/scripts/dev-frontend-bg.sh" --restart
else
  "${PROJECT_ROOT}/scripts/dev-frontend-bg.sh"
fi

INSTALL_URL=""
CURL_PREFIX="curl -fsSL"
CURL_CMD=(curl -fsSL)
attempt=0

while [[ "${attempt}" -lt 60 ]]; do
  if curl -kfsSL "https://localhost:8443/install.sh" -o /dev/null 2>/dev/null; then
    INSTALL_URL="https://localhost:8443/install.sh"
    CURL_PREFIX="curl -kfsSL"
    CURL_CMD=(curl -kfsSL)
    break
  fi
  if curl -fsSL "http://localhost:8080/install.sh" -o /dev/null 2>/dev/null; then
    INSTALL_URL="http://localhost:8080/install.sh"
    CURL_PREFIX="curl -fsSL"
    CURL_CMD=(curl -fsSL)
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done

if [[ -z "${INSTALL_URL}" ]]; then
  log_info "Dev sessions started, but install script endpoint is not reachable yet."
  log_info "Wait for backend startup, then rerun: make dev-up"
  exit 0
fi

if install_script="$("${CURL_CMD[@]}" "${INSTALL_URL}" 2>/dev/null)"; then
  if grep -q -- "--install-vnc-prereqs" <<<"${install_script}"; then
    log_pass "Install script is ready and supports --install-vnc-prereqs."
  else
    log_fail "Install script is reachable, but expected --install-vnc-prereqs is missing."
  fi
else
  log_fail "Install script became unreachable during readiness verification."
fi

log_info "Recommended Linux install command (compatibility-first):"
printf '  %s %s | sudo bash -s -- --enrollment-token <token> --install-vnc-prereqs\n' "${CURL_PREFIX}" "${INSTALL_URL}"
log_info "Note: --auto-install-vnc remains accepted as an alias on newer hubs."
