package agents

func agentInstallScriptTemplateArgsAndUninstall() string {
	return `# ── Arguments ──────────────────────────────────────────────────────────────
UNINSTALL=0
PURGE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --uninstall)
      UNINSTALL=1
      shift
      ;;
    --purge)
      UNINSTALL=1
      PURGE=1
      shift
      ;;
    --docker-enabled)
      if [[ $# -lt 2 ]]; then
        error "--docker-enabled requires a value."
        show_usage
        exit 1
      fi
      DOCKER_ENABLED="$2"
      shift 2
      ;;
    --docker-endpoint)
      if [[ $# -lt 2 ]]; then
        error "--docker-endpoint requires a value."
        show_usage
        exit 1
      fi
      DOCKER_ENDPOINT="$2"
      shift 2
      ;;
    --docker-discovery-interval)
      if [[ $# -lt 2 ]]; then
        error "--docker-discovery-interval requires a value."
        show_usage
        exit 1
      fi
      DOCKER_DISCOVERY_INTERVAL="$2"
      shift 2
      ;;
    --docker-wizard)
      DOCKER_WIZARD=1
      shift
      ;;
    --files-root-mode)
      if [[ $# -lt 2 ]]; then
        error "--files-root-mode requires a value."
        show_usage
        exit 1
      fi
      FILES_ROOT_MODE="$(echo "$2" | tr '[:upper:]' '[:lower:]')"
      shift 2
      ;;
    --auto-update)
      if [[ $# -lt 2 ]]; then
        error "--auto-update requires a value."
        show_usage
        exit 1
      fi
      AUTO_UPDATE="$(echo "$2" | tr '[:upper:]' '[:lower:]')"
      shift 2
      ;;
    --force-update)
      FORCE_UPDATE=1
      shift
      ;;
    --enrollment-token)
      if [[ $# -lt 2 ]]; then
        error "--enrollment-token requires a value."
        show_usage
        exit 1
      fi
      ENROLLMENT_TOKEN="$2"
      shift 2
      ;;
    --enrollment-token-file)
      if [[ $# -lt 2 ]]; then
        error "--enrollment-token-file requires a value."
        show_usage
        exit 1
      fi
      ENROLLMENT_TOKEN_SOURCE_FILE="$2"
      shift 2
      ;;
    --vnc-prereqs)
      if [[ $# -lt 2 ]]; then
        error "--vnc-prereqs requires a value."
        show_usage
        exit 1
      fi
      VNC_PREREQS_MODE="$(echo "$2" | tr '[:upper:]' '[:lower:]')"
      shift 2
      ;;
    --install-vnc-prereqs)
      VNC_PREREQS_MODE="install"
      shift
      ;;
    --auto-install-vnc)
      VNC_PREREQS_MODE="install"
      shift
      ;;
    --skip-vnc-prereqs)
      VNC_PREREQS_MODE="skip"
      shift
      ;;
    --tls-skip-verify)
      if [[ $# -lt 2 ]]; then
        error "--tls-skip-verify requires a value."
        show_usage
        exit 1
      fi
      TLS_SKIP_VERIFY="$(echo "$2" | tr '[:upper:]' '[:lower:]')"
      shift 2
      ;;
    --tls-ca-file)
      if [[ $# -lt 2 ]]; then
        error "--tls-ca-file requires a value."
        show_usage
        exit 1
      fi
      TLS_CA_FILE="$2"
      shift 2
      ;;
    -h|--help)
      show_usage
      exit 0
      ;;
    *)
      error "Unknown argument: $1"
      show_usage
      exit 1
      ;;
  esac
done

if [[ -n "${ENROLLMENT_TOKEN}" && -n "${ENROLLMENT_TOKEN_SOURCE_FILE}" ]]; then
  error "Use only one of --enrollment-token or --enrollment-token-file."
  exit 1
fi
if [[ -n "${ENROLLMENT_TOKEN_SOURCE_FILE}" ]]; then
  if [[ -L "${ENROLLMENT_TOKEN_SOURCE_FILE}" || ! -f "${ENROLLMENT_TOKEN_SOURCE_FILE}" ]]; then
    error "Enrollment token file must be a regular, non-symlink file."
    exit 1
  fi
  TOKEN_SOURCE_MODE="$(stat -c '%%a' "${ENROLLMENT_TOKEN_SOURCE_FILE}" 2>/dev/null || stat -f '%%Lp' "${ENROLLMENT_TOKEN_SOURCE_FILE}" 2>/dev/null || true)"
  if [[ ! "${TOKEN_SOURCE_MODE}" =~ ^[0-7]{3,4}$ ]] || (( (8#${TOKEN_SOURCE_MODE} & 8#077) != 0 )); then
    error "Enrollment token file must not be readable or writable by group/other users (use chmod 600)."
    exit 1
  fi
  if [[ "$(wc -c < "${ENROLLMENT_TOKEN_SOURCE_FILE}")" -gt 4096 ]]; then
    error "Enrollment token file is unexpectedly large."
    exit 1
  fi
  # Append a sentinel before command substitution so Bash does not silently
  # strip every trailing newline from the token file. Accept at most one
  # conventional terminal newline (LF or CRLF), then reject any remaining
  # line break instead of joining multiple lines into a different token.
  TOKEN_FILE_CONTENT="$(cat -- "${ENROLLMENT_TOKEN_SOURCE_FILE}"; printf '\001')"
  TOKEN_FILE_CONTENT="${TOKEN_FILE_CONTENT%%$'\001'}"
  if [[ "${TOKEN_FILE_CONTENT}" == *$'\r\n' ]]; then
    TOKEN_FILE_CONTENT="${TOKEN_FILE_CONTENT%%$'\r\n'}"
  elif [[ "${TOKEN_FILE_CONTENT}" == *$'\n' ]]; then
    TOKEN_FILE_CONTENT="${TOKEN_FILE_CONTENT%%$'\n'}"
  fi
  if [[ "${TOKEN_FILE_CONTENT}" == *$'\r'* || "${TOKEN_FILE_CONTENT}" == *$'\n'* ]]; then
    error "Enrollment token file must contain exactly one token with at most one terminal newline."
    exit 1
  fi
  ENROLLMENT_TOKEN="${TOKEN_FILE_CONTENT}"
  unset TOKEN_FILE_CONTENT
  if [[ -z "${ENROLLMENT_TOKEN}" || "${ENROLLMENT_TOKEN}" =~ [[:space:]] ]]; then
    error "Enrollment token file contains an invalid token."
    exit 1
  fi
fi
LOW_POWER_MODE="$(echo "${LOW_POWER_MODE}" | tr '[:upper:]' '[:lower:]')"
LOG_STREAM_ENABLED="$(echo "${LOG_STREAM_ENABLED}" | tr '[:upper:]' '[:lower:]')"
if [[ "${LOW_POWER_MODE}" != "true" && "${LOW_POWER_MODE}" != "false" ]]; then
  error "LABTETHER_LOW_POWER_MODE must be true or false."
  exit 1
fi
if [[ "${LOG_STREAM_ENABLED}" != "true" && "${LOG_STREAM_ENABLED}" != "false" ]]; then
  error "LABTETHER_LOG_STREAM_ENABLED must be true or false."
  exit 1
fi

print_banner

# ── Uninstall ──────────────────────────────────────────────────────────────
if [[ "${UNINSTALL}" -eq 1 ]]; then
  info "Stopping service..."
  systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
  systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
  rm -f "${SERVICE_FILE}"
  systemctl daemon-reload
  success "Service stopped and disabled"
  rm -f "${BINARY_DEST}"
  success "Binary removed"
  rm -f "${ENV_FILE}"
  success "Configuration removed"
  if [[ "${PURGE}" -eq 1 ]]; then
    rm -f "${TOKEN_FILE}"
    rm -f "${DEVICE_KEY_FILE}"
    rm -f "${DEVICE_PUBLIC_KEY_FILE}"
    rm -f "${FINGERPRINT_FILE}"
    rm -rf "${CONFIG_DIR}"
    success "Identity and tokens purged"
    printf "\n  ${GREEN}${SYM_OK}${RESET} ${BOLD}LabTether agent uninstalled and purged.${RESET}\n"
    info "All local identity and token material has been removed."
    if command -v labtether >/dev/null 2>&1; then
      info "CLI helper remains available at: $(command -v labtether)"
    fi
  else
    # Preserve agent-token and identity so a reinstall can reconnect without re-approval.
    printf "\n  ${GREEN}${SYM_OK}${RESET} ${BOLD}LabTether agent uninstalled.${RESET}\n"
    info "Token and device identity files preserved at ${CONFIG_DIR}"
    info "Use --purge to remove all local agent credentials and identity."
    if command -v labtether >/dev/null 2>&1; then
      info "Or run: sudo labtether agent purge"
    fi
  fi
  exit 0
fi

`
}
