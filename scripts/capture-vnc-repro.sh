#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
LOOKBACK_MINUTES="${LOOKBACK_MINUTES:-20}"
BACKEND_PANE_LINES="${BACKEND_PANE_LINES:-1200}"
FRONTEND_PANE_LINES="${FRONTEND_PANE_LINES:-400}"
OUTPUT_PATH=""
TARGET=""
TRACE_ID="${LABTETHER_VNC_REPRO_TRACE_ID:-}"
RUN_SMOKE=0
VERBOSE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib/script-common.sh"

EXTERNAL_API_BASE_URL="${LABTETHER_API_BASE_URL-}"
EXTERNAL_API_TOKEN="${LABTETHER_API_TOKEN-}"
EXTERNAL_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"

BANNER_FILE=""
declare -a BANNER_LINES=()

usage() {
  cat <<'USAGE'
Usage: scripts/capture-vnc-repro.sh [options]

Capture a targeted VNC repro correlation bundle for iOS desktop black-screen
triage. The script snapshots hub/frontend panes, asset presence, and macOS agent
logs (when the target is a connected darwin agent), then writes a dated report.

Typical workflow:
  1. Reproduce the black-screen or disconnect on iPhone/iPad.
  2. Copy the full rolling Connection Debug Banner lines.
  3. Run this script immediately against the affected asset.
  4. Attach the generated report when triaging.

Options:
  --target <asset-id>         Required asset ID (for example Michael-MBP.local)
  --banner-line <text>        Add one debug-banner line (repeatable)
  --banner-file <path>        Read debug-banner lines from a text file
  --trace <lt_trace>          Explicit stream trace ID; auto-detected from banner when present
  --lookback-minutes <n>      Log lookback window (default: 20)
  --output <path>             Write report to an explicit path
  --run-smoke                 Also run desktop smoke against the target and include output
  --verbose                   Include fuller API payloads in the report
  -h, --help                  Show this help

Environment:
  ENV_FILE=/path/to/.env
  LOOKBACK_MINUTES=20
  LABTETHER_VNC_REPRO_TRACE_ID=ios-...

Notes:
  - Run this from the same timezone as the iOS banner when possible; the banner
    uses local wall-clock time and the report preserves local timestamps.
  - When the iOS build includes desktop stream trace propagation, the banner
    will include `trace=ios-...`, which the script auto-detects.
USAGE
}

require_json_tools() {
  require_command curl
  require_command jq
  require_command rg
  require_command tmux
}

