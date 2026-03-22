#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-30}"
TARGET="${LABTETHER_DESKTOP_SMOKE_TARGET:-}"
PROTOCOL="${LABTETHER_DESKTOP_SMOKE_PROTOCOL:-vnc}"
QUALITY="${LABTETHER_DESKTOP_SMOKE_QUALITY:-medium}"
DISPLAY_NAME="${LABTETHER_DESKTOP_SMOKE_DISPLAY:-}"
RECORD="${LABTETHER_DESKTOP_SMOKE_RECORD:-false}"
EXPECT_AGENT_VNC="${LABTETHER_DESKTOP_SMOKE_EXPECT_AGENT_VNC:-0}"
PROBE_STREAM="${LABTETHER_DESKTOP_SMOKE_PROBE_STREAM:-1}"
PROBE_AUDIO="${LABTETHER_DESKTOP_SMOKE_PROBE_AUDIO:-0}"
LIST_TARGETS=0
VERBOSE=0

EXTERNAL_API_BASE_URL="${LABTETHER_API_BASE_URL-}"
EXTERNAL_API_TOKEN="${LABTETHER_API_TOKEN-}"
EXTERNAL_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/desktop-smoke-test.sh [options]

Runs a live desktop-session API smoke check against a real managed asset and
then deletes the created session to avoid leaving runtime state behind.

Options:
  --target <asset-id>         Managed asset ID to use for the smoke run
  --list-targets              Print likely desktop smoke targets and exit
  --protocol <name>           vnc | webrtc | spice | rdp (default: vnc)
  --quality <name>            low | medium | high (default: medium)
  --display <name>            Optional display/monitor selector value
  --record                    Request server-side recording at session create
  --expect-agent-vnc          Require agent-backed VNC extras (`vnc_password` + `audio_stream_path`)
  --no-probe-stream           Skip the live desktop WebSocket probe
  --probe-audio               Also probe the VNC audio sideband (requires --expect-agent-vnc)
  --verbose                   Print response bodies for easier debugging
  -h, --help                  Show this help

Environment:
  ENV_FILE=/path/to/.env
  LABTETHER_DESKTOP_SMOKE_TARGET=<asset-id>
  LABTETHER_DESKTOP_SMOKE_PROTOCOL=vnc
  LABTETHER_DESKTOP_SMOKE_QUALITY=medium
  LABTETHER_DESKTOP_SMOKE_DISPLAY="Display 2"
  LABTETHER_DESKTOP_SMOKE_RECORD=true
  LABTETHER_DESKTOP_SMOKE_EXPECT_AGENT_VNC=1
  LABTETHER_DESKTOP_SMOKE_PROBE_STREAM=1
  LABTETHER_DESKTOP_SMOKE_PROBE_AUDIO=0
  TIMEOUT_SECONDS=30
USAGE
}

log() {
  log_info "$*"
}

require_json_tools() {
  require_command curl
  require_command jq
}

assert_equal() {
  local label=$1
  local expected=$2
  local actual=$3
  if [[ "$expected" == "$actual" ]]; then
    printf '  [PASS] %s (%s)\n' "$label" "$actual"
  else
    printf '  [FAIL] %s expected=%s actual=%s\n' "$label" "$expected" "$actual"
    exit 1
  fi
}

assert_contains() {
  local label=$1
  local needle=$2
  local haystack=$3
  if [[ "$haystack" == *"$needle"* ]]; then
    printf '  [PASS] %s\n' "$label"
  else
    printf '  [FAIL] %s missing=%s\n' "$label" "$needle"
    exit 1
  fi
}

assert_nonempty() {
  local label=$1
  local value=$2
  if [[ -n "$value" ]]; then
    printf '  [PASS] %s\n' "$label"
  else
    printf '  [FAIL] %s\n' "$label"
    exit 1
  fi
}

