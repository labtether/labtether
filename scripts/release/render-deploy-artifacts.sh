#!/usr/bin/env bash
set -Eeuo pipefail
set +x
set +a
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/dist/release-artifacts"
VERSION=""
REPOSITORY=""
IMAGE_DIGEST=""
POSTGRES_IMAGE="postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
GUACD_IMAGE="guacamole/guacd:1.6.0@sha256:8974eaa9ba32f713daf311e7cc8cd7e4cdfba1edea39eed75524e78ef4b08f4f"

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
  --image-digest <sha256>  Required pushed multi-platform image digest
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
    --image-digest)
      IMAGE_DIGEST="$2"
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
if [[ ! "${VERSION}" =~ ^v[0-9A-Za-z._-]+$ ]]; then
  log_fail "version contains unsupported characters"
  exit 1
fi
if [[ ! "${REPOSITORY}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  log_fail "repository must be an owner/repo slug"
  exit 1
fi
if [[ -z "${IMAGE_DIGEST}" ]]; then
  log_fail "--image-digest is required"
  exit 1
fi
if [[ ! "${IMAGE_DIGEST}" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  log_fail "image digest must be a sha256 OCI digest"
  exit 1
fi
if [[ ! "${POSTGRES_IMAGE}" =~ ^[A-Za-z0-9._/@:-]+$ ]]; then
  log_fail "postgres image contains unsupported characters"
  exit 1
fi

for command_name in chmod mktemp mv sed; do
  require_command "$command_name" || exit 1
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  log_fail "required SHA-256 command not found: install sha256sum or shasum"
  exit 1
fi
labtether_prepare_owned_output_dir "$OUTPUT_DIR" "release artifact output directory" || exit 1

normalized_repository="$(printf '%s' "${REPOSITORY}" | tr '[:upper:]' '[:lower:]')"
labtether_image="ghcr.io/${normalized_repository}:${VERSION}@${IMAGE_DIGEST}"

template_path="${PROJECT_ROOT}/deploy/release/docker-compose.deploy.yml.tmpl"
if [[ ! -f "${template_path}" ]]; then
  log_fail "missing template: ${template_path}"
  exit 1
fi

compose_output="${OUTPUT_DIR}/docker-compose.deploy.yml"
env_output="${OUTPUT_DIR}/.env.deploy.example"
manifest_output="${OUTPUT_DIR}/deploy-manifest.json"
checksums_output="${OUTPUT_DIR}/SHA256SUMS"

for destination in "$compose_output" "$env_output" "$manifest_output" "$checksums_output"; do
  if [[ ( -e "$destination" || -L "$destination" ) && ( ! -f "$destination" || -L "$destination" ) ]]; then
    log_fail "release artifact destination must be a regular non-symlink file when it exists: $destination"
    exit 1
  fi
done

compose_tmp=$(mktemp "${OUTPUT_DIR}/.docker-compose.deploy.yml.tmp.XXXXXX")
env_tmp=$(mktemp "${OUTPUT_DIR}/.env.deploy.example.tmp.XXXXXX")
manifest_tmp=$(mktemp "${OUTPUT_DIR}/.deploy-manifest.json.tmp.XXXXXX")
checksums_tmp=$(mktemp "${OUTPUT_DIR}/.SHA256SUMS.tmp.XXXXXX")
cleanup() {
  local temporary=""
  for temporary in "$compose_tmp" "$env_tmp" "$manifest_tmp" "$checksums_tmp"; do
    [[ -z "$temporary" ]] || rm -f -- "$temporary"
  done
}
trap cleanup EXIT INT TERM HUP

sed \
  -e "s|__LABTETHER_IMAGE__|${labtether_image}|g" \
  -e "s|__POSTGRES_IMAGE__|${POSTGRES_IMAGE}|g" \
  -e "s|__GUACD_IMAGE__|${GUACD_IMAGE}|g" \
  "${template_path}" > "${compose_tmp}"

cat > "${env_tmp}" <<EOF
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
LABTETHER_HTTPS_BIND=127.0.0.1
LABTETHER_CONSOLE_BIND=127.0.0.1
LABTETHER_CONSOLE_INGRESS_MAX_CONNECTIONS=512
LABTETHER_TLS_MODE=auto
# Internal routing follows LABTETHER_TLS_MODE. Override only when an external
# certificate uses a DNS identity other than the Compose service hostname.
# LABTETHER_INTERNAL_API_BASE_URL=https://hub.internal.example:8443
# LABTETHER_INTERNAL_CONNECT_HOST=labtether
# LABTETHER_INTERNAL_CA_FILE=/ca/ca.crt
# NEXT_PUBLIC_HUB_API_PORT=8443
LABTETHER_TRUST_PROXY_HOPS=0
LABTETHER_OUTBOUND_ALLOW_PRIVATE=true
LABTETHER_OUTBOUND_ALLOW_LOOPBACK=false
LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL=false
EOF

cat > "${manifest_tmp}" <<EOF
{
  "version": "${VERSION}",
  "repository": "${REPOSITORY}",
  "artifacts": {
    "docker_compose_deploy": "docker-compose.deploy.yml",
    "env_example": ".env.deploy.example"
  },
  "images": {
    "labtether": "${labtether_image}",
    "postgres": "${POSTGRES_IMAGE}",
    "guacd": "${GUACD_IMAGE}"
  },
  "notes": {
    "compose_minimum_version": "2.33.1",
    "all_in_one_available": true,
    "default_install_requires_env_overrides": false,
    "runtime_secrets_generated_on_first_boot": true
  }
}
EOF

chmod 0644 "$compose_tmp" "$env_tmp" "$manifest_tmp"
mv -f "$compose_tmp" "$compose_output"
mv -f "$env_tmp" "$env_output"
mv -f "$manifest_tmp" "$manifest_output"
compose_tmp=""
env_tmp=""
manifest_tmp=""

(
  cd "${OUTPUT_DIR}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum docker-compose.deploy.yml .env.deploy.example deploy-manifest.json > "$checksums_tmp"
  else
    shasum -a 256 docker-compose.deploy.yml .env.deploy.example deploy-manifest.json > "$checksums_tmp"
  fi
)
chmod 0644 "$checksums_tmp"
mv -f "$checksums_tmp" "$checksums_output"
checksums_tmp=""

log_pass "rendered release artifacts into ${OUTPUT_DIR}"
log_info "generated:"
printf '  %s\n' "${compose_output}" "${env_output}" "${manifest_output}" "$checksums_output"
