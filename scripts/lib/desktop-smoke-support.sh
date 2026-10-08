#!/usr/bin/env bash
# Desktop HTTP/WebSocket probes and diagnostic presentation.

log() {
  log_info "$*"
}

require_json_tools() {
  require_command curl
  require_command jq
  require_command openssl
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

print_verbose_json() {
  local label=$1
  local json=$2
  [[ "$VERBOSE" == "1" ]] || return 0
  printf '%s: %s\n' "$label" "$(labtether_redact_json_for_log "$json")"
}

run_request() {
  local __body_var=$1
  local __status_var=$2
  local method=$3
  local url=$4
  local payload=${5:-}

  local -a args=("-sS" "--connect-timeout" "${DESKTOP_CONNECT_TIMEOUT_SECONDS:-5}" "--max-time" "$TIMEOUT_SECONDS" "-w" $'\n%{http_code}' -X "$method" "$url")
  if ! labtether_build_curl_request_args "$url" 1; then
    printf -v "$__body_var" '%s' ''
    printf -v "$__status_var" '%s' '000'
    return 1
  fi
  args=("${LABTETHER_CURL_REQUEST_ARGS[@]}" "${args[@]}")
  if [[ -n "$payload" ]]; then
    args+=(-H "Content-Type: application/json" --data-binary @-)
  fi

  local response
  if [[ -n "$payload" ]]; then
    if ! response=$(labtether_curl "${args[@]}" <<<"$payload"); then
      printf -v "$__body_var" '%s' "$response"
      printf -v "$__status_var" '%s' '000'
      return 1
    fi
  elif ! response=$(labtether_curl "${args[@]}"); then
    printf -v "$__body_var" '%s' "$response"
    printf -v "$__status_var" '%s' '000'
    return 1
  fi

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
  local status=""

  labtether_build_curl_request_args "$ws_url" 1 || exit 1
  local -a curl_security_args=("${LABTETHER_CURL_REQUEST_ARGS[@]}")
  status=$(
    {
      labtether_curl "${curl_security_args[@]}" -sS --http1.1 --connect-timeout 5 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: ${WS_HANDSHAKE_KEY}" \
        -H "Origin: https://localhost:3000" \
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
  local audio_status=""
  local desktop_pid=""

  labtether_build_curl_request_args "$desktop_ws_url" 1 || exit 1
  local -a desktop_curl_security_args=("${LABTETHER_CURL_REQUEST_ARGS[@]}")
  labtether_build_curl_request_args "$audio_ws_url" 1 || exit 1
  local -a audio_curl_security_args=("${LABTETHER_CURL_REQUEST_ARGS[@]}")

  labtether_curl "${desktop_curl_security_args[@]}" -sS --http1.1 --connect-timeout 5 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: ${WS_HANDSHAKE_KEY}" \
    -H "Origin: https://localhost:3000" \
    "$desktop_ws_url" >"${LABTETHER_SECURE_CURL_DIR}/desktop-probe.log" 2>&1 &
  desktop_pid=$!

  sleep 1

  audio_status=$(
    {
      labtether_curl "${audio_curl_security_args[@]}" -sS --http1.1 --connect-timeout 5 --max-time "$TIMEOUT_SECONDS" -o /dev/null -D - \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: ${AUDIO_WS_HANDSHAKE_KEY}" \
        -H "Origin: https://localhost:3000" \
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
