#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml}"
ENV_FILE="${ENV_FILE:-${PROJECT_ROOT}/.env.deploy}"
ENV_TEMPLATE="${ENV_TEMPLATE:-${PROJECT_ROOT}/.env.deploy.example}"

VERSION="${LABTETHER_VERSION:-}"
ENABLE_REMOTE_DESKTOP=0
BUILD_LOCAL=0

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/install-compose.sh --version <tag> [options]

Prepare a simple end-user Docker Compose deployment:
1) creates .env.deploy from .env.deploy.example when missing,
2) pins the requested LabTether release version,
3) lets Compose generate the managed database password on first boot,
4) pulls and starts the deploy compose stack.

Options:
  --version <tag>          Required LabTether release tag (example: v1.2.3)
  --build                  Build images locally instead of pulling from registry
  --with-remote-desktop    Enable the optional guacd profile during install
  -h, --help               Show this help

Environment:
  COMPOSE_FILE             Compose file path (default: ./deploy/compose/docker-compose.deploy.yml)
  ENV_FILE                 Deploy env file path (default: ./.env.deploy)
  ENV_TEMPLATE             Deploy env template path (default: ./.env.deploy.example)
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --build)
      BUILD_LOCAL=1
      shift
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
  log_fail "--version is required (example: ./scripts/install-compose.sh --version v1.2.3)"
  exit 1
fi

if ! require_command docker; then
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  log_fail "docker compose plugin is required"
  exit 1
fi
ensure_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    return 0
  fi
  if [[ ! -f "${ENV_TEMPLATE}" ]]; then
    log_fail "missing env template: ${ENV_TEMPLATE}"
    return 1
  fi
  cp "${ENV_TEMPLATE}" "${ENV_FILE}"
  chmod 600 "${ENV_FILE}"
  log_pass "created ${ENV_FILE} from ${ENV_TEMPLATE}"
}

read_env_value() {
  local key=$1
  local line
  line=$(grep -E "^${key}=" "${ENV_FILE}" | head -n 1 || true)
  printf '%s' "${line#*=}"
}

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
  chmod 600 "${ENV_FILE}"
}

is_placeholder() {
  local value=$1
  [[ -z "${value// }" || "${value}" == REPLACE_WITH_* || "${value}" == vX.Y.Z ]]
}

ensure_env_file

upsert_env_value "LABTETHER_VERSION" "${VERSION}"

log_info "Prepared ${ENV_FILE} for LabTether ${VERSION}"

admin_username="$(read_env_value "LABTETHER_ADMIN_USERNAME")"
if is_placeholder "${admin_username}"; then
  admin_username="admin"
fi
admin_password="$(read_env_value "LABTETHER_ADMIN_PASSWORD")"
admin_password_seeded=1
if is_placeholder "${admin_password}"; then
  admin_password_seeded=0
fi

compose_args=(--env-file "${ENV_FILE}" -f "${COMPOSE_FILE}")
if [[ "${ENABLE_REMOTE_DESKTOP}" -eq 1 ]]; then
  compose_args+=(--profile remote-desktop)
fi

cd "${PROJECT_ROOT}"

if [[ "${BUILD_LOCAL}" -eq 1 ]]; then
  # When --build is used, override the image references to use local builds.
  # The deploy compose expects pre-built images via env vars; we point them at
  # locally-built tags and add the build compose overlay.
  local_hub_image="labtether/hub:${VERSION}"
  local_web_image="labtether/web-console:${VERSION}"
  upsert_env_value "LABTETHER_HUB_IMAGE" "${local_hub_image}"
  upsert_env_value "LABTETHER_WEB_IMAGE" "${local_web_image}"

  log_info "Building hub image (${local_hub_image})..."
  docker build -t "${local_hub_image}" \
    --build-arg SERVICE_DIR=cmd/labtether \
    --build-arg APP_VERSION="${VERSION}" \
    -f build/go-service.Dockerfile .

  log_info "Building web console image (${local_web_image})..."
  docker build -t "${local_web_image}" \
    -f web/console/Dockerfile web/console/

  log_info "Pulling third-party images..."
  docker compose "${compose_args[@]}" pull postgres
else
  log_info "Pulling release images..."
  docker compose "${compose_args[@]}" pull
fi

log_info "Starting deploy stack..."
docker compose "${compose_args[@]}" up -d

cat <<EOF

LabTether deploy install complete.

- Env file: ${ENV_FILE}
- Console: http://localhost:3000
- API health: http://localhost:8080/healthz
- HTTPS hub: https://localhost:8443

Admin login:
- Username: ${admin_username}
EOF

if [[ "${admin_password_seeded}" -eq 1 ]]; then
  cat <<EOF
- Password source: LABTETHER_ADMIN_PASSWORD in ${ENV_FILE}
EOF
else
  cat <<EOF
- Finish setup in the browser: http://localhost:3000/setup
EOF
fi

cat <<EOF

Runtime owner/API/encryption secrets are generated automatically on first boot
and persisted in the LabTether data volume.
The split-Postgres password is generated automatically into the bootstrap volume
and can be revealed later in Settings by an admin.

If your browser warns about the self-signed cert, install the generated CA:
https://localhost:8443/api/v1/tls/ca.crt
EOF
