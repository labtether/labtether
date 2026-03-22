#!/usr/bin/env bash

set -euo pipefail

threshold="${PROXMOX_COVERAGE_THRESHOLD:-100}"
include_connector="${PROXMOX_INCLUDE_CONNECTOR:-1}"

tmp_cmd="$(mktemp)"
tmp_connector="$(mktemp)"
cleanup() {
  rm -f "$tmp_cmd" "$tmp_connector"
}
trap cleanup EXIT

echo "Running cmd/labtether Proxmox coverage check (threshold: ${threshold}%)"
go test ./cmd/labtether -coverprofile="$tmp_cmd" >/dev/null

cmd_report="$(go tool cover -func="$tmp_cmd")"
echo "$cmd_report" | awk '
  /cmd\/labtether\/proxmox_(action_runtime|api_handlers|stream_helpers)\.go:/ { print }
'

cmd_fail=0
while IFS= read -r line; do
  pct="$(awk '{print $NF}' <<<"$line" | tr -d '%')"
  if awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 < t+0 ? 0 : 1) }'; then
    echo "LOW: $line"
    cmd_fail=1
  fi
done < <(echo "$cmd_report" | awk '/cmd\/labtether\/proxmox_(action_runtime|api_handlers|stream_helpers)\.go:/ { print }')

connector_fail=0
if [[ "$include_connector" == "1" ]]; then
  echo
  echo "Running internal/connectors/proxmox coverage check (threshold: ${threshold}%)"
  go test ./internal/connectors/proxmox -coverprofile="$tmp_connector" >/dev/null

  connector_report="$(go tool cover -func="$tmp_connector")"
  echo "$connector_report" | awk '/internal\/connectors\/proxmox\/(client|connector)\.go:/ { print }'

  while IFS= read -r line; do
    pct="$(awk '{print $NF}' <<<"$line" | tr -d '%')"
    if awk -v p="$pct" -v t="$threshold" 'BEGIN { exit (p+0 < t+0 ? 0 : 1) }'; then
      echo "LOW: $line"
      connector_fail=1
    fi
  done < <(echo "$connector_report" | awk '/internal\/connectors\/proxmox\/(client|connector)\.go:/ { print }')
fi

if [[ "$cmd_fail" -ne 0 || "$connector_fail" -ne 0 ]]; then
  echo
  echo "Proxmox coverage check failed."
  exit 1
fi

echo
echo "Proxmox coverage check passed."
