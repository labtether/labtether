#!/usr/bin/env bash

emit_dev_runtime_warnings() {
  if [[ "${LABTETHER_SUPPRESS_DEV_RUNTIME_WARNINGS:-false}" == "true" ]]; then
    return 0
  fi

  if [[ "$(uname)" != "Darwin" ]]; then
    return 0
  fi

  if command -v xcrun >/dev/null 2>&1; then
    local booted_devices
    booted_devices="$(xcrun simctl list devices booted 2>/dev/null | awk '/Booted/ {print $0}')"
    if [[ -n "${booted_devices}" ]]; then
      log_warn "booted iOS Simulator detected. Default backend/frontend dev does not require it."
      printf '      %s\n' "${booted_devices}"
      log_info "      Stop it with: xcrun simctl shutdown all"
    fi
  fi

  if command -v colima >/dev/null 2>&1 && colima status >/dev/null 2>&1; then
    log_warn "Colima is running. Default backend/frontend dev does not require Docker unless you are intentionally using compose-backed services."
    log_info "      Stop it with: colima stop"
  fi
}
