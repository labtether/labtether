#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<USAGE
Usage: $(basename "$0") --version <version> --image-prefix <ghcr-image-prefix> [--output-dir <path>]

Builds a Home Assistant add-on repository layout and release artifact tarball.

Options:
  --version <version>          Add-on version (for example: 1.2.3)
  --image-prefix <prefix>      Image prefix without arch suffix
                               (for example: ghcr.io/labtether/labtether-homeassistant-addon)
  --output-dir <path>          Output directory for repository layout
                               (default: ./dist/ha-addon-repository)
  -h, --help                   Show help
USAGE
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_DIR="${ROOT_DIR}/integrations/homeassistant/addon/labtether"
SOURCE_REPOSITORY_META="${ROOT_DIR}/integrations/homeassistant/addon/repository.yaml"
OUTPUT_DIR="${ROOT_DIR}/dist/ha-addon-repository"
VERSION=""
IMAGE_PREFIX=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --image-prefix)
      IMAGE_PREFIX="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "${VERSION}" ]]; then
  echo "--version is required" >&2
  usage >&2
  exit 1
fi
if [[ -z "${IMAGE_PREFIX}" ]]; then
  echo "--image-prefix is required" >&2
  usage >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi
if [[ ! -f "${SOURCE_REPOSITORY_META}" ]]; then
  echo "missing repository metadata: ${SOURCE_REPOSITORY_META}" >&2
  exit 1
fi
if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "missing addon source: ${SOURCE_DIR}" >&2
  exit 1
fi

rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}/labtether"

cp "${SOURCE_REPOSITORY_META}" "${OUTPUT_DIR}/repository.yaml"
cp -R "${SOURCE_DIR}/." "${OUTPUT_DIR}/labtether/"

CONFIG_FILE="${OUTPUT_DIR}/labtether/config.json"
if [[ ! -f "${CONFIG_FILE}" ]]; then
  echo "missing config.json in packaged addon" >&2
  exit 1
fi

TMP_CONFIG="$(mktemp)"
jq \
  --arg version "${VERSION}" \
  --arg image "${IMAGE_PREFIX}-{arch}" \
  '.version = $version
   | .image = $image' \
  "${CONFIG_FILE}" > "${TMP_CONFIG}"
mv "${TMP_CONFIG}" "${CONFIG_FILE}"

REPO_NAME="$(awk -F': ' '/^name:/ {print $2; exit}' "${OUTPUT_DIR}/repository.yaml")"
if [[ -z "${REPO_NAME}" ]]; then
  REPO_NAME="LabTether Add-ons"
fi

ADDON_META="${OUTPUT_DIR}/repo-index.json"
TMP_INDEX="$(mktemp)"
jq -n \
  --arg generated_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
  --arg repository_name "${REPO_NAME}" \
  --arg version "${VERSION}" \
  --argjson addon "$(jq '. + {path:"labtether"}' "${CONFIG_FILE}")" \
  '{
    generated_at: $generated_at,
    repository_name: $repository_name,
    version: $version,
    addons: [$addon]
  }' > "${TMP_INDEX}"
mv "${TMP_INDEX}" "${ADDON_META}"

ARTIFACT_PATH="${ROOT_DIR}/dist/labtether-ha-addon-repository-${VERSION}.tar.gz"
mkdir -p "${ROOT_DIR}/dist"
tar -C "${OUTPUT_DIR}" -czf "${ARTIFACT_PATH}" .

cat <<SUMMARY
Home Assistant add-on repository packaged.
- output_dir: ${OUTPUT_DIR}
- index: ${ADDON_META}
- artifact: ${ARTIFACT_PATH}
SUMMARY