extract_json_string() {
  local key=$1
  local json=$2
  printf '%s' "$json" | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\\1/p" | head -n 1
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

json_extract() {
  local json=$1
  local expr=$2
  printf '%s' "$json" | jq -r "$expr"
}

json_extract_string() {
  local json=$1
  local expr=$2
  local value
  value=$(json_extract "$json" "$expr")
  if [[ "$value" == "null" ]]; then
    printf ''
    return
  fi
  printf '%s' "$value"
}

is_truthy() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | xargs)" in
    1|true|yes|on)
      return 0
      ;;
  esac
  return 1
}

target_in_connected_agents() {
  local assets_json=$1
  local target=$2
  printf '%s' "$assets_json" | jq -e --arg target "$target" '.assets | index($target) != null' >/dev/null
}

print_target_candidates() {
  local assets_json=$1
  local connected_json=$2
  local presence_json=$3

  printf 'Likely desktop smoke targets:\n'
  printf '%-32s  %-10s  %-10s  %-8s  %-10s  %-8s  %-24s  %-18s  %s\n' "ASSET ID" "SOURCE" "PLATFORM" "STATUS" "CONNECTED" "WEBRTC" "WEBRTC REASON" "LOCAL CONNECTORS" "NAME"

  printf '%s' "$assets_json" | jq -r --argjson connected "$(printf '%s' "$connected_json")" --argjson presence "$(printf '%s' "$presence_json")" '
    .assets
    | map(select(
        .source == "agent"
        or .source == "proxmox"
        or .type == "host"
        or .type == "virtual-machine"
        or .type == "vm"
        or ((.metadata.proxmox_type // "") != "")
      ))
    | sort_by(.source, .platform, .id)
    | .[] as $asset
    | [
        $asset.id,
        ($asset.source // ""),
        ($asset.platform // ""),
        ($asset.status // ""),
        (if ($connected.assets | index($asset.id)) != null then "yes" else "no" end),
        (((($asset.metadata // {}).webrtc_available) // "false") | ascii_downcase),
        (
          (((($asset.metadata // {}).webrtc_unavailable_reason) // "") | gsub("[[:space:]]+"; " ")) as $reason
          | if $reason == "" then "-" else $reason end
        ),
        (
          (
            [
              (
                ($presence.presence // [])
                | map(select(.asset_id == $asset.id))
                | .[0].metadata.connectors // []
              )[]
              | select(.reachable == true)
              | .type
            ] | unique
          ) as $connectors
          | if ($connectors | length) == 0 then "-" else ($connectors | join(",")) end
        ),
        ($asset.name // "")
      ]
    | @tsv
  ' | while IFS=$'\t' read -r asset_id source platform status connected webrtc webrtc_reason local_connectors name; do
    printf '%-32s  %-10s  %-10s  %-8s  %-10s  %-8s  %-24s  %-18s  %s\n' \
      "$asset_id" "$source" "${platform:-unknown}" "${status:-unknown}" "$connected" "${webrtc:-false}" "${webrtc_reason:--}" "${local_connectors:--}" "$name"
  done
}

print_collector_summary() {
  local collectors_json=$1

  printf '\nConfigured hub collectors:\n'
  if [[ "$(printf '%s' "$collectors_json" | jq -r '(.collectors // []) | length')" == "0" ]]; then
    printf '  (none)\n'
    return
  fi

  printf '%s' "$collectors_json" | jq -r '
    (.collectors // [])
    | sort_by(.collector_type, .asset_id)
    | .[]
    | [.collector_type, .asset_id, (if .enabled then "enabled" else "disabled" end)]
    | @tsv
  ' | while IFS=$'\t' read -r collector_type asset_id enabled; do
    printf '  - %s (%s, %s)\n' "$collector_type" "$asset_id" "$enabled"
  done
}

wait_for_http() {
  local label=$1
  local url=$2
  local timeout=${3:-$TIMEOUT_SECONDS}
  local deadline=$((SECONDS + timeout))

  log "Waiting for $label at $url"
  while [[ $SECONDS -lt $deadline ]]; do
    if check_http_status "$url" 200 2; then
      log "  available: $label"
      return 0
    fi
    sleep 2
  done

  log "  timeout waiting for $label"
  return 1
}

build_ws_url() {
  local base=$1
  local path=$2
  if [[ "$base" == https://* ]]; then
    printf 'wss://%s%s' "${base#https://}" "$path"
    return
  fi
  printf 'ws://%s%s' "${base#http://}" "$path"
}

build_http_url() {
  local base=$1
  local path=$2
  printf '%s%s' "$base" "$path"
}

probe_websocket() {
  local label=$1
  local ws_url=$2
  local headers=""
  local status=""

  status=$(
    {
      curl -sk --http1.1 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
        -H "Origin: https://localhost:3000" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        "$ws_url" || true
    } | tr -d '\r' | awk '$1 ~ /^HTTP\/1\.[01]$/ {code=$2} END {print code}'
  )

  if [[ "$status" != "101" ]]; then
    printf '  [FAIL] %s (expected websocket upgrade 101, got %s)\n' "$label" "${status:-none}"
    exit 1
  fi

  printf '  [PASS] %s (101 Switching Protocols)\n' "$label"
}

probe_vnc_with_audio() {
  local desktop_ws_url=$1
  local audio_ws_url=$2
  local desktop_headers=""
  local audio_status=""
  local desktop_pid=""

  curl -sk --http1.1 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    -H "Origin: https://localhost:3000" \
    -H "Authorization: Bearer ${AUTH_TOKEN}" \
    "$desktop_ws_url" >/tmp/labtether-desktop-smoke-desktop-probe.log 2>&1 &
  desktop_pid=$!

  sleep 1

  audio_status=$(
    {
      curl -sk --http1.1 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: bXlhdWRpb3NhbXBsZWtleQ==" \
        -H "Origin: https://localhost:3000" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        "$audio_ws_url" || true
    } | tr -d '\r' | awk '$1 ~ /^HTTP\/1\.[01]$/ {code=$2} END {print code}'
  )

  wait "$desktop_pid" || true

  if [[ "$audio_status" != "101" ]]; then
    printf '  [FAIL] desktop+audio websocket probe (audio upgrade expected 101, got %s)\n' "${audio_status:-none}"
    exit 1
  fi

  printf '  [PASS] desktop+audio websocket probe (101 Switching Protocols)\n'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="$2"
      shift
      ;;
    --list-targets)
      LIST_TARGETS=1
      ;;
    --protocol)
      PROTOCOL="$2"
      shift
      ;;
    --quality)
      QUALITY="$2"
      shift
      ;;
    --display)
      DISPLAY_NAME="$2"
      shift
      ;;
    --record)
      RECORD=true
      ;;
    --expect-agent-vnc)
      EXPECT_AGENT_VNC=1
      ;;
    --no-probe-stream)
      PROBE_STREAM=0
      ;;
    --probe-audio)
      PROBE_AUDIO=1
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
    log "Detected API HTTP redirect mode; switching desktop smoke API base to ${API_BASE}"
  fi
fi

TARGET="$(printf '%s' "$TARGET" | xargs)"
PROTOCOL="$(printf '%s' "$PROTOCOL" | tr '[:upper:]' '[:lower:]' | xargs)"
QUALITY="$(printf '%s' "$QUALITY" | tr '[:upper:]' '[:lower:]' | xargs)"
DISPLAY_NAME="$(printf '%s' "$DISPLAY_NAME" | xargs)"

case "$PROTOCOL" in
  vnc|webrtc|spice|rdp) ;;
  *)
    echo "Unsupported protocol: $PROTOCOL" >&2
    exit 1
    ;;
esac

case "$QUALITY" in
  low|medium|high) ;;
  *)
    echo "Unsupported quality: $QUALITY" >&2
    exit 1
    ;;
esac

wait_for_http "API health" "$API_BASE/healthz"

connected_agents_body=""
connected_agents_status=""
run_request connected_agents_body connected_agents_status GET "$API_BASE/agents/connected"
assert_equal "GET /agents/connected" "200" "$connected_agents_status"

if [[ "$LIST_TARGETS" == "1" ]]; then
  assets_body=""
  assets_status=""
  run_request assets_body assets_status GET "$API_BASE/assets?limit=500"
  assert_equal "GET /assets" "200" "$assets_status"
  presence_body=""
  presence_status=""
  run_request presence_body presence_status GET "$API_BASE/agents/presence"
  assert_equal "GET /agents/presence" "200" "$presence_status"
  collectors_body=""
  collectors_status=""
  run_request collectors_body collectors_status GET "$API_BASE/hub-collectors?enabled=false"
  assert_equal "GET /hub-collectors" "200" "$collectors_status"
  print_target_candidates "$assets_body" "$connected_agents_body" "$presence_body"
  print_collector_summary "$collectors_body"
  non_agent_target_count="$(printf '%s' "$assets_body" | jq -r '
    (.assets // [])
    | map(select(
        (.source != "agent" and .source != "docker")
        and (
          .type == "host"
          or .type == "virtual-machine"
          or .type == "vm"
          or ((.metadata.proxmox_type // "") != "")
        )
      ))
    | length
  ')"
  if [[ "$non_agent_target_count" == "0" ]]; then
    printf '\nNote: no connector-backed desktop targets are currently present in /assets.\n'
    printf '      In this environment, desktop smoke can only verify the agent-backed paths until a Proxmox or similar desktop-capable collector is configured and synced.\n'
  fi
  exit 0
fi

if [[ -z "$TARGET" ]]; then
  echo "Desktop smoke target is required (--target or LABTETHER_DESKTOP_SMOKE_TARGET)" >&2
  exit 1
fi

asset_body=""
asset_status=""
run_request asset_body asset_status GET "$API_BASE/assets/${TARGET}"
assert_equal "GET /assets/{target}" "200" "$asset_status"

asset_source=$(json_extract_string "$asset_body" '.asset.source // ""')
asset_platform=$(json_extract_string "$asset_body" '.asset.platform // ""')
asset_status_value=$(json_extract_string "$asset_body" '.asset.status // ""')
asset_webrtc_available=$(json_extract_string "$asset_body" '((.asset.metadata // {}).webrtc_available) // "false"')
asset_webrtc_reason=$(json_extract_string "$asset_body" '((.asset.metadata // {}).webrtc_unavailable_reason) // ""')

target_connected=0
if target_in_connected_agents "$connected_agents_body" "$TARGET"; then
  target_connected=1
fi

desktop_direct_proxy_enabled=0
if is_truthy "${LABTETHER_DESKTOP_DIRECT_PROXY_ENABLED:-false}"; then
  desktop_direct_proxy_enabled=1
fi

if [[ "$PROTOCOL" == "webrtc" && "$target_connected" != "1" ]]; then
  printf '  [FAIL] WebRTC target %s is not in /agents/connected (asset source=%s status=%s platform=%s)\n' \
    "$TARGET" "${asset_source:-unknown}" "${asset_status_value:-unknown}" "${asset_platform:-unknown}"
  exit 1
fi

if [[ "$PROTOCOL" == "webrtc" && "$(printf '%s' "${asset_webrtc_available:-false}" | tr '[:upper:]' '[:lower:]' | xargs)" != "true" ]]; then
  printf '  [FAIL] WebRTC target %s does not currently advertise webrtc_available=true (reported=%s)\n' \
    "$TARGET" "${asset_webrtc_available:-false}"
  if [[ -n "${asset_webrtc_reason:-}" ]]; then
    printf '         reason=%s\n' "${asset_webrtc_reason}"
  fi
  printf '         source=%s status=%s platform=%s\n' "${asset_source:-unknown}" "${asset_status_value:-unknown}" "${asset_platform:-unknown}"
  exit 1
fi

if [[ "$EXPECT_AGENT_VNC" == "1" && "$target_connected" != "1" ]]; then
  printf '  [FAIL] agent-backed VNC target %s is not in /agents/connected (asset source=%s status=%s platform=%s)\n' \
    "$TARGET" "${asset_source:-unknown}" "${asset_status_value:-unknown}" "${asset_platform:-unknown}"
  exit 1
fi

if [[ "$asset_source" == "agent" && "$target_connected" != "1" && "$PROTOCOL" == "vnc" && "$desktop_direct_proxy_enabled" != "1" ]]; then
  printf '  [FAIL] asset %s is agent-sourced but not currently in /agents/connected, and direct VNC proxy is disabled\n' "$TARGET"
  printf '         asset status=%s platform=%s\n' "${asset_status_value:-unknown}" "${asset_platform:-unknown}"
  exit 1
fi

printf '  [PASS] desktop smoke preflight target=%s source=%s status=%s platform=%s connected_agent=%s webrtc_available=%s\n' \
  "$TARGET" "${asset_source:-unknown}" "${asset_status_value:-unknown}" "${asset_platform:-unknown}" "$target_connected" "${asset_webrtc_available:-false}"
if [[ -n "${asset_webrtc_reason:-}" ]]; then
  printf '         webrtc_reason=%s\n' "${asset_webrtc_reason}"
fi

desktop_session_id=""
cleanup() {
  if [[ -z "${desktop_session_id:-}" ]]; then
    return
  fi
  local cleanup_body=""
  local cleanup_status=""
  run_request cleanup_body cleanup_status DELETE "$API_BASE/desktop/sessions/${desktop_session_id}"
  if [[ "$cleanup_status" == "204" || "$cleanup_status" == "404" ]]; then
    printf '  [PASS] cleanup desktop session %s (%s)\n' "$desktop_session_id" "$cleanup_status"
  else
    printf '  [WARN] cleanup desktop session %s failed (%s)\n' "$desktop_session_id" "${cleanup_status:-000}"
    [[ "$VERBOSE" == "1" && -n "$cleanup_body" ]] && printf 'cleanup body: %s\n' "$cleanup_body"
  fi
}
trap cleanup EXIT

create_payload=$(printf '{"target":"%s","quality":"%s","protocol":"%s","record":%s' \
  "$TARGET" "$QUALITY" "$PROTOCOL" "$RECORD")
if [[ -n "$DISPLAY_NAME" ]]; then
  create_payload+=$(printf ',"display":"%s"' "$DISPLAY_NAME")
fi
create_payload+='}'

body=""
status=""
run_request body status POST "$API_BASE/desktop/sessions" "$create_payload"
assert_equal "POST /desktop/sessions" "201" "$status"
[[ "$VERBOSE" == "1" ]] && printf 'create response: %s\n' "$body"

desktop_session_id=$(extract_json_string "id" "$body")
assert_nonempty "desktop session id parsed" "$desktop_session_id"

body=""
status=""
run_request body status GET "$API_BASE/desktop/sessions/${desktop_session_id}"
assert_equal "GET /desktop/sessions/{id}" "200" "$status"
assert_contains "desktop session target preserved" "\"target\":\"${TARGET}\"" "$body"
assert_contains "desktop session mode preserved" "\"mode\":\"desktop\"" "$body"

if [[ "$PROTOCOL" == "spice" ]]; then
  body=""
  status=""
  run_request body status POST "$API_BASE/desktop/sessions/${desktop_session_id}/spice-ticket"
  assert_equal "POST /desktop/sessions/{id}/spice-ticket" "201" "$status"
  [[ "$VERBOSE" == "1" ]] && printf 'spice ticket response: %s\n' "$body"

  stream_path=$(extract_json_string "stream_path" "$body")
  password=$(extract_json_string "password" "$body")
  assert_nonempty "SPICE stream_path present" "$stream_path"
  assert_nonempty "SPICE password present" "$password"
  assert_contains "SPICE stream_path targets session stream" "/desktop/sessions/${desktop_session_id}/stream" "$stream_path"
  assert_contains "SPICE stream_path keeps protocol=spice" "protocol=spice" "$stream_path"
else
  body=""
  status=""
  run_request body status POST "$API_BASE/desktop/sessions/${desktop_session_id}/stream-ticket"
  assert_equal "POST /desktop/sessions/{id}/stream-ticket" "201" "$status"
  [[ "$VERBOSE" == "1" ]] && printf 'stream ticket response: %s\n' "$body"

  ticket=$(extract_json_string "ticket" "$body")
  stream_path=$(extract_json_string "stream_path" "$body")
  returned_protocol=$(extract_json_string "protocol" "$body")
  assert_nonempty "desktop stream ticket present" "$ticket"
  assert_nonempty "desktop stream_path present" "$stream_path"
  assert_contains "desktop stream_path targets session stream" "/desktop/sessions/${desktop_session_id}/stream" "$stream_path"

  if [[ "$PROTOCOL" != "vnc" ]]; then
    assert_contains "desktop stream_path carries protocol selector" "protocol=${PROTOCOL}" "$stream_path"
    assert_nonempty "desktop ticket protocol present" "$returned_protocol"
  fi

  if [[ "$PROTOCOL" == "vnc" && "$EXPECT_AGENT_VNC" == "1" ]]; then
    vnc_password=$(extract_json_string "vnc_password" "$body")
    audio_stream_path=$(extract_json_string "audio_stream_path" "$body")
    assert_nonempty "agent-backed VNC password present" "$vnc_password"
    assert_nonempty "agent-backed VNC audio stream path present" "$audio_stream_path"
    if [[ "$stream_path" == *"$vnc_password"* ]]; then
      printf '  [FAIL] VNC password leaked into stream_path\n'
      exit 1
    fi
    if [[ "$audio_stream_path" == *"$vnc_password"* ]]; then
      printf '  [FAIL] VNC password leaked into audio_stream_path\n'
      exit 1
    fi
    assert_contains "agent-backed VNC audio path targets session audio stream" "/desktop/sessions/${desktop_session_id}/audio" "$audio_stream_path"
  fi
fi

if [[ "$PROBE_STREAM" == "1" ]]; then
  desktop_ws_url=$(build_http_url "$API_BASE" "$stream_path")
  if [[ "$PROTOCOL" == "vnc" && "$EXPECT_AGENT_VNC" == "1" && "$PROBE_AUDIO" == "1" ]]; then
    if [[ -z "${audio_stream_path:-}" ]]; then
      printf '  [FAIL] audio sideband probe requested but no audio_stream_path was returned\n'
      exit 1
    fi
    audio_ws_url=$(build_http_url "$API_BASE" "$audio_stream_path")
    probe_vnc_with_audio "$desktop_ws_url" "$audio_ws_url"
  else
    probe_websocket "desktop stream websocket probe" "$desktop_ws_url"
  fi
fi

body=""
status=""
deleted_session_id="$desktop_session_id"
run_request body status DELETE "$API_BASE/desktop/sessions/${desktop_session_id}"
assert_equal "DELETE /desktop/sessions/{id}" "204" "$status"
desktop_session_id=""

body=""
status=""
run_request body status GET "$API_BASE/desktop/sessions/${deleted_session_id}"
if [[ "$status" != "404" ]]; then
  printf '  [FAIL] GET deleted desktop session expected=404 actual=%s\n' "$status"
  exit 1
fi
printf '  [PASS] deleted desktop session is no longer accessible (404)\n'

log "Desktop smoke test completed successfully"
