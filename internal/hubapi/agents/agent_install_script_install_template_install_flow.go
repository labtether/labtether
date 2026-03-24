package agents

func agentInstallScriptTemplateInstallFlow() string {
	return `# ── Desktop prerequisites ──────────────────────────────────────────────────
step 2 "Desktop prerequisites"

MISSING_VNC_PREREQS=()
if ! command -v x11vnc &>/dev/null; then
  MISSING_VNC_PREREQS+=("x11vnc")
fi
if ! command -v Xvfb &>/dev/null; then
  MISSING_VNC_PREREQS+=("Xvfb")
fi
if ! command -v xdotool &>/dev/null; then
  MISSING_VNC_PREREQS+=("xdotool")
fi
if ! command -v gst-launch-1.0 &>/dev/null; then
  MISSING_VNC_PREREQS+=("gst-launch-1.0")
fi
if ! command -v gst-inspect-1.0 &>/dev/null; then
  MISSING_VNC_PREREQS+=("gst-inspect-1.0")
fi

if [[ "${#MISSING_VNC_PREREQS[@]}" -gt 0 ]]; then
  warn "Missing: ${MISSING_VNC_PREREQS[*]}"
  VNC_PREREQS_STATUS="missing"
  if [[ "${VNC_PREREQS_MODE}" == "ask" ]]; then
    if [[ -r /dev/tty && -w /dev/tty ]]; then
      printf "  ${YELLOW}${SYM_WARN}${RESET} Install desktop prerequisites now (x11vnc + Xvfb + xterm + xdotool + GStreamer)? [Y/n]: " > /dev/tty
      read -r vnc_prompt_reply < /dev/tty || vnc_prompt_reply=""
      vnc_prompt_reply="$(echo "${vnc_prompt_reply}" | tr '[:upper:]' '[:lower:]')"
      if [[ -z "${vnc_prompt_reply}" || "${vnc_prompt_reply}" == "y" || "${vnc_prompt_reply}" == "yes" ]]; then
        VNC_PREREQS_MODE="install"
      else
        VNC_PREREQS_MODE="skip"
      fi
    else
      info "No TTY detected; skipping prerequisite prompt."
      info "Rerun installer with --install-vnc-prereqs (alias: --auto-install-vnc) to auto-install the desktop stack."
      VNC_PREREQS_MODE="skip"
    fi
  fi

  if [[ "${VNC_PREREQS_MODE}" == "install" ]]; then
    VNC_LOG="$(mktemp)"
    install_vnc_prereqs >"${VNC_LOG}" 2>&1 &
    spin $! "Installing desktop prerequisites (x11vnc + Xvfb + xterm + xdotool + GStreamer)..."
    if wait $!; then
      success "Desktop prerequisites installed"
      VNC_PREREQS_STATUS="installed"
    else
      warn "Desktop prerequisites install failed. Continuing."
      info "Install manually, then reconnect desktop from LabTether."
      VNC_PREREQS_STATUS="install_failed"
    fi
    rm -f "${VNC_LOG}"
  else
    info "Skipping desktop prerequisite installation."
    info "Remote desktop features may be unavailable until x11vnc/Xvfb/xdotool/GStreamer are installed."
    VNC_PREREQS_STATUS="missing_skipped"
  fi
else
  success "Desktop prerequisites available (x11vnc + Xvfb + xterm + xdotool + GStreamer)"
  VNC_PREREQS_STATUS="available"
fi

# ── Arch detection ─────────────────────────────────────────────────────────
step 3 "Detecting platform"

MACHINE="$(uname -m)"
case "${MACHINE}" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *)
    error "Unsupported architecture: ${MACHINE}"
    exit 1
    ;;
esac
success "Architecture: ${ARCH}"

# ── Stop existing agent (reinstall) ────────────────────────────────────────
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
  info "Stopping existing agent service..."
  systemctl stop "${SERVICE_NAME}"
  success "Existing agent stopped"
fi

# ── Download binary ────────────────────────────────────────────────────────
step 4 "Downloading agent"

BINARY_URL="${HUB_URL}/api/v1/agent/binary?arch=${ARCH}"
TMP_BINARY="$(mktemp)"
trap 'rm -f "${TMP_BINARY}"' EXIT

if [[ "${DOWNLOADER}" == "curl" ]]; then
  if [[ "${#CURL_TLS_ARGS[@]}" -gt 0 ]]; then
    curl "${CURL_TLS_ARGS[@]}" -fsSL --output "${TMP_BINARY}" "${BINARY_URL}" &
  else
    curl -fsSL --output "${TMP_BINARY}" "${BINARY_URL}" &
  fi
  spin $! "Downloading labtether-agent..."
else
  if [[ "${#WGET_TLS_ARGS[@]}" -gt 0 ]]; then
    wget "${WGET_TLS_ARGS[@]}" -q -O "${TMP_BINARY}" "${BINARY_URL}" &
  else
    wget -q -O "${TMP_BINARY}" "${BINARY_URL}" &
  fi
  spin $! "Downloading labtether-agent..."
fi
if ! wait $! 2>/dev/null; then
  error "Download failed from ${BINARY_URL}"
  exit 1
fi

# ── Verify checksum ───────────────────────────────────────────────────────
info "Verifying binary integrity"

RELEASE_URL="${HUB_URL}/api/v1/agent/releases/latest?arch=${ARCH}"
if [[ "${DOWNLOADER}" == "curl" ]]; then
  if [[ "${#CURL_TLS_ARGS[@]}" -gt 0 ]]; then
    RELEASE_JSON=$(curl "${CURL_TLS_ARGS[@]}" -fsSL "${RELEASE_URL}" 2>/dev/null || true)
  else
    RELEASE_JSON=$(curl -fsSL "${RELEASE_URL}" 2>/dev/null || true)
  fi
else
  if [[ "${#WGET_TLS_ARGS[@]}" -gt 0 ]]; then
    RELEASE_JSON=$(wget "${WGET_TLS_ARGS[@]}" -q -O - "${RELEASE_URL}" 2>/dev/null || true)
  else
    RELEASE_JSON=$(wget -q -O - "${RELEASE_URL}" 2>/dev/null || true)
  fi
fi

if [[ -n "${RELEASE_JSON}" ]]; then
  EXPECTED_SHA256=$(echo "${RELEASE_JSON}" | grep -o '"sha256":"[^"]*"' | head -1 | cut -d'"' -f4)
  if [[ -n "${EXPECTED_SHA256}" ]]; then
    ACTUAL_SHA256=$(sha256sum "${TMP_BINARY}" 2>/dev/null | awk '{print $1}')
    if [[ -z "${ACTUAL_SHA256}" ]]; then
      ACTUAL_SHA256=$(shasum -a 256 "${TMP_BINARY}" 2>/dev/null | awk '{print $1}')
    fi
    if [[ "${ACTUAL_SHA256}" != "${EXPECTED_SHA256}" ]]; then
      error "SHA256 mismatch! Expected: ${EXPECTED_SHA256}, Got: ${ACTUAL_SHA256}"
      rm -f "${TMP_BINARY}"
      exit 1
    fi
    success "SHA256 verified"
  else
    warn "Could not extract checksum from release metadata, skipping verification"
  fi
else
  warn "Could not fetch release metadata, skipping verification"
fi

chmod 755 "${TMP_BINARY}"
mv "${TMP_BINARY}" "${BINARY_DEST}"
trap - EXIT
success "Installed ${SYM_ARROW} ${BINARY_DEST}"

# ── Configuration ──────────────────────────────────────────────────────────
step 5 "Configuring"

mkdir -p "${CONFIG_DIR}"
chmod 750 "${CONFIG_DIR}"

cat > "${ENV_FILE}" <<EOF
LABTETHER_WS_URL=${WS_URL}
LABTETHER_ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}
LABTETHER_TOKEN_FILE=${TOKEN_FILE}
LABTETHER_DOCKER_ENABLED=${DOCKER_ENABLED}
LABTETHER_DOCKER_SOCKET="${DOCKER_ENDPOINT}"
LABTETHER_DOCKER_DISCOVERY_INTERVAL=${DOCKER_DISCOVERY_INTERVAL}
LABTETHER_FILES_ROOT_MODE=${FILES_ROOT_MODE}
LABTETHER_AUTO_UPDATE=${AUTO_UPDATE}
LABTETHER_TLS_SKIP_VERIFY=${TLS_SKIP_VERIFY}
LABTETHER_TLS_CA_FILE=${TLS_CA_FILE}
EOF
chmod 640 "${ENV_FILE}"
success "Environment ${SYM_ARROW} ${ENV_FILE}"

LABTETHER_WRAPPER_INSTALLED=0
INSTALL_LABTETHER_WRAPPER=1
if [[ -e "${LABTETHER_CLI_DEST}" ]]; then
  if grep -q "LABTETHER_AGENT_WRAPPER=1" "${LABTETHER_CLI_DEST}" 2>/dev/null; then
    INSTALL_LABTETHER_WRAPPER=1
  else
    INSTALL_LABTETHER_WRAPPER=0
    warn "Skipping CLI helper install: ${LABTETHER_CLI_DEST} already exists."
    info "Use --uninstall/--purge or systemctl commands for removal."
  fi
fi

if [[ "${INSTALL_LABTETHER_WRAPPER}" -eq 1 ]]; then
  cat > "${LABTETHER_CLI_DEST}" <<'WRAPPER'
#!/bin/bash
set -euo pipefail
# LABTETHER_AGENT_WRAPPER=1

SERVICE_NAME="labtether-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BINARY_DEST="/usr/local/bin/labtether-agent"
CONFIG_DIR="/etc/labtether"
ENV_FILE="${CONFIG_DIR}/agent.env"
TOKEN_FILE="${CONFIG_DIR}/agent-token"
DEVICE_KEY_FILE="${CONFIG_DIR}/device-key"
DEVICE_PUBLIC_KEY_FILE="${CONFIG_DIR}/device-key.pub"
FINGERPRINT_FILE="${CONFIG_DIR}/device-fingerprint"

print_usage() {
  cat <<'USAGE'
LabTether agent helper

Usage:
  labtether agent status
  sudo labtether agent uninstall
  sudo labtether agent purge
USAGE
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "labtether: this command requires root. Run with sudo."
    exit 1
  fi
}

run_uninstall() {
  local purge="${1:-0}"

  systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
  systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
  rm -f "${SERVICE_FILE}"
  systemctl daemon-reload

  rm -f "${BINARY_DEST}"
  rm -f "${ENV_FILE}"

  if [[ "${purge}" -eq 1 ]]; then
    rm -f "${TOKEN_FILE}" "${DEVICE_KEY_FILE}" "${DEVICE_PUBLIC_KEY_FILE}" "${FINGERPRINT_FILE}"
    rm -rf "${CONFIG_DIR}"
    echo "labtether: agent uninstalled and purged."
  else
    echo "labtether: agent uninstalled."
    echo "labtether: token and device identity preserved at ${CONFIG_DIR}."
  fi
}

case "${1:-}" in
  agent)
    case "${2:-}" in
      status)
        exec systemctl status "${SERVICE_NAME}"
        ;;
      uninstall)
        require_root
        run_uninstall 0
        ;;
      purge)
        require_root
        run_uninstall 1
        ;;
      ""|help|--help|-h)
        print_usage
        ;;
      *)
        print_usage
        exit 1
        ;;
    esac
    ;;
  ""|help|--help|-h)
    print_usage
    ;;
  *)
    print_usage
    exit 1
    ;;
esac
WRAPPER
  chmod 755 "${LABTETHER_CLI_DEST}"
  LABTETHER_WRAPPER_INSTALLED=1
  success "CLI helper ${SYM_ARROW} ${LABTETHER_CLI_DEST}"
fi

if [[ "${DOCKER_ENABLED}" != "false" ]]; then
  DOCKER_LOCAL_SOCKET=""
  if [[ "${DOCKER_ENDPOINT}" == /* ]]; then
    DOCKER_LOCAL_SOCKET="${DOCKER_ENDPOINT}"
  elif [[ "${DOCKER_ENDPOINT}" == unix://* ]]; then
    DOCKER_LOCAL_SOCKET="${DOCKER_ENDPOINT#unix://}"
  fi
  if [[ -n "${DOCKER_LOCAL_SOCKET}" && ! -S "${DOCKER_LOCAL_SOCKET}" ]]; then
    warn "Docker socket not found at ${DOCKER_LOCAL_SOCKET}"
    info "Agent will install, but Docker integration may be unavailable."
  fi
fi

# If a new enrollment token is provided, force a fresh enrollment by clearing
# any persisted per-agent token from previous hubs/installations.
if [[ -n "${ENROLLMENT_TOKEN}" && -f "${TOKEN_FILE}" ]]; then
  info "Enrollment token provided; removing existing persisted token to force re-enrollment."
  rm -f "${TOKEN_FILE}"
fi

# Preserve existing agent token by default so reinstalling against the same hub
# does not require re-approval.
if [[ ! -f "${TOKEN_FILE}" ]]; then
  info "New agent token will be generated on first run."
fi

# ── Systemd service ────────────────────────────────────────────────────────
cat > "${SERVICE_FILE}" <<'UNIT'
[Unit]
Description=LabTether Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
EnvironmentFile=-/etc/labtether/agent.env
ExecStart=/usr/local/bin/labtether-agent
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
# Package actions, OS updates, and agent self-update need to write system paths.
ProtectSystem=off
ProtectHome=read-only
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now "${SERVICE_NAME}"
success "Systemd service enabled and started"

`
}