run_request() {
  local __body_var=$1
  local __status_var=$2
  local method=$3
  local url=$4
  local payload=${5:-}

  local -a args=("-sS" "-w" $'\n%{http_code}' -X "$method" "$url")
  if [[ "${url}" == https://* ]]; then
    args+=("-k")
  fi
  args+=(-H "Authorization: Bearer ${AUTH_TOKEN}")
  if [[ -n "$payload" ]]; then
    args+=(-H "Content-Type: application/json" --data "$payload")
  fi

  local response
  response=$(curl "${args[@]}" || true)

  local parsed_status
  parsed_status=$(printf '%s' "$response" | tail -n 1)
  local parsed_body
  parsed_body=$(printf '%s' "$response" | sed '$d')

  printf -v "$__body_var" '%s' "$parsed_body"
  printf -v "$__status_var" '%s' "$parsed_status"
}

json_extract_string() {
  local json=$1
  local expr=$2
  local value
  value=$(printf '%s' "$json" | jq -r "$expr")
  if [[ "$value" == "null" ]]; then
    printf ''
    return
  fi
  printf '%s' "$value"
}

banner_blob() {
  local line
  if [[ -n "$BANNER_FILE" ]]; then
    cat "$BANNER_FILE"
  fi
  if [[ ${#BANNER_LINES[@]} -gt 0 ]]; then
    for line in "${BANNER_LINES[@]}"; do
      printf '%s\n' "$line"
    done
  fi
}

extract_trace_from_banner() {
  local line
  while IFS= read -r line; do
    if [[ "$line" =~ trace=([^[:space:]\|]+) ]]; then
      printf '%s' "${BASH_REMATCH[1]}"
      return
    fi
  done < <(banner_blob)
}

extract_banner_times() {
  banner_blob | rg -o '\[[0-9]{2}:[0-9]{2}:[0-9]{2}\]' | tr '\n' ' ' | sed 's/[[:space:]]*$//'
}

capture_tmux_pane() {
  local target=$1
  local lines=$2
  if tmux has-session -t "$target" 2>/dev/null; then
    tmux capture-pane -pt "$target" -S "-${lines}" -J 2>/dev/null || true
    return
  fi
  printf '(tmux target %s not found)\n' "$target"
}

filter_backend_events() {
  local input=$1
  local escaped_target=""
  local escaped_trace=""
  escaped_target=$(printf '%s' "$TARGET" | sed 's/[][(){}.^$*+?|\\/]/\\&/g')
  escaped_trace=$(printf '%s' "$TRACE_ID" | sed 's/[][(){}.^$*+?|\\/]/\\&/g')
  local pattern="desktop-agent|desktop: |vnc|5900|screensharing|screen sharing|credentialsrequired|securityfailure|ticket_expired"
  if [[ -n "$escaped_target" ]]; then
    pattern="${pattern}|${escaped_target}"
  fi
  if [[ -n "$escaped_trace" ]]; then
    pattern="${pattern}|${escaped_trace}"
  fi
  printf '%s' "$input" | rg -i --context 2 -- "$pattern" || true
}

filter_frontend_events() {
  local input=$1
  local escaped_target=""
  escaped_target=$(printf '%s' "$TARGET" | sed 's/[][(){}.^$*+?|\\/]/\\&/g')
  local pattern="desktop|remote view|stream-ticket|stream ticket|vnc"
  if [[ -n "$escaped_target" ]]; then
    pattern="${pattern}|${escaped_target}"
  fi
  printf '%s' "$input" | rg -i --context 1 -- "$pattern" || true
}

latest_matching_line() {
  local input=$1
  local pattern=$2
  printf '%s' "$input" | rg -i -- "$pattern" | tail -n 1 || true
}

build_report_summary() {
  local banner_text=$1
  local backend_text=$2
  local macos_text=$3

  local correlation_mode="target+timestamp"
  if [[ -n "$TRACE_ID" && "$TRACE_ID" != "-" ]]; then
    correlation_mode="trace"
  fi

  local latest_banner=""
  latest_banner=$(printf '%s' "$banner_text" | sed '/^[[:space:]]*$/d' | tail -n 1 || true)

  local backend_connect=""
  backend_connect=$(latest_matching_line "$backend_text" 'desktop-agent: stream_connected|desktop-agent: agent_reported_started|desktop: agent session started')

  local backend_close=""
  backend_close=$(latest_matching_line "$backend_text" 'desktop-agent: stream_ended|desktop-agent: stream_setup_failed|desktop-agent: agent_reported_closed')
  if [[ -z "$backend_close" ]]; then
    backend_close=$(latest_matching_line "$backend_text" 'stream_runtime_event')
  fi

  local mac_vnc=""
  mac_vnc=$(latest_matching_line "$macos_text" '5900')

  local mac_screen=""
  mac_screen=$(latest_matching_line "$macos_text" 'screensharingd|screen.?sharing')

  local likely_close_source="undetermined from current evidence"
  if [[ "$backend_close" == *"browser_ws_closed_code_"* ]]; then
    likely_close_source="browser/webview websocket side observed the close first"
  elif [[ "$backend_close" == *"agent_reported_closed"* ]]; then
    likely_close_source="agent or target desktop runtime reported the close"
  elif [[ "$backend_close" == *"stream_setup_failed reason=agent_"* ]]; then
    likely_close_source="agent-side setup failed before the browser stream stabilized"
  elif [[ "$backend_close" == *"stream_setup_failed reason=browser_"* ]]; then
    likely_close_source="browser/websocket upgrade path failed before the agent stream stabilized"
  elif [[ -n "$mac_vnc" && -n "$mac_screen" ]]; then
    likely_close_source="macOS Screen Sharing path was reached; inspect the 5900 and screensharingd lines together"
  fi

  printf 'correlation_mode=%s\n' "$correlation_mode"
  printf 'latest_banner=%s\n' "${latest_banner:--}"
  printf 'backend_connect=%s\n' "${backend_connect:--}"
  printf 'backend_close=%s\n' "${backend_close:--}"
  printf 'mac_vnc=%s\n' "${mac_vnc:--}"
  printf 'mac_screensharing=%s\n' "${mac_screen:--}"
  printf 'likely_close_source=%s\n' "$likely_close_source"
}

capture_macos_logs() {
  if ! has_command log; then
    printf '(macOS unified log tool not available)\n'
    return
  fi

  local predicate='process == "LabTetherAgent" || process == "labtether-agent" || process == "screensharingd"'
  log show --style compact --last "${LOOKBACK_MINUTES}m" --predicate "$predicate" 2>/dev/null \
    | rg -i 'labtether|5900|vnc|screen.?sharing|trust|tls|desktop|agent' || true
}

append_section() {
  local report=$1
  local title=$2
  shift 2
  {
    printf '\n[%s]\n' "$title"
    "$@"
  } >>"$report"
}

write_text_section() {
  local report=$1
  local title=$2
  local body=$3
  {
    printf '\n[%s]\n' "$title"
    printf '%s\n' "$body"
  } >>"$report"
}

run_smoke_capture() {
  local report=$1
  if [[ "$RUN_SMOKE" != "1" ]]; then
    return
  fi
  {
    printf '\n[desktop smoke baseline]\n'
    LABTETHER_DESKTOP_SMOKE_TARGET="$TARGET" "${PROJECT_ROOT}/scripts/desktop-smoke-test.sh" --verbose || true
  } >>"$report"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="$2"
      shift
      ;;
    --banner-line)
      BANNER_LINES+=("$2")
      shift
      ;;
    --banner-file)
      BANNER_FILE="$2"
      shift
      ;;
    --trace)
      TRACE_ID="$2"
      shift
      ;;
    --lookback-minutes)
      LOOKBACK_MINUTES="$2"
      shift
      ;;
    --output)
      OUTPUT_PATH="$2"
      shift
      ;;
    --run-smoke)
      RUN_SMOKE=1
      ;;
    --verbose)
      VERBOSE=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

