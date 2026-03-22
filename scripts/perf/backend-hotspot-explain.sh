#!/usr/bin/env bash
# Run EXPLAIN (ANALYZE, BUFFERS) plans for backend hotspot query shapes.
# Usage:
#   ./scripts/perf/backend-hotspot-explain.sh --scenario projected-group

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

if ! require_command go; then
  exit 1
fi

exec go run "${PROJECT_ROOT}/scripts/perf/backend_hotspot_explain/main.go" "$@"
