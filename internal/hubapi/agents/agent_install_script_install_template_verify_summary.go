package agents

func agentInstallScriptTemplateVerifyAndSummary() string {
	return `# ── Verify ─────────────────────────────────────────────────────────────────
step 6 "Checking the agent"

# Ask the agent on the port its service runs with: a unit or drop-in can set
# AGENT_PORT; the agent's default is 8090.
SERVICE_AGENT_PORT="$(systemctl show --property=Environment --value "${SERVICE_NAME}" 2>/dev/null | tr ' ' '\n' | sed -n 's/^AGENT_PORT=\([0-9]\{1,5\}\)$/\1/p' | tail -n 1 || true)"
AGENT_PORT="${SERVICE_AGENT_PORT:-${AGENT_PORT}}"
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
spin $! "Waiting for the agent to start"
if wait $!; then
  AGENT_OK=1
  success "Agent is running"
else
  warn "The agent did not answer within 10 seconds"
  info "Check it with: systemctl status ${SERVICE_NAME}"
fi

if [[ "${FORCE_UPDATE}" -eq 1 ]]; then
  FORCE_UPDATE_LOG="$(mktemp "${CONFIG_DIR}/.force-update.XXXXXX")"
  trap 'rm -f -- "${FORCE_UPDATE_LOG}"' EXIT
  chmod 600 "${FORCE_UPDATE_LOG}"
  info "Checking for an agent update (--force-update)"
  # Use the service's environment so the update sees the same Hub URL, TLS
  # settings and release-signing key. Lines are read as KEY=VALUE, not run.
  FORCE_UPDATE_ENV=()
  while IFS= read -r env_line || [[ -n "${env_line}" ]]; do
    [[ "${env_line}" =~ ^(LABTETHER_[A-Z0-9_]+)=(.*)$ ]] || continue
    env_value="${BASH_REMATCH[2]}"
    if [[ ${#env_value} -ge 2 && "${env_value}" == \"*\" ]]; then
      env_value="${env_value:1:${#env_value}-2}"
    fi
    FORCE_UPDATE_ENV+=("${BASH_REMATCH[1]}=${env_value}")
  done < "${ENV_FILE}"
  if env "${FORCE_UPDATE_ENV[@]}" "${BINARY_DEST}" update self --force >"${FORCE_UPDATE_LOG}" 2>&1; then
    success "Agent update finished"
    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
      systemctl restart "${SERVICE_NAME}" || true
    fi
  else
    warn "Agent update failed:"
    info "$(cat "${FORCE_UPDATE_LOG}" 2>/dev/null || true)"
  fi
  rm -f -- "${FORCE_UPDATE_LOG}"
  trap - EXIT
fi

HOSTNAME_VALUE="$(hostname 2>/dev/null || true)"
DEVICE_FINGERPRINT=""
if [[ -f "${FINGERPRINT_FILE}" ]]; then
  DEVICE_FINGERPRINT="$(tr -d '\r\n' < "${FINGERPRINT_FILE}" 2>/dev/null || true)"
fi

# Report preserved enrollment only when the installed agent has written a
# non-empty, owner-readable private token file. Do not read the token value:
# this boolean is used only to keep the completion guidance truthful.
PERSISTED_AGENT_TOKEN=0
if [[ ! -L "${TOKEN_FILE}" && -f "${TOKEN_FILE}" && -s "${TOKEN_FILE}" && -r "${TOKEN_FILE}" ]]; then
  TOKEN_FILE_MODE="$(stat -c '%%a' "${TOKEN_FILE}" 2>/dev/null || stat -f '%%Lp' "${TOKEN_FILE}" 2>/dev/null || true)"
  if [[ "${TOKEN_FILE_MODE}" =~ ^[0-7]{3,4}$ ]] &&
     (( (8#${TOKEN_FILE_MODE} & 8#400) != 0 )) &&
     (( (8#${TOKEN_FILE_MODE} & 8#077) == 0 )); then
    PERSISTED_AGENT_TOKEN=1
  fi
fi
unset TOKEN_FILE_MODE

DOCKER_STATUS="off"
if [[ "${DOCKER_ENABLED}" != "false" ]]; then
  if "${BINARY_DEST}" settings test docker "${DOCKER_ENDPOINT}" >/dev/null 2>&1; then
    DOCKER_STATUS="connected"
  else
    DOCKER_STATUS="not reachable at ${DOCKER_ENDPOINT}"
  fi
fi

case "${VNC_PREREQS_STATUS:-unknown}" in
  available) REMOTE_VIEW_STATUS="ready" ;;
  installed) REMOTE_VIEW_STATUS="tools installed" ;;
  install_failed) REMOTE_VIEW_STATUS="tools failed to install" ;;
  missing_skipped) REMOTE_VIEW_STATUS="tools not installed" ;;
  *) REMOTE_VIEW_STATUS="unknown" ;;
esac
if [[ "${FILES_ROOT_MODE}" == "full" ]]; then
  FILES_STATUS="all folders"
else
  FILES_STATUS="home folders"
fi
if [[ "${AUTO_UPDATE}" == "true" ]]; then
  UPDATES_STATUS="automatic"
else
  UPDATES_STATUS="off"
fi
if [[ "${TLS_SKIP_VERIFY}" == "true" ]]; then
  TLS_STATUS="not verified (--tls-skip-verify)"
elif [[ -n "${TLS_CA_FILE}" ]]; then
  TLS_STATUS="verified with ${TLS_CA_FILE}"
else
  TLS_STATUS="verified"
fi
# ── Summary ───────────────────────────────────────────────────────────────
summary_row() {
  printf "  %%-12s %%s\n" "$1" "$2"
}

DEVICE_NAME="${HOSTNAME_VALUE:-this device}"
if [[ "${AGENT_OK}" -eq 1 ]]; then
  printf "\nLabTether agent installed on %%s and running.\n" "${DEVICE_NAME}"
else
  printf "\nLabTether agent installed on %%s. It is not answering yet.\n" "${DEVICE_NAME}"
fi
summary_row "Agent" "${BINARY_DEST}"
summary_row "Settings" "${ENV_FILE}"
if [[ -n "${DEVICE_FINGERPRINT}" ]]; then
  summary_row "Fingerprint" "${DEVICE_FINGERPRINT}"
fi
summary_row "Docker" "${DOCKER_STATUS}"
summary_row "Remote View" "${REMOTE_VIEW_STATUS}"
summary_row "Files" "${FILES_STATUS}"
summary_row "Updates" "${UPDATES_STATUS}"
summary_row "TLS" "${TLS_STATUS}"
if [[ "${LABTETHER_WRAPPER_INSTALLED}" -eq 1 ]]; then
  summary_row "Uninstall" "sudo labtether agent uninstall"
fi
printf "\n"
if [[ -n "${ENROLLMENT_TOKEN}" && "${PERSISTED_AGENT_TOKEN}" -eq 1 ]]; then
  success "Existing approval kept. The agent is reconnecting to LabTether."
  info "The new enrollment token is used only if LabTether no longer accepts this agent."
elif [[ -n "${ENROLLMENT_TOKEN}" ]]; then
  success "Enrollment token saved. The agent uses it to join LabTether; no approval needed."
elif [[ "${PERSISTED_AGENT_TOKEN}" -eq 1 ]]; then
  success "Existing approval kept. The agent is reconnecting to LabTether."
else
  warn "Waiting for approval. In LabTether, open Devices and approve ${DEVICE_NAME}."
  if [[ -n "${DEVICE_FINGERPRINT}" ]]; then
    info "Approve it only if LabTether shows the fingerprint above."
  fi
fi
printf "\n"

`
}
