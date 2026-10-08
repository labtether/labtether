#!/usr/bin/env bash
# Run from the repository root, matching the CI formatting boundary.
set -euo pipefail

UNFORMATTED="$(find . -name '*.go' -not -path './vendor/*' -print0 | xargs -0 gofmt -l)"
if [[ -n "${UNFORMATTED}" ]]; then
  echo "Go files need formatting:"
  echo "${UNFORMATTED}"
  exit 1
fi
