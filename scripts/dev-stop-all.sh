#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KEEP_SIMULATOR=0
KEEP_COLIMA=0

usage() {
  cat <<'USAGE'
Usage: scripts/dev-stop-all.sh [options]

Stop local LabTether dev services and clean up optional runtimes that are
commonly left behind by Docker or iOS workflows.

Options:
  --keep-simulator   Leave any booted iOS simulators running.
  --keep-colima      Leave Colima running.
  -h, --help         Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep-simulator)
      KEEP_SIMULATOR=1
      ;;
    --keep-colima)
      KEEP_COLIMA=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown arg: $1" >&2
      usage
      exit 2
      ;;
  esac
  shift
done

"${PROJECT_ROOT}/scripts/dev-backend-stop.sh"
"${PROJECT_ROOT}/scripts/dev-frontend-stop.sh"

if [[ "$(uname)" == "Darwin" ]]; then
  if [[ "${KEEP_SIMULATOR}" -eq 0 ]] && command -v xcrun >/dev/null 2>&1; then
    if xcrun simctl list devices booted 2>/dev/null | grep -q "Booted"; then
      echo "Stopping booted iOS simulators..."
      xcrun simctl shutdown all
    else
      echo "No booted iOS simulators."
    fi
  fi

  if [[ "${KEEP_COLIMA}" -eq 0 ]] && command -v colima >/dev/null 2>&1; then
    if colima status >/dev/null 2>&1; then
      echo "Stopping Colima..."
      colima stop
    else
      echo "Colima is not running."
    fi
  fi
fi
