#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/dist/release-artifacts"
VERSION=""
REPOSITORY=""
POSTGRES_IMAGE="postgres:18-alpine"

# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/release/render-deploy-artifacts.sh --version <tag> --repository <owner/repo> [options]

Render release-specific deploy artifacts with baked-in image references.

Options:
  --version <tag>          Required release tag (example: v1.2.3)
  --repository <owner/repo>
                           Required GitHub repository slug used for GHCR image paths
  --output-dir <path>      Output directory (default: ./dist/release-artifacts)
  --postgres-image <ref>   Override tested Postgres image reference
  -h, --help               Show this help
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --repository)
      REPOSITORY="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --postgres-image)
      POSTGRES_IMAGE="$2"
      shift 2
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
  log_fail "--version is required"
  exit 1
fi
if [[ -z "${REPOSITORY}" ]]; then
  log_fail "--repository is required"
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"

labtether_image="ghcr.io/${REPOSITORY}/labtether:${VERSION}"

template_path="${PROJECT_ROOT}/deploy/release/docker-compose.deploy.yml.tmpl"
if [[ ! -f "${template_path}" ]]; then
  log_fail "missing template: ${template_path}"
  exit 1
fi

compose_output="${OUTPUT_DIR}/docker-compose.deploy.yml"
env_output="${OUTPUT_DIR}/.env.deploy.example"
manifest_output="${OUTPUT_DIR}/deploy-manifest.json"

sed \
  -e "s|__LABTETHER_IMAGE__|${labtether_image}|g" \
  -e "s|__POSTGRES_IMAGE__|${POSTGRES_IMAGE}|g" \
  "${template_path}" > "${compose_output}"

cat > "${env_output}" <<EOF
# Deploy overrides for the ${VERSION} LabTether deploy artifact.
# This file is optional for the default install path.

LABTETHER_IMAGE=${labtether_image}

POSTGRES_DB=labtether
POSTGRES_USER=labtether
# Optional bootstrap overrides. Leave password blank to use the website setup flow.
# Runtime DB/owner/API/encryption secrets are generated automatically on first boot.
LABTETHER_ADMIN_USERNAME=admin
LABTETHER_ADMIN_PASSWORD=

LABTETHER_HTTP_BIND=127.0.0.1
LABTETHER_HTTPS_BIND=0.0.0.0
LABTETHER_CONSOLE_BIND=127.0.0.1
LABTETHER_TLS_MODE=auto
EOF

cat > "${manifest_output}" <<EOF
{
  "version": "${VERSION}",
  "repository": "${REPOSITORY}",
  "artifacts": {
    "docker_compose_deploy": "docker-compose.deploy.yml",
    "env_example": ".env.deploy.example"
  },
  "images": {
    "labtether": "${labtether_image}",
    "postgres": "${POSTGRES_IMAGE}"
  },
  "notes": {
    "all_in_one_available": true,
    "default_install_requires_env_overrides": false,
    "runtime_secrets_generated_on_first_boot": true
  }
}
EOF

log_pass "rendered release artifacts into ${OUTPUT_DIR}"
log_info "generated:"
printf '  %s\n' "${compose_output}" "${env_output}" "${manifest_output}"