if [[ -z "$TARGET" ]]; then
  echo "Missing required --target <asset-id>" >&2
  exit 1
fi

if [[ -n "$BANNER_FILE" && ! -f "$BANNER_FILE" ]]; then
  echo "Banner file not found: $BANNER_FILE" >&2
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing env file: $ENV_FILE" >&2
  exit 1
fi

require_json_tools

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

if [[ -n "$EXTERNAL_API_BASE_URL" ]]; then
  LABTETHER_API_BASE_URL="$EXTERNAL_API_BASE_URL"
fi
if [[ -n "$EXTERNAL_API_TOKEN" ]]; then
  LABTETHER_API_TOKEN="$EXTERNAL_API_TOKEN"
fi
if [[ -n "$EXTERNAL_OWNER_TOKEN" ]]; then
  LABTETHER_OWNER_TOKEN="$EXTERNAL_OWNER_TOKEN"
fi

API_BASE="${LABTETHER_API_BASE_URL:-http://localhost:8080}"
AUTH_TOKEN="${LABTETHER_API_TOKEN:-${LABTETHER_OWNER_TOKEN:-}}"

if [[ -z "$AUTH_TOKEN" ]]; then
  echo "No LABTETHER_API_TOKEN or LABTETHER_OWNER_TOKEN found in $ENV_FILE" >&2
  exit 1
fi

