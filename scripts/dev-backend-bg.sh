#!/usr/bin/env bash
set -euo pipefail
set +x
set +a
umask 077

SESSION_NAME="${LABTETHER_BACKEND_TMUX_SESSION:-labtether-backend}"
LOG_FILE="${LABTETHER_BACKEND_LOG_FILE:-${TMPDIR:-/tmp}/labtether-dev-backend.log}"
PID_FILE="${LABTETHER_BACKEND_PID_FILE:-${TMPDIR:-/tmp}/labtether-dev-backend.pid}"
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
Usage: scripts/dev-backend-bg.sh [options]

Start the LabTether backend in the background.

Options:
  --restart     Restart the backend if already running.
  --stop        Stop the background backend and exit.
  -h, --help    Show this help.

Environment:
  LABTETHER_BACKEND_TMUX_SESSION   Session name (default: labtether-backend)
  LABTETHER_BACKEND_LOG_FILE       Private log path (default: the per-user temporary directory)
  LABTETHER_BACKEND_PID_FILE       Private PID path (default: the per-user temporary directory)
USAGE
}

wait_for_exit() {
  local pid=$1
  local timeout=${2:-10}
  local waited=0
  while kill -0 "${pid}" 2>/dev/null; do
    if [[ "${waited}" -ge "${timeout}" ]]; then
      return 1
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 0
}

