#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWLIST_FILE="${ROOT_DIR}/security/gosec_allowlist.tsv"
# gosec commit 359a2c3 (securego/gosec#1803), not the v2.30.0 tag. v2.30.0
# loads a package as a list of absolute file paths; go/packages splits a list
# over 16383 characters into separate partial packages. At CI's checkout path
# internal/persistence crossed that, failed to type-check and lost its SSA
# rules (securego/gosec#1828). Move to the next tagged release once it exists.
GOSEC_VERSION="${GOSEC_VERSION:-v2.30.1-0.20261026094559-359a2c3f87db}"

if [[ ! -f "${ALLOWLIST_FILE}" ]]; then
  echo "missing allowlist: ${ALLOWLIST_FILE}" >&2
  exit 1
fi

if [[ -x "${ROOT_DIR}/.bin/gosec" ]]; then
  export PATH="${ROOT_DIR}/.bin:${PATH}"
fi

if ! command -v gosec >/dev/null 2>&1; then
  echo "gosec not found in PATH." >&2
  echo "install with: GOBIN=\"${ROOT_DIR}/.bin\" go install github.com/securego/gosec/v2/cmd/gosec@${GOSEC_VERSION}" >&2
  exit 1
fi

tmp_json="$(mktemp)"
tmp_err="$(mktemp)"
tmp_current="$(mktemp)"
tmp_allowlisted="$(mktemp)"
tmp_expected="$(mktemp)"
tmp_analysed="$(mktemp)"
cleanup() {
  rm -f "${tmp_json}" "${tmp_err}" "${tmp_current}" "${tmp_allowlisted}" \
    "${tmp_expected}" "${tmp_analysed}"
}
trap cleanup EXIT

scan_gosec_json() {
  : >"${tmp_json}"
  : >"${tmp_err}"
  (
    cd "${ROOT_DIR}"
    gosec -fmt=json ./... >"${tmp_json}" 2>"${tmp_err}" || true
  )
  jq -e '.' "${tmp_json}" >/dev/null 2>&1
}

scan_ok=false
for attempt in 1 2; do
  if scan_gosec_json; then
    scan_ok=true
    break
  fi
  if [[ "${attempt}" -lt 2 ]]; then
    sleep 1
  fi
done

if [[ "${scan_ok}" != "true" ]]; then
  echo "gosec check failed: scanner did not produce valid JSON output" >&2
  tail -n 20 "${tmp_err}" >&2 || true
  exit 1
fi

# A gosec built with an older golang.org/x/tools cannot read packages from a
# newer Go release. It then reports "Golang errors", skips every package and
# finds nothing; this gate then passed while analysing nothing (found 2026-11-16).
# Fail unless gosec type-checked every package that `go list ./...` gives.
go_error_count="$(jq '[(.["Golang errors"] // {})[] | length] | add // 0' "${tmp_json}")"
if [[ "${go_error_count}" != "0" ]]; then
  echo "gosec check failed: gosec could not load the code (${go_error_count} Go errors)" >&2
  jq -r '(.["Golang errors"] // {}) | to_entries[] | .key as $f | .value[] | "\($f):\(.line): \(.error)"' \
    "${tmp_json}" | head -n 5 >&2 || true
  echo "Use a gosec release built for this Go version." >&2
  exit 1
fi

if grep -F "Error building the SSA representation" "${tmp_err}" >&2; then
  echo "gosec check failed: gosec skipped analysis of the packages above" >&2
  exit 1
fi

(cd "${ROOT_DIR}" && go list -f '{{.Dir}}' ./...) | LC_ALL=C sort -u >"${tmp_expected}"
sed -n 's/^.*Import directory: //p' "${tmp_err}" | LC_ALL=C sort -u >"${tmp_analysed}"
not_analysed="$(comm -23 "${tmp_expected}" "${tmp_analysed}")"
if [[ ! -s "${tmp_expected}" || -n "${not_analysed}" ]]; then
  echo "gosec check failed: gosec did not analyse every Go package" >&2
  echo "expected $(wc -l <"${tmp_expected}" | tr -d '[:space:]'), analysed $(wc -l <"${tmp_analysed}" | tr -d '[:space:]')" >&2
  printf '%s\n' "${not_analysed}" | head -n 20 >&2 || true
  exit 1
fi
package_count="$(wc -l <"${tmp_expected}" | tr -d '[:space:]')"

current_findings="$(
  jq -r --arg prefix "${ROOT_DIR}/" '
    .Issues[]? | [.rule_id, (.file | sub("^" + $prefix; "")), (.line | tostring)] | @tsv
  ' "${tmp_json}" \
    | awk -F '\t' '$2 !~ /^(\.worktrees|\.claude\/worktrees)\//' || true
)"

printf '%s\n' "${current_findings}" | awk 'NF > 0 { print }' | LC_ALL=C sort -u >"${tmp_current}"

awk -F '\t' '
  BEGIN { OFS = "\t" }
  /^#/ { next }
  NF < 4 { next }
  { print $1, $2, $3 }
' "${ALLOWLIST_FILE}" | LC_ALL=C sort -u >"${tmp_allowlisted}"

unapproved="$(comm -23 "${tmp_current}" "${tmp_allowlisted}" || true)"
stale="$(comm -13 "${tmp_current}" "${tmp_allowlisted}" || true)"

if [[ -n "${unapproved}" ]]; then
  echo "gosec check failed: unallowlisted findings detected" >&2
  echo "${unapproved}" >&2
  exit 1
fi

if [[ -n "${stale}" ]]; then
  echo "gosec check failed: stale allowlist entries detected" >&2
  echo "${stale}" >&2
  exit 1
fi

finding_count="$(wc -l < "${tmp_current}" | tr -d '[:space:]')"
echo "gosec allowlist check passed (${package_count} packages, ${finding_count} reviewed findings)."
