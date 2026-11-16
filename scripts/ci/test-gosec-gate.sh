#!/usr/bin/env bash
# Checks scripts/check-gosec-allowlist.sh against stand-in gosec and go
# commands. Fault this guards: gosec v2.29.0 could not read Go 1.27 packages,
# reported only "Golang errors" and zero issues, and the gate still passed, so
# the required security check analysed nothing for five weeks.
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/labtether-gosec-gate.XXXXXX")"
trap 'rm -rf -- "${tmp_dir}"' EXIT

root="${tmp_dir}/repo"
mkdir -p "${root}/scripts" "${root}/security" "${tmp_dir}/bin"
cp "${PROJECT_ROOT}/scripts/check-gosec-allowlist.sh" "${root}/scripts/"
printf '# rule_id\tpath\tline\tjustification\n' >"${root}/security/gosec_allowlist.tsv"

# Stand-ins read the scenario from files the test writes.
cat >"${tmp_dir}/bin/gosec" <<'EOF'
#!/usr/bin/env bash
cat "${FAKE_DIR}/gosec.json"
cat "${FAKE_DIR}/gosec.log" >&2
EOF
cat >"${tmp_dir}/bin/go" <<'EOF'
#!/usr/bin/env bash
cat "${FAKE_DIR}/go-list.txt"
EOF
chmod +x "${tmp_dir}/bin/gosec" "${tmp_dir}/bin/go"

failures=0
scenario() { # scenario <name> <want: pass|fail> <json> <imported dirs...>
  local name=$1 want=$2 json=$3
  shift 3
  printf '%s\n' "${root}/a" "${root}/b" >"${tmp_dir}/go-list.txt"
  printf '%s\n' "${json}" >"${tmp_dir}/gosec.json"
  : >"${tmp_dir}/gosec.log"
  local dir
  for dir in "$@"; do
    printf '[gosec] 2026/11/16 00:00:00 Import directory: %s\n' "${dir}" >>"${tmp_dir}/gosec.log"
  done
  local got=pass
  if ! PATH="${tmp_dir}/bin:${PATH}" FAKE_DIR="${tmp_dir}" \
    bash "${root}/scripts/check-gosec-allowlist.sh" >"${tmp_dir}/out.txt" 2>&1; then
    got=fail
  fi
  if [[ "${got}" == "${want}" ]]; then
    printf 'PASS: %s\n' "${name}"
  else
    printf 'FAIL: %s (gate said %s, want %s)\n' "${name}" "${got}" "${want}" >&2
    sed 's/^/  /' "${tmp_dir}/out.txt" >&2
    failures=$((failures + 1))
  fi
}

clean='{"Golang errors":{},"Issues":[],"Stats":{"found":0}}'
blind='{"Golang errors":{"/repo/a/x.go":[{"line":4,"column":2,"error":"export data version 5 is greater than maximum supported version 4"}]},"Issues":[],"Stats":{"found":0}}'

scenario "every package analysed, no findings" pass "${clean}" "${root}/a" "${root}/b"
scenario "gosec cannot load the code" fail "${blind}" "${root}/a" "${root}/b"
scenario "a package was not analysed" fail "${clean}" "${root}/a"

if [[ "${failures}" -ne 0 ]]; then
  exit 1
fi
