package agents

func agentInstallScriptTemplateDockerValidationAndPreflight() string {
	return `# ── Docker option normalization/validation ─────────────────────────────────
DOCKER_ENABLED="$(echo "${DOCKER_ENABLED}" | tr '[:upper:]' '[:lower:]')"

if [[ "${DOCKER_WIZARD}" -eq 1 ]]; then
  if [[ -t 0 ]]; then
    read -r -p "Docker mode [auto|true|false] (${DOCKER_ENABLED}): " docker_mode_input
    if [[ -n "${docker_mode_input}" ]]; then
      DOCKER_ENABLED="$(echo "${docker_mode_input}" | tr '[:upper:]' '[:lower:]')"
    fi

    if [[ "${DOCKER_ENABLED}" != "false" ]]; then
      read -r -p "Docker endpoint (${DOCKER_ENDPOINT}): " docker_endpoint_input
      if [[ -n "${docker_endpoint_input}" ]]; then
        DOCKER_ENDPOINT="${docker_endpoint_input}"
      fi

      read -r -p "Docker discovery interval seconds (${DOCKER_DISCOVERY_INTERVAL}): " docker_interval_input
      if [[ -n "${docker_interval_input}" ]]; then
        DOCKER_DISCOVERY_INTERVAL="${docker_interval_input}"
      fi
    fi

    read -r -p "File access scope [home|full] (${FILES_ROOT_MODE}): " files_root_input
    if [[ -n "${files_root_input}" ]]; then
      FILES_ROOT_MODE="$(echo "${files_root_input}" | tr '[:upper:]' '[:lower:]')"
    fi
  else
    warn "--docker-wizard requested without TTY; using provided/default Docker settings."
  fi
fi

case "${DOCKER_ENABLED}" in
  auto|true|false)
    ;;
  *)
    error "--docker-enabled must be one of auto|true|false."
    exit 1
    ;;
esac

if [[ "${DOCKER_ENABLED}" != "false" ]]; then
  if [[ -z "${DOCKER_ENDPOINT}" ]]; then
    error "--docker-endpoint cannot be empty when Docker is enabled."
    exit 1
  fi
  if [[ "${DOCKER_ENDPOINT}" != /* && "${DOCKER_ENDPOINT}" != unix://* && ! "${DOCKER_ENDPOINT}" =~ ^https?:// ]]; then
    error "--docker-endpoint must be an absolute path, unix:// path, or http(s) URL."
    exit 1
  fi
fi

if ! [[ "${DOCKER_DISCOVERY_INTERVAL}" =~ ^[0-9]+$ ]]; then
  error "--docker-discovery-interval must be an integer number of seconds."
  exit 1
fi
if (( DOCKER_DISCOVERY_INTERVAL < 5 || DOCKER_DISCOVERY_INTERVAL > 3600 )); then
  error "--docker-discovery-interval must be between 5 and 3600 seconds."
  exit 1
fi

case "${FILES_ROOT_MODE}" in
  home|full)
    ;;
  *)
    error "--files-root-mode must be one of home|full."
    exit 1
    ;;
esac

case "${AUTO_UPDATE}" in
  true|false)
    ;;
  *)
    error "--auto-update must be true or false."
    exit 1
    ;;
esac

case "${VNC_PREREQS_MODE}" in
  ask|install|skip)
    ;;
  *)
    error "--vnc-prereqs must be ask, install, or skip."
    exit 1
    ;;
esac

case "${TLS_SKIP_VERIFY}" in
  true|false)
    ;;
  *)
    error "--tls-skip-verify must be true or false."
    exit 1
    ;;
esac

if [[ -n "${TLS_CA_FILE}" && "${TLS_CA_FILE}" != /* ]]; then
  error "--tls-ca-file must be an absolute path."
  exit 1
fi

# ── Preflight checks ───────────────────────────────────────────────────────
step 1 "Preflight checks"

if [[ "${EUID}" -ne 0 ]]; then
  error "This script must be run as root."
  exit 1
fi
success "Running as root"

if ! command -v systemctl &>/dev/null; then
  error "systemd is required but not found."
  exit 1
fi
success "systemd available"

DOWNLOADER=""
if command -v curl &>/dev/null; then
  DOWNLOADER="curl"
elif command -v wget &>/dev/null; then
  DOWNLOADER="wget"
else
  error "curl or wget is required to download the agent binary."
  exit 1
fi
success "Downloader: ${DOWNLOADER}"

if [[ -n "${TLS_CA_FILE}" && ! -f "${TLS_CA_FILE}" ]]; then
  error "--tls-ca-file does not exist: ${TLS_CA_FILE}"
  exit 1
fi

if [[ "${TLS_SKIP_VERIFY}" == "true" ]]; then
  warn "TLS verification is disabled for install downloads"
elif [[ -n "${TLS_CA_FILE}" ]]; then
  success "TLS CA: ${TLS_CA_FILE}"
else
  success "TLS verification enabled"
fi

CURL_TLS_ARGS=()
WGET_TLS_ARGS=()
if [[ "${TLS_SKIP_VERIFY}" == "true" ]]; then
  CURL_TLS_ARGS+=("-k")
  WGET_TLS_ARGS+=("--no-check-certificate")
elif [[ -n "${TLS_CA_FILE}" ]]; then
  CURL_TLS_ARGS+=("--cacert" "${TLS_CA_FILE}")
  WGET_TLS_ARGS+=("--ca-certificate=${TLS_CA_FILE}")
fi

install_vnc_prereqs() {
  if command -v apt-get &>/dev/null; then
    DEBIAN_FRONTEND=noninteractive apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
      x11vnc xvfb xterm xdotool \
      gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
      gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav \
      gstreamer1.0-x
    return 0
  fi
  if command -v dnf &>/dev/null; then
    dnf install -y \
      x11vnc xorg-x11-server-Xvfb xterm xdotool \
      gstreamer1 gstreamer1-plugins-base gstreamer1-plugins-good \
      gstreamer1-plugins-bad-free gstreamer1-plugins-ugly-free gstreamer1-libav
    return 0
  fi
  if command -v yum &>/dev/null; then
    yum install -y \
      x11vnc xorg-x11-server-Xvfb xterm xdotool \
      gstreamer1 gstreamer1-plugins-base gstreamer1-plugins-good
    return 0
  fi
  if command -v pacman &>/dev/null; then
    pacman -Sy --noconfirm \
      x11vnc xorg-server-xvfb xterm xdotool \
      gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav
    return 0
  fi
  if command -v zypper &>/dev/null; then
    zypper --non-interactive install \
      x11vnc xorg-x11-server-Xvfb xterm xdotool \
      gstreamer gstreamer-plugins-base gstreamer-plugins-good \
      gstreamer-plugins-bad gstreamer-plugins-ugly gstreamer-plugins-libav
    return 0
  fi
  if command -v apk &>/dev/null; then
    apk add --no-cache \
      x11vnc xvfb xterm xdotool \
      gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav
    return 0
  fi
  error "No supported package manager detected for automatic desktop prerequisite installation."
  info "Install manually: x11vnc + Xvfb + xterm + xdotool + GStreamer, then reconnect desktop from LabTether."
  return 1
}

`
}
