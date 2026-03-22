#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-${PROJECT_ROOT}/docker-compose.deploy.yml}"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env.deploy}"

VERSION="${LABTETHER_VERSION:-}"
ENABLE_REMOTE_DESKTOP=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/upgrade-compose.sh --version <tag> [options]

Upgrade an existing LabTether deploy compose install by pinning a new release
version, pulling images, and recreating the stack.

Options:
  --version <tag>          Required target LabTether release tag (example: v1.2.4)
  --with-remote-desktop    Enable the optional guacd profile during upgrade
  -h, --help               Show this help

Environment:
  COMPOSE_FILE             Compose file path (default: ./docker-compose.deploy.yml)
  ENV_FILE                 Deploy env file path (default: ./.env.deploy)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --with-remote-desktop)
      ENABLE_REMOTE_DESKTOP=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log_fail "unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${VERSION}" ]]; then
  log_fail "--version is required (example: ./scripts/upgrade-compose.sh --version v1.2.4)"
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  log_fail "missing deploy env file: ${ENV_FILE}"
  log_info "Run ./scripts/install-compose.sh first."
  exit 1
fi

if ! require_command docker; then
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  log_fail "docker compose plugin is required"
  exit 1
fi

upsert_env_value() {
  local key=$1
  local value=$2
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v k="$key" -v v="$value" -F= '
    BEGIN { written = 0 }
    $1 == k { print k "=" v; written = 1; next }
    { print }
    END { if (!written) print k "=" v }
  ' "${ENV_FILE}" > "${tmp_file}"
  mv "${tmp_file}" "${ENV_FILE}"
}

upsert_env_value "LABTETHER_VERSION" "${VERSION}"

compose_args=(--env-file "${ENV_FILE}" -f "${COMPOSE_FILE}")
if [[ "${ENABLE_REMOTE_DESKTOP}" -eq 1 ]]; then
  compose_args+=(--profile remote-desktop)
fi

cd "${PROJECT_ROOT}"

log_info "Pulling LabTether ${VERSION} images..."
docker compose "${compose_args[@]}" pull

log_info "Recreating deploy stack..."
docker compose "${compose_args[@]}" up -d

cat <<EOF

LabTether deploy upgrade complete.

- Env file: ${ENV_FILE}
- Current version pin: ${VERSION}
- Console: http://localhost:3000
- API health: http://localhost:8080/healthz
EOF
