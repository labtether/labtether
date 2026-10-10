#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/smoke-common.sh"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/smoke/sessions.sh"

audit_full_page=$(jq -cn '[range(0;200) | {type:"unrelated",timestamp:"2026-10-09T10:00:01Z"}] | {events:.}')
audit_old_page=$(jq -cn '[range(0;200) | {type:"unrelated",timestamp:"2026-10-09T09:59:59Z"}] | {events:.}')
audit_match_page='{"events":[{"type":"terminal.command.queued","session_id":"session-1","command_id":"command-1","timestamp":"2026-10-09T10:00:00Z"}]}'
audit_wrong_page='{"events":[{"type":"terminal.command.queued","session_id":"other-session","command_id":"command-1","timestamp":"2026-10-09T10:00:00Z"}]}'

smoke_audit_fixture() (
  # shellcheck disable=SC2034 # API_BASE is read by the sourced audit helper.
  PASS_COUNT=0 FAIL_COUNT=0 API_BASE=https://fixture.invalid
  local mode=$1 expected=$2 requests=0 response=""
  # shellcheck disable=SC2329 # Called by the sourced audit helper.
  run_request() {
    requests=$((requests + 1))
    case "$mode" in
      paged) if [[ "$4" == *'offset=0' ]]; then response=$audit_full_page; else response=$audit_match_page; fi ;;
      wrong) response=$audit_wrong_page ;;
      older) response=$audit_old_page ;;
    esac
    printf -v "$1" '%s' "$response"
    printf -v "$2" '%s' '200'
  }
  if smoke_check_queued_command_audit session-1 command-1 2026-10-09T10:00:00 >/dev/null; then
    [[ "$expected" == pass && "$requests" == 2 && "$PASS_COUNT" == 1 && "$FAIL_COUNT" == 0 ]]
  else
    [[ "$expected" == fail && "$requests" == 1 && "$PASS_COUNT" == 0 && "$FAIL_COUNT" == 1 ]]
  fi
)

for audit_case in 'paged|pass' 'wrong|fail' 'older|fail'; do
  IFS='|' read -r audit_mode audit_expected <<< "$audit_case"
  if smoke_audit_fixture "$audit_mode" "$audit_expected"; then
    printf 'PASS: queued audit fixture %s\n' "$audit_mode"
  else
    printf 'FAIL: queued audit fixture %s\n' "$audit_mode" >&2
    exit 1
  fi
done
