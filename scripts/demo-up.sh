#!/usr/bin/env bash
set -euo pipefail

# ────────────────────────────────────────────────────────────────
# demo-up.sh — One-command LabTether demo launcher
#
# Usage:
#   ./scripts/demo-up.sh              # Start/restart the demo
#   ./scripts/demo-up.sh --fresh      # Nuke everything and start fresh
#   ./scripts/demo-up.sh --down       # Tear down the demo
#
# Prerequisites:
#   - Docker with compose v2
#   - Authenticated to ghcr.io (if image is private):
#       echo "$PAT" | docker login ghcr.io -u USERNAME --password-stdin
# ────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

ENV_FILE="$REPO_ROOT/.env.demo"
BASE_COMPOSE="$REPO_ROOT/deploy/compose/docker-compose.deploy.yml"
DEMO_COMPOSE="$REPO_ROOT/docker-compose.demo.yml"

COMPOSE_CMD=(docker compose --env-file "$ENV_FILE" -f "$BASE_COMPOSE" -f "$DEMO_COMPOSE")

cd "$REPO_ROOT"

# ── Preflight checks ───────────────────────────────────────────

if ! command -v docker &>/dev/null; then
  echo "Error: docker is not installed." >&2
  exit 1
fi

if ! docker compose version &>/dev/null; then
  echo "Error: docker compose v2 is required." >&2
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: $ENV_FILE not found. Are you in the hub repo root?" >&2
  exit 1
fi

if [[ ! -f "$BASE_COMPOSE" ]]; then
  echo "Error: $BASE_COMPOSE not found." >&2
  exit 1
fi

if [[ ! -f "$DEMO_COMPOSE" ]]; then
  echo "Error: $DEMO_COMPOSE not found." >&2
  exit 1
fi

# ── Parse args ─────────────────────────────────────────────────

ACTION="up"
for arg in "$@"; do
  case "$arg" in
    --fresh) ACTION="fresh" ;;
    --down)  ACTION="down" ;;
    --help|-h)
      echo "Usage: $0 [--fresh | --down]"
      echo ""
      echo "  (no args)   Start or restart the demo"
      echo "  --fresh     Nuke volumes/images and start from scratch"
      echo "  --down      Tear down the demo completely"
      exit 0
      ;;
    *) echo "Unknown arg: $arg"; exit 1 ;;
  esac
done

# ── Actions ────────────────────────────────────────────────────

case "$ACTION" in
  down)
    echo "Tearing down demo..."
    "${COMPOSE_CMD[@]}" down -v --remove-orphans
    echo "Demo stopped and volumes removed."
    ;;

  fresh)
    echo "Nuking old demo (containers, volumes, images)..."
    "${COMPOSE_CMD[@]}" down -v --rmi all --remove-orphans 2>/dev/null || true
    echo ""
    echo "Starting fresh demo..."
    "${COMPOSE_CMD[@]}" up -d --pull always
    ;;

  up)
    echo "Starting demo..."
    "${COMPOSE_CMD[@]}" up -d --pull always
    ;;
esac

if [[ "$ACTION" != "down" ]]; then
  echo ""
  echo "Waiting for hub to become healthy..."
  for i in $(seq 1 30); do
    if "${COMPOSE_CMD[@]}" ps --format json 2>/dev/null | grep -q '"Health":"healthy"'; then
      break
    fi
    sleep 2
  done

  echo ""
  "${COMPOSE_CMD[@]}" ps
  echo ""
  echo "Demo is running on port 80."
  echo "Visitors are auto-logged in with a read-only session."
fi
