#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWLIST_FILE="${ROOT_DIR}/security/gosec_allowlist.tsv"
GOSEC_VERSION="${GOSEC_VERSION:-v2.25.0}"

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
cleanup() {
  rm -f "${tmp_json}" "${tmp_err}" "${tmp_current}" "${tmp_allowlisted}"
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

current_findings="$(
  jq -r --arg prefix "${ROOT_DIR}/" '
    .Issues[]? | [.rule_id, (.file | sub("^" + $prefix; "")), (.line | tostring)] | @tsv
  ' "${tmp_json}" \
    | rg -v '^[^\t]+\t(\.worktrees|\.claude/worktrees)/' || true
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
echo "gosec allowlist check passed (${finding_count} reviewed findings)."
