#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env}"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

if [[ $# -gt 0 ]]; then
  case "$1" in
    -h|--help)
      cat <<'USAGE'
Usage: scripts/dev-backend-run.sh

Build and run the local LabTether backend using literal, allowlisted values
from the private .env file. This is the foreground command behind
`make dev-backend`.
USAGE
      exit 0
      ;;
    *)
      log_fail "dev-backend-run does not accept arguments"
      exit 2
      ;;
  esac
fi

require_command go || exit 1

# Preserve the application-facing development configuration while refusing
# process-loader and shell-control variables from dotenv files.
dotenv_allowlist='^(LABTETHER_[A-Z0-9_]+|DATABASE_URL|POSTGRES_[A-Z0-9_]+|API_PORT|APP_VERSION|JOB_WORKERS|STRUCTURED_ENABLED|INTERACTIVE_ENABLED|CONNECTOR_ENABLED|QUEUE_MAX_DELIVERIES|JOB_POLL_INTERVAL|SCHEDULE_POLL_INTERVAL|RETENTION_APPLY_INTERVAL|GUACD_(HOST|PORT)|HA_[A-Z0-9_]+|PBS_[A-Z0-9_]+|PORTAINER_[A-Z0-9_]+|PROXMOX_[A-Z0-9_]+|SMTP_[A-Z0-9_]+|APNS_[A-Z0-9_]+)$'
if [[ -e "$ENV_FILE" || -L "$ENV_FILE" ]]; then
  labtether_load_env_file_literals "$ENV_FILE" "$dotenv_allowlist" || exit 1
fi

DATABASE_URL="${DATABASE_URL:-postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable}"
# Blank owner/admin values deliberately delegate first-run generation/setup to
# the backend; development must never fall back to a published credential.
LABTETHER_OWNER_TOKEN="${LABTETHER_OWNER_TOKEN-}"
LABTETHER_ADMIN_PASSWORD="${LABTETHER_ADMIN_PASSWORD-}"
LABTETHER_ENCRYPTION_KEY="${LABTETHER_ENCRYPTION_KEY:?LABTETHER_ENCRYPTION_KEY must be set (generate: openssl rand -base64 32)}"
LABTETHER_TLS_MODE="${LABTETHER_TLS_MODE:-auto}"
API_PORT="${API_PORT:-8080}"

if [[ ! "$API_PORT" =~ ^[0-9]+$ ]] || ((API_PORT < 1 || API_PORT > 65535)); then
  log_fail "API_PORT must be an integer between 1 and 65535"
  exit 1
fi
if [[ ! "$LABTETHER_TLS_MODE" =~ ^[A-Za-z0-9._-]+$ ]]; then
  log_fail "LABTETHER_TLS_MODE contains unsupported characters"
  exit 1
fi

cd "$PROJECT_ROOT"
log_info "Building Go backend..."
mkdir -p build
go build -o build/labtether ./cmd/labtether

log_info "Starting backend on :${API_PORT} (Postgres must be running)..."
exec env \
  DATABASE_URL="$DATABASE_URL" \
  LABTETHER_OWNER_TOKEN="$LABTETHER_OWNER_TOKEN" \
  LABTETHER_ADMIN_PASSWORD="$LABTETHER_ADMIN_PASSWORD" \
  LABTETHER_ENCRYPTION_KEY="$LABTETHER_ENCRYPTION_KEY" \
  LABTETHER_TLS_MODE="$LABTETHER_TLS_MODE" \
  API_PORT="$API_PORT" \
  ./build/labtether
