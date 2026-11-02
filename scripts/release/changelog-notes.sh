#!/usr/bin/env bash
# changelog-notes.sh: print one release's section of CHANGELOG.md, used as the
# GitHub release body. Fails when the section is missing or empty, so a release
# cannot go out without its notes.
#
# Usage: scripts/release/changelog-notes.sh <vX.Y.Z> [changelog-path]
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TAG="${1:?Usage: $0 <vX.Y.Z> [changelog-path]}"
CHANGELOG="${2:-${PROJECT_ROOT}/CHANGELOG.md}"

if [[ ! "${TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "changelog-notes: ${TAG} is not a vX.Y.Z release tag" >&2
  exit 2
fi
if [[ ! -f "${CHANGELOG}" ]]; then
  echo "changelog-notes: ${CHANGELOG} not found" >&2
  exit 2
fi

# The heading is "## [X.Y.Z]" followed by a date or "unreleased". The section
# ends at the next "## [" heading. Leading and trailing blank lines are dropped.
notes="$(awk -v version="${TAG#v}" '
  index($0, "## [") == 1 {
    if (found) { exit }
    if (index($0, "## [" version "]") == 1) { found = 1; next }
  }
  found { lines[++count] = $0 }
  END {
    first = 1
    while (first <= count && lines[first] ~ /^[[:space:]]*$/) { first++ }
    last = count
    while (last >= first && lines[last] ~ /^[[:space:]]*$/) { last-- }
    for (i = first; i <= last; i++) { print lines[i] }
  }
' "${CHANGELOG}")"

if [[ -z "${notes}" ]]; then
  echo "changelog-notes: CHANGELOG.md has no notes under \"## [${TAG#v}]\"" >&2
  exit 1
fi
printf '%s\n' "${notes}"
