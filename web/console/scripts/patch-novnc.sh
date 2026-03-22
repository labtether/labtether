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
NO_VNC_FILE="${SCRIPT_DIR}/../node_modules/@novnc/novnc/lib/util/browser.js"

# Patch @novnc/novnc to remove a top-level await in lib/util/browser.js.
# noVNC v1.6.0 ships a Babel-compiled CJS file that contains a top-level
# `await` for WebCodecs H264 detection. This breaks Turbopack (Next.js 16)
# which wraps async CJS modules in a context where `exports` is undefined.
# The fix: replace the top-level await with a .then() callback so the
# module stays synchronous. The capability flag starts as `false` and gets
# updated when the async check completes (fine for feature detection).

require_command grep
require_command sed

if [ ! -f "$NO_VNC_FILE" ]; then
  log_info "skip: noVNC browser.js not found at ${NO_VNC_FILE}"
  exit 0
fi

if grep -q '^exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = await' "$NO_VNC_FILE"; then
  sed -i.bak 's/^exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = await _checkWebCodecsH264DecodeSupport();/_checkWebCodecsH264DecodeSupport().then(function(r) { exports.supportsWebCodecsH264Decode = supportsWebCodecsH264Decode = r; });/' "$NO_VNC_FILE"
  rm -f "${NO_VNC_FILE}.bak"
  log_info "patched @novnc/novnc: removed top-level await in browser.js"
else
  log_info "no patch needed: expected top-level await pattern not present"
fi
