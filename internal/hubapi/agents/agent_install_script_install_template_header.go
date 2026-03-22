package agents

func agentInstallScriptTemplateHeader() string {
	return `#!/bin/bash
set -euo pipefail

# LabTether Agent Installer
# Usage:  curl -fsSL %[1]s/install.sh | sudo bash
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --uninstall
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --purge
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --docker-enabled true --docker-endpoint /var/run/docker.sock
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --docker-wizard
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --files-root-mode full
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --auto-update false
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --force-update
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --enrollment-token <token>
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --install-vnc-prereqs
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --auto-install-vnc
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --tls-skip-verify true
#         curl -fsSL %[1]s/install.sh | sudo bash -s -- --tls-ca-file /etc/labtether/ca.crt

HUB_URL="%[1]s"
WS_URL="%[2]s"
BINARY_DEST="/usr/local/bin/labtether-agent"
LABTETHER_CLI_DEST="/usr/local/bin/labtether"
CONFIG_DIR="/etc/labtether"
ENV_FILE="${CONFIG_DIR}/agent.env"
TOKEN_FILE="${CONFIG_DIR}/agent-token"
DEVICE_KEY_FILE="${CONFIG_DIR}/device-key"
DEVICE_PUBLIC_KEY_FILE="${CONFIG_DIR}/device-key.pub"
FINGERPRINT_FILE="${CONFIG_DIR}/device-fingerprint"
SERVICE_NAME="labtether-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
AGENT_PORT="8090"
DOCKER_ENABLED="auto"
DOCKER_ENDPOINT="/var/run/docker.sock"
DOCKER_DISCOVERY_INTERVAL="30"
DOCKER_WIZARD=0
FILES_ROOT_MODE="home"
AUTO_UPDATE="true"
FORCE_UPDATE=0
ENROLLMENT_TOKEN=""
VNC_PREREQS_MODE="ask"
TLS_SKIP_VERIFY="false"
TLS_CA_FILE=""

# ── UI helpers ────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  BLUE=$'\033[0;34m'
  CYAN=$'\033[0;36m'
  GREEN=$'\033[0;32m'
  YELLOW=$'\033[0;33m'
  RED=$'\033[0;31m'
  WHITE=$'\033[1;37m'
  BOLD=$'\033[1m'
  DIM=$'\033[2m'
  RESET=$'\033[0m'
  SYM_OK='✓'
  SYM_ERR='✗'
  SYM_WARN='!'
  SYM_INFO='▸'
  SYM_ARROW='→'
else
  BLUE='' CYAN='' GREEN='' YELLOW='' RED='' WHITE='' BOLD='' DIM='' RESET=''
  SYM_OK='[ok]'
  SYM_ERR='[err]'
  SYM_WARN='[!!]'
  SYM_INFO='--'
  SYM_ARROW='->'
fi

TOTAL_STEPS=6

step() {
  local n="$1"; shift
  printf "\n  ${CYAN}${BOLD}[%%d/%%d]${RESET} ${BOLD}%%s${RESET}\n" "$n" "$TOTAL_STEPS" "$*"
}

success() {
  printf "  ${GREEN}${SYM_OK}${RESET} %%s\n" "$*"
}

warn() {
  printf "  ${YELLOW}${SYM_WARN} %%s${RESET}\n" "$*"
}

error() {
  printf "  ${RED}${SYM_ERR} %%s${RESET}\n" "$*" >&2
}

info() {
  printf "  ${DIM}${SYM_INFO} %%s${RESET}\n" "$*"
}

spin() {
  local pid="$1" msg="$2"
  if [[ -t 1 ]]; then
    local chars='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local i=0
    tput civis 2>/dev/null || true
    while kill -0 "$pid" 2>/dev/null; do
      printf "\r  ${CYAN}%%s${RESET} %%s" "${chars:i++%%${#chars}:1}" "$msg"
      sleep 0.08
    done
    tput cnorm 2>/dev/null || true
    printf "\r\033[2K"
  else
    wait "$pid"
  fi
}

terminal_columns() {
  local cols=""
  if command -v tput >/dev/null 2>&1; then
    cols="$(tput cols 2>/dev/null || true)"
  fi
  if [[ -z "${cols}" || ! "${cols}" =~ ^[0-9]+$ ]]; then
    cols=80
  fi
  if (( cols < 64 )); then
    cols=64
  fi
  printf '%%s' "${cols}"
}

repeat_char() {
  local char="$1" count="$2"
  local out=""
  if (( count <= 0 )); then
    return
  fi
  while (( ${#out} < count )); do
    out+="${char}"
  done
  printf '%%s' "${out:0:count}"
}

init_box_layout() {
  local cols
  cols="$(terminal_columns)"

  BOX_TOTAL_WIDTH=$((cols - 4))
  if (( BOX_TOTAL_WIDTH < 64 )); then
    BOX_TOTAL_WIDTH=64
  elif (( BOX_TOTAL_WIDTH > 94 )); then
    BOX_TOTAL_WIDTH=94
  fi

  BOX_INNER_WIDTH=$((BOX_TOTAL_WIDTH - 4))
  BOX_LABEL_WIDTH=16
  if (( BOX_INNER_WIDTH - BOX_LABEL_WIDTH - 1 < 24 )); then
    BOX_LABEL_WIDTH=12
  fi
  BOX_VALUE_WIDTH=$((BOX_INNER_WIDTH - BOX_LABEL_WIDTH - 1))
  BOX_BORDER_COLOR="${BLUE}"
  BOX_LABEL_COLOR="${DIM}${CYAN}"
  BOX_VALUE_COLOR="${WHITE}"
  BOX_MUTED_COLOR="${DIM}"
}

wrap_text() {
  local width="$1" text="$2"
  if [[ -z "${text}" ]]; then
    printf '\n'
    return
  fi
  printf '%%s' "${text}" | fold -w "${width}"
}

box_render_line() {
  local line="$1" visible_width="${2:-${#1}}" pad=""
  if (( visible_width < 0 )); then
    visible_width=0
  fi
  if (( visible_width < BOX_INNER_WIDTH )); then
    pad="$(repeat_char " " "$((BOX_INNER_WIDTH - visible_width))")"
  fi
  printf "  ${BOX_BORDER_COLOR}│${RESET} %%s%%s ${BOX_BORDER_COLOR}│${RESET}\n" "${line}" "${pad}"
}

box_border() {
  local left="$1" fill="$2" right="$3"
  printf "  ${BOX_BORDER_COLOR}%%s%%s%%s${RESET}\n" "${left}" "$(repeat_char "${fill}" "$((BOX_TOTAL_WIDTH - 2))")" "${right}"
}

box_style_text() {
  local tone="$1" text="$2"
  case "${tone}" in
    title)
      printf '%%s%%s%%s' "${WHITE}" "${text}" "${RESET}"
      ;;
    accent)
      printf '%%s%%s%%s' "${CYAN}${BOLD}" "${text}" "${RESET}"
      ;;
    success)
      printf '%%s%%s%%s' "${GREEN}${BOLD}" "${text}" "${RESET}"
      ;;
    warning)
      printf '%%s%%s%%s' "${YELLOW}${BOLD}" "${text}" "${RESET}"
      ;;
    danger)
      printf '%%s%%s%%s' "${RED}${BOLD}" "${text}" "${RESET}"
      ;;
    muted)
      printf '%%s%%s%%s' "${BOX_MUTED_COLOR}" "${text}" "${RESET}"
      ;;
    label)
      printf '%%s%%s%%s' "${BOX_LABEL_COLOR}" "${text}" "${RESET}"
      ;;
    value)
      printf '%%s%%s%%s' "${BOX_VALUE_COLOR}" "${text}" "${RESET}"
      ;;
    *)
      printf '%%s' "${text}"
      ;;
  esac
}

box_line() {
  local text="$1"
  while IFS= read -r line || [[ -n "${line}" ]]; do
    box_render_line "${line}" "${#line}"
  done < <(wrap_text "${BOX_INNER_WIDTH}" "${text}")
}

box_line_tone() {
  local tone="$1" text="$2" styled line
  while IFS= read -r line || [[ -n "${line}" ]]; do
    styled="$(box_style_text "${tone}" "${line}")"
    box_render_line "${styled}" "${#line}"
  done < <(wrap_text "${BOX_INNER_WIDTH}" "${text}")
}

box_title() {
  local heading="$1" status="$2"
  local plain="${heading} ${SYM_ARROW} ${status}"
  if (( ${#plain} > BOX_INNER_WIDTH )); then
    box_line "${plain}"
    return
  fi
  box_render_line "$(box_style_text title "${heading}") $(box_style_text accent "${SYM_ARROW}") $(box_style_text success "${status}")" "${#plain}"
}

box_kv_tone() {
  local label="$1" value="$2" tone="${3:-value}" label_text line first=1 styled
  printf -v label_text "%%-*s" "${BOX_LABEL_WIDTH}" "${label}:"
  while IFS= read -r line || [[ -n "${line}" ]]; do
    if (( first )); then
      styled="$(box_style_text label "${label_text}") $(box_style_text "${tone}" "${line}")"
      box_render_line "${styled}" "$(( ${#label_text} + 1 + ${#line} ))"
      first=0
    else
      styled="$(box_style_text label "$(printf "%%-*s" "${BOX_LABEL_WIDTH}" "")") $(box_style_text "${tone}" "${line}")"
      box_render_line "${styled}" "$(( BOX_LABEL_WIDTH + 1 + ${#line} ))"
    fi
  done < <(wrap_text "${BOX_VALUE_WIDTH}" "${value}")
}

box_kv() {
  box_kv_tone "$1" "$2" value
}

box_top() {
  box_border "┌" "─" "┐"
}

box_mid() {
  box_border "├" "─" "┤"
}

box_bottom() {
  box_border "└" "─" "┘"
}

print_banner() {
  local BT
  BT=$(printf '\x60')
  printf "${CYAN}\n"
  printf '  _          _    _____    _   _\n'
  printf ' | |    __ _| |__|_   _|__| |_| |_  ___ _ _\n'
  printf ' | |__ / _%%s | '"'"'_ \\| |/ -_)  _| '"'"' \\/ -_) '"'"'_|\n' "${BT}"
  printf ' |____|\\__,_|_.__/|_|\\___|\\___|_||_\\___|_|\n'
  printf '\n'
  printf "${RESET}"
  printf "  ${DIM}Agent Installer${RESET}\n"
}

show_usage() {
  cat <<'USAGE'
Usage: install.sh [options]

Options:
  --uninstall
      Uninstall service and binary, preserve token/device identity.
  --purge
      Uninstall and remove all token/device identity files.
  --docker-enabled <auto|true|false>
      Configure Docker integration mode (default: auto).
  --docker-endpoint <path|unix://path|http(s)://host:port>
      Configure Docker endpoint (default: /var/run/docker.sock).
  --docker-discovery-interval <seconds>
      Docker discovery/stats interval in seconds (default: 30).
  --docker-wizard
      Prompt interactively for Docker options during install.
  --files-root-mode <home|full>
      Configure file browser scope (default: home).
  --auto-update <true|false>
      Enable startup self-update checks (default: true).
  --force-update
      Force a one-time self-update check immediately after install.
  --enrollment-token <token>
      Optional one-time enrollment token to auto-enroll without pending approval flow.
  --vnc-prereqs <ask|install|skip>
      Manage desktop prerequisite installation on Linux (default: ask).
  --install-vnc-prereqs
      Install desktop prerequisites (x11vnc + Xvfb + xterm + xdotool + GStreamer) without prompting.
  --auto-install-vnc
      Alias for --install-vnc-prereqs.
  --skip-vnc-prereqs
      Skip desktop prerequisite installation prompt/install.
  --tls-skip-verify <true|false>
      Temporarily bypass hub certificate verification (default: false).
  --tls-ca-file <absolute-path>
      Path to a CA certificate PEM file used to verify the hub TLS certificate.
  -h, --help
      Show this help.
USAGE
}

`
}
