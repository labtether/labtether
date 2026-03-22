#!/usr/bin/env bash

log_info() {
  printf '%s\n' "$*"
}

log_pass() {
  local message=$1
  printf 'PASS: %s\n' "$message"
}

log_fail() {
  local message=$1
  printf 'FAIL: %s\n' "$message"
}

log_warn() {
  local message=$1
  printf 'WARN: %s\n' "$message"
}

require_command() {
  local cmd=$1
  if ! command -v "$cmd" >/dev/null 2>&1; then
    log_fail "required command not found: $cmd"
    return 1
  fi
}

has_command() {
  local cmd=$1
  command -v "$cmd" >/dev/null 2>&1
}

check_http_status() {
  local url=$1
  local expected=${2:-200}
  local timeout=${3:-5}
  local status
  local -a curl_args=("-sS" "--max-time" "${timeout}" "-o" "/dev/null" "-w" "%{http_code}")

  if [[ "${url}" == https://* ]]; then
    # Local dev commonly uses self-signed hub certs.
    curl_args+=("-k")
  fi

  status=$(curl "${curl_args[@]}" "${url}" || true)
  if [[ "${status}" == "${expected}" ]]; then
    return 0
  fi

  return 1
}