if [[ "$API_BASE" == http://* ]]; then
  health_probe="$(curl -sS --max-time 5 "${API_BASE}/healthz" 2>/dev/null || true)"
  if [[ "${health_probe}" == *'"status":"redirect_active"'* ]]; then
    base_no_scheme="${API_BASE#http://}"
    host_port="${base_no_scheme%%/*}"
    host="${host_port%%:*}"
    redirect_port="$(printf '%s' "${health_probe}" | sed -n 's/.*https on port \([0-9][0-9]*\).*/\1/p')"
    if [[ -z "${redirect_port}" ]]; then
      redirect_port=8443
    fi
    API_BASE="https://${host}:${redirect_port}"
  fi
fi

if [[ -z "$TRACE_ID" ]]; then
  TRACE_ID="$(extract_trace_from_banner || true)"
fi

timestamp="$(date '+%Y%m%d-%H%M%S')"
if [[ -z "$OUTPUT_PATH" ]]; then
  OUTPUT_PATH="/tmp/labtether-vnc-repro-report-${timestamp}.txt"
fi

asset_body=""
asset_status=""
run_request asset_body asset_status GET "$API_BASE/assets/${TARGET}"
if [[ "$asset_status" != "200" ]]; then
  printf 'Failed to load asset %s (HTTP %s)\n' "$TARGET" "$asset_status" >&2
  exit 1
fi

connected_agents_body=""
connected_agents_status=""
run_request connected_agents_body connected_agents_status GET "$API_BASE/agents/connected"

presence_body=""
presence_status=""
run_request presence_body presence_status GET "$API_BASE/agents/presence"

asset_name="$(json_extract_string "$asset_body" '.asset.name // ""')"
asset_source="$(json_extract_string "$asset_body" '.asset.source // ""')"
asset_platform="$(json_extract_string "$asset_body" '.asset.platform // ""')"
asset_status_value="$(json_extract_string "$asset_body" '.asset.status // ""')"
asset_connected="$(printf '%s' "$connected_agents_body" | jq -r --arg target "$TARGET" 'if (.assets // [] | index($target)) != null then "yes" else "no" end' 2>/dev/null || printf 'unknown')"
banner_times="$(extract_banner_times || true)"

backend_pane="$(capture_tmux_pane "labtether-backend:0" "$BACKEND_PANE_LINES")"
frontend_pane="$(capture_tmux_pane "labtether-frontend:0" "$FRONTEND_PANE_LINES")"
backend_filtered="$(filter_backend_events "$backend_pane")"
frontend_filtered="$(filter_frontend_events "$frontend_pane")"
banner_text="$(banner_blob)"
macos_logs=""
if [[ "$asset_platform" == "darwin" || "$TARGET" == "Michael-MBP.local" ]]; then
  macos_logs="$(capture_macos_logs)"
fi

{
  printf 'LabTether VNC Repro Correlation Report\n'
  printf 'generated_at=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf 'host_tz=%s\n' "$(date '+%Z')"
  printf 'target=%s\n' "$TARGET"
  printf 'asset_name=%s\n' "${asset_name:--}"
  printf 'asset_source=%s\n' "${asset_source:--}"
  printf 'asset_platform=%s\n' "${asset_platform:--}"
  printf 'asset_status=%s\n' "${asset_status_value:--}"
  printf 'agent_connected=%s\n' "${asset_connected:--}"
  printf 'lookback_minutes=%s\n' "$LOOKBACK_MINUTES"
  printf 'trace_id=%s\n' "${TRACE_ID:--}"
  printf 'banner_times=%s\n' "${banner_times:--}"
} >"$OUTPUT_PATH"

write_text_section "$OUTPUT_PATH" "capture guidance" "Run this script immediately after the iOS repro. If the banner includes trace=ios-..., the backend desktop-agent lines below should match the same trace directly."
write_text_section "$OUTPUT_PATH" "summary" "$(build_report_summary "$banner_text" "$backend_filtered" "$macos_logs")"

write_text_section "$OUTPUT_PATH" "iOS banner lines" "$banner_text"
write_text_section "$OUTPUT_PATH" "asset api snapshot" "$(printf '%s\n' "$asset_body" | jq '.' 2>/dev/null || printf '%s\n' "$asset_body")"

if [[ "$VERBOSE" == "1" ]]; then
  write_text_section "$OUTPUT_PATH" "agents connected api snapshot" "$(printf '%s\n' "$connected_agents_body" | jq '.' 2>/dev/null || printf '%s\n' "$connected_agents_body")"
  write_text_section "$OUTPUT_PATH" "agents presence api snapshot" "$(printf '%s\n' "$presence_body" | jq '.' 2>/dev/null || printf '%s\n' "$presence_body")"
fi

write_text_section "$OUTPUT_PATH" "tmux sessions" "$(tmux ls 2>/dev/null || true)"
write_text_section "$OUTPUT_PATH" "frontend pane filtered desktop events" "$frontend_filtered"
write_text_section "$OUTPUT_PATH" "backend pane filtered desktop/agent events" "$backend_filtered"

if [[ "$VERBOSE" == "1" ]]; then
  write_text_section "$OUTPUT_PATH" "backend pane raw tail" "$backend_pane"
fi

if [[ -n "$macos_logs" ]]; then
  write_text_section "$OUTPUT_PATH" "macOS unified logs" "$macos_logs"
fi

run_smoke_capture "$OUTPUT_PATH"

printf 'Wrote VNC repro report: %s\n' "$OUTPUT_PATH"
