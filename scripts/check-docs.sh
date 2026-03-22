#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

echo "Running markdown lint..."
npx --yes markdownlint-cli2@0.14.0 \
  "README.md" \
  "docs/README.md" \
  "docs/USER_GUIDE.md" \
  "docs/wiki/**/*.md"

echo "Running docs link checks..."
"$ROOT/scripts/check-doc-links.sh"

echo "Docs checks passed."
