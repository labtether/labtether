#!/usr/bin/env bash
set -euo pipefail

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/portainer-coverage.XXXXXX")"
cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

echo "Running strict Portainer coverage checks..."

go test ./internal/connectors/portainer -coverprofile="${tmp_dir}/portainer.out" >/dev/null
go test ./cmd/labtether -coverprofile="${tmp_dir}/cmd.out" >/dev/null

pkg_cov="$(
  go tool cover -func="${tmp_dir}/portainer.out" \
    | awk '/^total:/ { gsub("%", "", $3); print $3 }'
)"
api_test_cov="$(
  go tool cover -func="${tmp_dir}/cmd.out" \
    | awk '$2=="handlePortainerConnectorTest" { gsub("%", "", $3); print $3 }'
)"
api_routes_cov="$(
  go tool cover -func="${tmp_dir}/cmd.out" \
    | awk '$2=="handlePortainerConnectorActions" { gsub("%", "", $3); print $3 }'
)"

fail=0
check_cov() {
  local label="$1"
  local value="$2"

  if [[ -z "${value}" ]]; then
    echo "missing coverage metric for ${label}"
    fail=1
    return
  fi

  if [[ "${value}" != "100.0" ]]; then
    echo "${label}: ${value}% (expected 100.0%)"
    fail=1
    return
  fi

  echo "${label}: ${value}%"
}

check_cov "internal/connectors/portainer" "${pkg_cov}"
check_cov "cmd/labtether handlePortainerConnectorTest" "${api_test_cov}"
check_cov "cmd/labtether handlePortainerConnectorActions" "${api_routes_cov}"

if [[ "${fail}" -ne 0 ]]; then
  exit 1
fi

echo "strict Portainer coverage gate passed"
