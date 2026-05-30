#!/bin/sh
set -eu

log_info() {
  printf '%s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'FAIL: required command not found: %s\n' "$1" >&2
    return 1
  fi
}

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
NO_VNC_FILE="${SCRIPT_DIR}/../node_modules/@novnc/novnc/core/util/browser.js"

# Patch @novnc/novnc to remove a top-level await in core/util/browser.js.
# noVNC's WebCodecs H264 detection does not need to block module evaluation;
# keeping it asynchronous also avoids Turbopack module-wrapper edge cases.

require_command grep
require_command sed

if [ ! -f "$NO_VNC_FILE" ]; then
  log_info "skip: noVNC browser.js not found at ${NO_VNC_FILE}"
  exit 0
fi

if grep -q '^supportsWebCodecsH264Decode = await _checkWebCodecsH264DecodeSupport();' "$NO_VNC_FILE"; then
  sed -i.bak 's/^supportsWebCodecsH264Decode = await _checkWebCodecsH264DecodeSupport();/_checkWebCodecsH264DecodeSupport().then((result) => { supportsWebCodecsH264Decode = result; });/' "$NO_VNC_FILE"
  rm -f "${NO_VNC_FILE}.bak"
  log_info "patched @novnc/novnc: removed top-level await in browser.js"
else
  log_info "no patch needed: expected top-level await pattern not present"
fi
