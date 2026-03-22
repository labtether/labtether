#!/usr/bin/env bash
set -euo pipefail

# Validate Tailscale deployment for LabTether
# Usage: ./scripts/validate-tailscale.sh

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

FAILURES=0

pass() { log_pass "$1"; }
fail() { log_fail "$1"; FAILURES=$((FAILURES + 1)); }

log_info "=== LabTether Tailscale Validation ==="
log_info ""

if ! require_command tailscale; then
  log_info "Install: https://tailscale.com/download"
  exit 1
fi
pass "tailscale binary found"

if ! require_command curl; then
  log_info "Install curl to run reachability checks"
  exit 1
fi

if tailscale status &>/dev/null; then
  pass "tailscale daemon is running"
else
  fail "tailscale daemon is not running"
  log_info "Run: sudo tailscale up"
fi

TS_IP=$(tailscale ip -4 2>/dev/null || true)
if [[ -n "$TS_IP" ]]; then
  pass "tailscale IPv4 address: $TS_IP"
else
  fail "no tailscale IPv4 address assigned"
fi

API_PORT=${API_PORT:-8080}
if [[ -n "$TS_IP" ]]; then
  if check_http_status "http://${TS_IP}:${API_PORT}/healthz" 200 5; then
    pass "API reachable at http://${TS_IP}:${API_PORT}/healthz"
  else
    fail "API not reachable at http://${TS_IP}:${API_PORT}/healthz"
  fi
fi

CONSOLE_PORT=${CONSOLE_PORT:-3000}
if [[ -n "$TS_IP" ]]; then
  if check_http_status "http://${TS_IP}:${CONSOLE_PORT}" 200 5; then
    pass "Console reachable at http://${TS_IP}:${CONSOLE_PORT}"
  else
    fail "Console not reachable at http://${TS_IP}:${CONSOLE_PORT}"
  fi
fi

log_info ""
log_info "--- Port binding check ---"
if has_command docker; then
  compose_cmd=()
  if docker compose version >/dev/null 2>&1; then
    compose_cmd=(docker compose)
  elif has_command docker-compose; then
    compose_cmd=(docker-compose)
  fi

  if [[ ${#compose_cmd[@]} -eq 0 ]]; then
    log_info "Skipping docker compose port check: docker compose command unavailable"
  elif "${compose_cmd[@]}" ps --format json 2>/dev/null | grep -q '"PublishedPort"'; then
    log_info "WARNING: Some services have published ports. Use the Tailscale overlay to remove them."
  else
    pass "No public port bindings detected"
  fi
fi

log_info ""
if [[ "$FAILURES" -eq 0 ]]; then
  pass "All checks passed."
else
  fail "${FAILURES} check(s) failed."
  exit 1
fi
