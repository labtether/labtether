#!/usr/bin/env bash
set -euo pipefail

SESSION_NAME="${LABTETHER_BACKEND_TMUX_SESSION:-labtether-backend}"
LOG_FILE="${LABTETHER_BACKEND_LOG_FILE:-/tmp/labtether-dev-backend.log}"
PID_FILE="${LABTETHER_BACKEND_PID_FILE:-/tmp/labtether-dev-backend.pid}"
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
  LABTETHER_BACKEND_LOG_FILE       Log file path (default: /tmp/labtether-dev-backend.log)
  LABTETHER_BACKEND_PID_FILE       PID file path (default: /tmp/labtether-dev-backend.pid)
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

find_backend_pids() {
  ps -Ao pid=,command= | awk '
    index($0, "/build/labtether") {
      print $1
    }
  '
}

stop_backend_processes() {
  local found=0
  local pid=""
  local pid_list=()

  if [[ -f "${PID_FILE}" ]]; then
    pid=$(cat "${PID_FILE}" 2>/dev/null || true)
    if [[ -n "${pid}" ]]; then
      pid_list+=("${pid}")
    fi
  fi

  while IFS= read -r pid; do
    [[ -n "${pid}" ]] || continue
    pid_list+=("${pid}")
  done < <(find_backend_pids)

  if [[ ${#pid_list[@]} -eq 0 ]]; then
    return 1
  fi

  local unique_pids=()
  local seen=" "
  for pid in "${pid_list[@]}"; do
    [[ "${seen}" == *" ${pid} "* ]] && continue
    seen="${seen}${pid} "
    unique_pids+=("${pid}")
  done

  for pid in "${unique_pids[@]}"; do
    if kill -0 "${pid}" 2>/dev/null; then
      found=1
      kill "${pid}" 2>/dev/null || true
      if ! wait_for_exit "${pid}" 10; then
        kill -9 "${pid}" 2>/dev/null || true
        wait_for_exit "${pid}" 5 || true
      fi
      log_info "Stopped backend PID ${pid}."
    fi
  done

  rm -f "${PID_FILE}"
  [[ "${found}" -eq 1 ]]
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
  if [[ "${STOP_SESSION}" -eq 1 || "${RESTART_SESSION}" -eq 1 ]]; then
    if stop_backend_processes; then
      if [[ "${STOP_SESSION}" -eq 1 ]]; then
        exit 0
      fi
    elif [[ "${STOP_SESSION}" -eq 1 ]]; then
      log_info "Backend PID file '${PID_FILE}' is not running."
      exit 0
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
  {
    echo "Building Go backend..."
    mkdir -p build
    go build -o build/labtether ./cmd/labtether
  } >> "${LOG_FILE}" 2>&1

  set -a
  if [[ -f .env ]]; then
    # shellcheck source=/dev/null
    . ./.env
  fi
  set +a

  # Preserve an intentionally blank password so /setup can handle first-run bootstrap.
  nohup env \
    DATABASE_URL="${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}" \
    LABTETHER_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN:-dev-owner-token-change-me}" \
    LABTETHER_ADMIN_PASSWORD="${LABTETHER_ADMIN_PASSWORD-password}" \
    LABTETHER_ENCRYPTION_KEY="${LABTETHER_ENCRYPTION_KEY:?LABTETHER_ENCRYPTION_KEY must be set (generate: openssl rand -base64 32)}" \
    LABTETHER_TLS_MODE="${LABTETHER_TLS_MODE:-auto}" \
    API_PORT="${API_PORT:-8080}" \
    ./build/labtether >> "${LOG_FILE}" 2>&1 < /dev/null &
  BACKEND_PID=$!
  disown "${BACKEND_PID}" 2>/dev/null || true
  echo "${BACKEND_PID}" > "${PID_FILE}"

  # Fail fast if the detached process exits immediately.
  sleep 1
  if ! kill -0 "${BACKEND_PID}" 2>/dev/null; then
    log_fail "backend failed to stay running; inspect ${LOG_FILE}"
    exit 1
  fi

  if ! wait_for_backend_ready "${BACKEND_PID}" 15; then
    if kill -0 "${BACKEND_PID}" 2>/dev/null; then
      log_fail "backend did not become reachable within 15s; inspect ${LOG_FILE}"
    else
      log_fail "backend exited before becoming ready; inspect ${LOG_FILE}"
    fi
    exit 1
  fi

  log_info "Backend started (PID ${BACKEND_PID})."
  log_info "Stop: kill ${BACKEND_PID}"
  log_info "Logs: ${LOG_FILE}"
else
  tmux new-session -d -s "${SESSION_NAME}" "make dev-backend | tee -a '${LOG_FILE}'"

  log_info "Backend started."
  log_info "Session: ${SESSION_NAME}"
  log_info "Attach: tmux attach -t ${SESSION_NAME}"
  log_info "Logs: ${LOG_FILE}"
fi