wait_for_listener() {
  local timeout=${1:-15}
  local waited=0
  while (( waited < timeout )); do
    if check_http_status "http://localhost:${API_PORT:-8080}/healthz" 200 2 || \
       check_http_status "https://localhost:8443/healthz" 200 2 || \
       nc -z localhost 8080 2>/dev/null || \
       nc -z localhost 8443 2>/dev/null; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

wait_for_backend_ready() {
  local pid=$1
  local timeout=${2:-15}
  local waited=0
  while (( waited < timeout )); do
    if ! kill -0 "${pid}" 2>/dev/null; then
      return 1
    fi
    if check_http_status "http://localhost:${API_PORT:-8080}/healthz" 200 2 || \
       check_http_status "https://localhost:8443/healthz" 200 2; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

read_running_backend_pid() {
  local __result_var=$1
  local pid=""
  if [[ ! -e "$PID_FILE" && ! -L "$PID_FILE" ]]; then
    return 1
  fi
  labtether_lock_down_private_file "$PID_FILE" "backend PID file" || return 2
  pid=$(<"$PID_FILE")
  if [[ ! "$pid" =~ ^[0-9]+$ || "$pid" == "0" ]]; then
    log_fail "backend PID file does not contain a valid process ID: $PID_FILE"
    return 2
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f -- "$PID_FILE"
    return 1
  fi
  local command_line=""
  command_line=$(ps -o command= -p "$pid" 2>/dev/null || true)
  if [[ "$command_line" != *"/build/labtether"* && "$command_line" != *"./build/labtether"* && "$command_line" != *"/scripts/dev-backend-run.sh"* ]]; then
    log_fail "refusing to use PID $pid because it is not a LabTether backend process"
    return 2
  fi
  printf -v "$__result_var" '%s' "$pid"
}

stop_backend_processes() {
  local pid=""
  local read_rc=0
  read_running_backend_pid pid || read_rc=$?
  if [[ "$read_rc" -ne 0 ]]; then
    return "$read_rc"
  fi
  kill "$pid" 2>/dev/null || true
  if ! wait_for_exit "$pid" 10; then
    kill -9 "$pid" 2>/dev/null || true
    if ! wait_for_exit "$pid" 5; then
      log_fail "backend PID $pid did not stop"
      return 2
    fi
  fi
  rm -f -- "$PID_FILE"
  log_info "Stopped backend PID ${pid}."
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

USE_TMUX=1
if [[ "$(uname)" == "Darwin" && "${LABTETHER_USE_TMUX:-}" != "true" ]]; then
  USE_TMUX=0
fi

if [[ "${USE_TMUX}" -eq 1 ]]; then
  if ! require_command tmux; then
    log_info "Install tmux, or run: make dev-backend"
    exit 1
  fi
fi

emit_dev_runtime_warnings

if [[ "${USE_TMUX}" -eq 0 ]]; then
  running_pid=""
  if [[ "${STOP_SESSION}" -eq 1 || "${RESTART_SESSION}" -eq 1 ]]; then
    stop_rc=0
    stop_backend_processes || stop_rc=$?
    if [[ "$stop_rc" -eq 0 ]]; then
      if [[ "${STOP_SESSION}" -eq 1 ]]; then
        exit 0
      fi
    elif [[ "$stop_rc" -eq 2 ]]; then
      exit 1
    elif [[ "${STOP_SESSION}" -eq 1 ]]; then
      log_info "Backend PID file '${PID_FILE}' is not running."
      exit 0
    fi
  else
    running_rc=0
    read_running_backend_pid running_pid || running_rc=$?
    if [[ "$running_rc" -eq 0 ]]; then
      log_info "Backend is already running as PID ${running_pid}."
      log_info "Logs: ${LOG_FILE}"
      exit 0
    elif [[ "$running_rc" -eq 2 ]]; then
      exit 1
    fi
  fi
fi

if [[ "${USE_TMUX}" -eq 1 ]] && tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
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
elif [[ "${USE_TMUX}" -eq 1 && "${STOP_SESSION}" -eq 1 ]]; then
  log_info "tmux session '${SESSION_NAME}' is not running."
  exit 0
fi

if [[ "${RESTART_SESSION}" -eq 1 ]]; then
  log_info "Restarting backend..."
else
  log_info "Starting backend..."
fi

export LABTETHER_TLS_MODE="${LABTETHER_TLS_MODE:-auto}"

if [[ -e "$LOG_FILE" || -L "$LOG_FILE" ]]; then
  labtether_lock_down_private_file "$LOG_FILE" "backend log file" || exit 1
else
  labtether_prepare_private_output_file "$LOG_FILE" || exit 1
fi

# On macOS, tmux runs as a daemon under launchd. Processes it spawns inherit
# tmux's TCC context which lacks Local Network permission (ad-hoc signed CLI
# tools are silently denied and never listed in System Settings). This blocks
# ALL outbound connections to LAN IPs from the hub process.
#
# Workaround: use Terminal.app via `open` to spawn a new tab that runs the
# backend. Terminal.app has Local Network permission, so child processes
# inherit it. Fall back to tmux on non-macOS or when LABTETHER_USE_TMUX=true.
if [[ "${USE_TMUX}" -eq 0 ]]; then
  # Build and start the backend detached from this launcher shell so it
  # survives after the script exits while still inheriting the caller's TCC
  # permissions. Avoid `make dev-backend` here; detached `make` invocations
  # proved unreliable on macOS even when the same command worked interactively.
  cd "${PROJECT_ROOT}"
  if [[ -e "$PID_FILE" || -L "$PID_FILE" ]]; then
    labtether_lock_down_private_file "$PID_FILE" "backend PID file" || exit 1
  else
    labtether_prepare_private_output_file "$PID_FILE" || exit 1
  fi
  nohup "${PROJECT_ROOT}/scripts/dev-backend-run.sh" >> "${LOG_FILE}" 2>&1 < /dev/null &
  BACKEND_PID=$!
  disown "${BACKEND_PID}" 2>/dev/null || true
  printf '%s\n' "${BACKEND_PID}" > "${PID_FILE}"

  # Fail fast if the detached process exits immediately.
  sleep 1
  if ! kill -0 "${BACKEND_PID}" 2>/dev/null; then
    log_fail "backend failed to stay running; inspect ${LOG_FILE}"
    exit 1
  fi

  if ! wait_for_backend_ready "${BACKEND_PID}" 60; then
    if kill -0 "${BACKEND_PID}" 2>/dev/null; then
      log_fail "backend did not become reachable within 60s; inspect ${LOG_FILE}"
    else
      log_fail "backend exited before becoming ready; inspect ${LOG_FILE}"
    fi
    exit 1
  fi

  log_info "Backend started (PID ${BACKEND_PID})."
  log_info "Stop: kill ${BACKEND_PID}"
  log_info "Logs: ${LOG_FILE}"
else
  printf -v quoted_runner '%q' "${PROJECT_ROOT}/scripts/dev-backend-run.sh"
  printf -v quoted_log '%q' "$LOG_FILE"
  tmux new-session -d -s "${SESSION_NAME}" -c "${PROJECT_ROOT}" \
    "${quoted_runner} 2>&1 | tee -a ${quoted_log}"

  log_info "Backend started."
  log_info "Session: ${SESSION_NAME}"
  log_info "Attach: tmux attach -t ${SESSION_NAME}"
  log_info "Logs: ${LOG_FILE}"
fi
