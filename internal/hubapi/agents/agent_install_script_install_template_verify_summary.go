package agents

func agentInstallScriptTemplateVerifyAndSummary() string {
	return `# ── Verify ─────────────────────────────────────────────────────────────────
step 6 "Verifying"

STATUS_URL="http://localhost:${AGENT_PORT}/agent/status"
AGENT_OK=0
(
  for i in $(seq 1 10); do
    if command -v curl &>/dev/null; then
      STATUS="$(curl -sf "${STATUS_URL}" 2>/dev/null || true)"
    else
      STATUS="$(wget -qO- "${STATUS_URL}" 2>/dev/null || true)"
    fi
    if [[ -n "${STATUS}" ]]; then
      exit 0
    fi
    sleep 1
  done
  exit 1
) &
spin $! "Waiting for agent to become ready..."
if wait $!; then
  AGENT_OK=1
  success "Agent is running"
else
  warn "Agent did not respond within 10 seconds"
  info "Check status with: systemctl status ${SERVICE_NAME}"
fi

if [[ "${FORCE_UPDATE}" -eq 1 ]]; then
  FORCE_UPDATE_LOG="/tmp/labtether-force-update.log"
  info "Force update requested: running self-update check..."
  if "${BINARY_DEST}" update self --force >"${FORCE_UPDATE_LOG}" 2>&1; then
    success "Self-update completed"
    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
      systemctl restart "${SERVICE_NAME}" || true
    fi
  else
    warn "Self-update failed"
    info "Output: $(cat "${FORCE_UPDATE_LOG}" 2>/dev/null || true)"
  fi
  rm -f "${FORCE_UPDATE_LOG}"
fi

HOSTNAME_VALUE="$(hostname 2>/dev/null || true)"
DEVICE_FINGERPRINT=""
if [[ -f "${FINGERPRINT_FILE}" ]]; then
  DEVICE_FINGERPRINT="$(tr -d '\r\n' < "${FINGERPRINT_FILE}" 2>/dev/null || true)"
fi

DOCKER_STATUS="disabled"
DOCKER_STATUS_TONE="muted"
if [[ "${DOCKER_ENABLED}" != "false" ]]; then
  if "${BINARY_DEST}" settings test docker "${DOCKER_ENDPOINT}" >/dev/null 2>&1; then
    DOCKER_STATUS="${DOCKER_ENABLED} (connected)"
    DOCKER_STATUS_TONE="success"
  else
    DOCKER_STATUS="${DOCKER_ENABLED} (check endpoint)"
    DOCKER_STATUS_TONE="warning"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────
printf "\n"
init_box_layout
box_top
box_title "LabTether Agent" "Installation Complete"
box_mid
box_kv_tone "Hostname" "${HOSTNAME_VALUE:-unknown}" accent
if [[ -n "${DEVICE_FINGERPRINT}" ]]; then
  box_kv_tone "Fingerprint" "${DEVICE_FINGERPRINT}" accent
fi
box_kv "Binary" "${BINARY_DEST}"
box_kv "Config" "${ENV_FILE}"
if [[ "${LABTETHER_WRAPPER_INSTALLED}" -eq 1 ]]; then
  box_kv_tone "CLI helper" "${LABTETHER_CLI_DEST}" accent
fi
box_kv_tone "Docker" "${DOCKER_STATUS}" "${DOCKER_STATUS_TONE}"
box_kv_tone "File scope" "${FILES_ROOT_MODE}" accent
case "${VNC_PREREQS_STATUS:-unknown}" in
  available)
    VNC_STATUS_LABEL="available"
    VNC_STATUS_TONE="success"
    ;;
  installed)
    VNC_STATUS_LABEL="installed by script"
    VNC_STATUS_TONE="success"
    ;;
  install_failed)
    VNC_STATUS_LABEL="install failed (manual action needed)"
    VNC_STATUS_TONE="danger"
    ;;
  missing_skipped)
    VNC_STATUS_LABEL="missing (skipped)"
    VNC_STATUS_TONE="warning"
    ;;
  *)
    VNC_STATUS_LABEL="unknown"
    VNC_STATUS_TONE="muted"
    ;;
esac
box_kv_tone "Desktop prereqs" "${VNC_STATUS_LABEL}" "${VNC_STATUS_TONE}"
if [[ "${AUTO_UPDATE}" == "true" ]]; then
  AUTO_UPDATE_LABEL="true"
  AUTO_UPDATE_TONE="success"
else
  AUTO_UPDATE_LABEL="false"
  AUTO_UPDATE_TONE="warning"
fi
box_kv_tone "Auto-update" "${AUTO_UPDATE_LABEL}" "${AUTO_UPDATE_TONE}"
if [[ "${TLS_SKIP_VERIFY}" == "true" ]]; then
  TLS_VERIFY_LABEL="disabled"
  TLS_VERIFY_TONE="warning"
else
  TLS_VERIFY_LABEL="enabled"
  TLS_VERIFY_TONE="success"
fi
box_kv_tone "TLS verify" "${TLS_VERIFY_LABEL}" "${TLS_VERIFY_TONE}"
if [[ -n "${TLS_CA_FILE}" ]]; then
  box_kv_tone "TLS CA" "${TLS_CA_FILE}" accent
fi
if [[ "${FORCE_UPDATE}" -eq 1 ]]; then
  box_kv_tone "Force update" "enabled" success
fi
box_mid
if [[ -n "${DEVICE_FINGERPRINT}" ]]; then
  box_line_tone "warning" "Verify this fingerprint in LabTether before approving this agent."
fi
if [[ -n "${ENROLLMENT_TOKEN}" ]]; then
  box_line_tone "success" "${SYM_OK} Auto-enrollment configured"
  box_line_tone "muted" "The agent will connect to LabTether automatically."
else
  box_line_tone "warning" "${SYM_WARN} Awaiting approval in LabTether console"
fi
box_bottom
printf "\n"
info "Tip: configure Add Device ${SYM_ARROW} Docker in the LabTether UI to"
info "surface Docker containers/stacks in the dashboard."
if [[ "${LABTETHER_WRAPPER_INSTALLED}" -eq 1 ]]; then
  info "Agent lifecycle helper: sudo labtether agent uninstall (or purge)."
fi
printf "\n"

`
}
