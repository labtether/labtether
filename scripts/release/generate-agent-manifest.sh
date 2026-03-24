#!/usr/bin/env bash
# generate-agent-manifest.sh — Query GitHub Releases for each agent repo,
# download Go agent binaries, and assemble agent-manifest.json.
#
# Usage: ./scripts/release/generate-agent-manifest.sh <hub-version> <output-dir>
#
# Requires: curl, jq, sha256sum (or shasum)
# Optional: GITHUB_TOKEN env var for private repos

set -euo pipefail

HUB_VERSION="${1:?Usage: $0 <hub-version> <output-dir>}"
OUTPUT_DIR="${2:?Usage: $0 <hub-version> <output-dir>}"

GITHUB_API="https://api.github.com"
AUTH_HEADER=""
if [[ -n "${GITHUB_TOKEN:-}" ]]; then
  AUTH_HEADER="Authorization: Bearer ${GITHUB_TOKEN}"
fi

gh_api() {
  local url="$1"
  if [[ -n "${AUTH_HEADER}" ]]; then
    curl -fsSL -H "${AUTH_HEADER}" -H "Accept: application/vnd.github+json" "${url}"
  else
    curl -fsSL -H "Accept: application/vnd.github+json" "${url}"
  fi
}

sha256_file() {
  sha256sum "$1" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
}

file_size() {
  stat -f%z "$1" 2>/dev/null || stat -c%s "$1" 2>/dev/null
}

mkdir -p "${OUTPUT_DIR}"

echo "=== Generating agent manifest for hub ${HUB_VERSION} ==="

# --- Go agent ---
GO_AGENT_REPO="labtether/labtether-agent"
echo "Fetching Go agent release..."
GO_RELEASE=$(gh_api "${GITHUB_API}/repos/${GO_AGENT_REPO}/releases/tags/${HUB_VERSION}" 2>/dev/null || \
             gh_api "${GITHUB_API}/repos/${GO_AGENT_REPO}/releases/latest")
GO_VERSION=$(echo "${GO_RELEASE}" | jq -r '.tag_name')
echo "  Go agent version: ${GO_VERSION}"

PLATFORMS=("linux-amd64:labtether-agent-linux-amd64" "linux-arm64:labtether-agent-linux-arm64" "windows-amd64:labtether-agent-windows-amd64.exe" "windows-arm64:labtether-agent-windows-arm64.exe")
GO_BINARIES_JSON="{"
FIRST=true

for platform_binary in "${PLATFORMS[@]}"; do
  PLATFORM="${platform_binary%%:*}"
  BINARY_NAME="${platform_binary#*:}"

  ASSET_URL=$(echo "${GO_RELEASE}" | jq -r --arg name "${BINARY_NAME}" '.assets[] | select(.name == $name) | .browser_download_url')
  if [[ -z "${ASSET_URL}" || "${ASSET_URL}" == "null" ]]; then
    echo "  WARNING: ${BINARY_NAME} not found in release ${GO_VERSION}, skipping"
    continue
  fi

  echo "  Downloading ${BINARY_NAME}..."
  if [[ -n "${AUTH_HEADER}" ]]; then
    curl -fsSL -H "${AUTH_HEADER}" -H "Accept: application/octet-stream" -o "${OUTPUT_DIR}/${BINARY_NAME}" -L "${ASSET_URL}"
  else
    curl -fsSL -o "${OUTPUT_DIR}/${BINARY_NAME}" -L "${ASSET_URL}"
  fi
  chmod 755 "${OUTPUT_DIR}/${BINARY_NAME}"

  SHA256=$(sha256_file "${OUTPUT_DIR}/${BINARY_NAME}")
  SIZE=$(file_size "${OUTPUT_DIR}/${BINARY_NAME}")

  if [[ "${FIRST}" != "true" ]]; then GO_BINARIES_JSON+=","; fi
  FIRST=false
  GO_BINARIES_JSON+="\"${PLATFORM}\":{\"name\":\"${BINARY_NAME}\",\"sha256\":\"${SHA256}\",\"size_bytes\":${SIZE},\"url\":\"https://github.com/${GO_AGENT_REPO}/releases/download/${GO_VERSION}/${BINARY_NAME}\"}"
done
GO_BINARIES_JSON+="}"

# --- macOS native agent (metadata only) ---
MAC_REPO="labtether/labtether-mac"
echo "Fetching macOS agent metadata..."
MAC_RELEASE=$(gh_api "${GITHUB_API}/repos/${MAC_REPO}/releases/tags/${HUB_VERSION}" 2>/dev/null || \
              gh_api "${GITHUB_API}/repos/${MAC_REPO}/releases/latest" 2>/dev/null || echo '{}')
MAC_VERSION=$(echo "${MAC_RELEASE}" | jq -r '.tag_name // "unknown"')
MAC_BINARY="labtether-agent-macos-universal.tar.gz"
MAC_URL=$(echo "${MAC_RELEASE}" | jq -r --arg name "${MAC_BINARY}" '(.assets // [])[] | select(.name == $name) | .browser_download_url // ""' 2>/dev/null || echo "")

MAC_SHA256_VAL=""
CHECKSUMS_URL=$(echo "${MAC_RELEASE}" | jq -r '(.assets // [])[] | select(.name | endswith("checksums.txt")) | .browser_download_url // ""' 2>/dev/null || echo "")
if [[ -n "${CHECKSUMS_URL}" && "${CHECKSUMS_URL}" != "null" && "${CHECKSUMS_URL}" != "" ]]; then
  MAC_SHA256_VAL=$(curl -fsSL "${CHECKSUMS_URL}" 2>/dev/null | grep "${MAC_BINARY}" | awk '{print $1}' || echo "")
fi
echo "  macOS agent version: ${MAC_VERSION}"

# --- Windows native agent (metadata only) ---
WIN_REPO="labtether/labtether-win"
echo "Fetching Windows agent metadata..."
WIN_RELEASE=$(gh_api "${GITHUB_API}/repos/${WIN_REPO}/releases/tags/${HUB_VERSION}" 2>/dev/null || \
              gh_api "${GITHUB_API}/repos/${WIN_REPO}/releases/latest" 2>/dev/null || echo '{}')
WIN_VERSION=$(echo "${WIN_RELEASE}" | jq -r '.tag_name // "unknown"')
WIN_BINARY="labtether-agent-win-x64.zip"
WIN_URL=$(echo "${WIN_RELEASE}" | jq -r --arg name "${WIN_BINARY}" '(.assets // [])[] | select(.name == $name) | .browser_download_url // ""' 2>/dev/null || echo "")

WIN_SHA256_VAL=""
WIN_CHECKSUMS_URL=$(echo "${WIN_RELEASE}" | jq -r '(.assets // [])[] | select(.name | endswith("checksums.txt")) | .browser_download_url // ""' 2>/dev/null || echo "")
if [[ -n "${WIN_CHECKSUMS_URL}" && "${WIN_CHECKSUMS_URL}" != "null" && "${WIN_CHECKSUMS_URL}" != "" ]]; then
  WIN_SHA256_VAL=$(curl -fsSL "${WIN_CHECKSUMS_URL}" 2>/dev/null | grep "${WIN_BINARY}" | awk '{print $1}' || echo "")
fi
echo "  Windows agent version: ${WIN_VERSION}"

# --- Assemble manifest ---
GENERATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

cat > "${OUTPUT_DIR}/agent-manifest.json" <<MANIFEST
{
  "schema_version": 1,
  "generated_at": "${GENERATED_AT}",
  "hub_version": "${HUB_VERSION}",
  "agents": {
    "labtether-agent": {
      "version": "${GO_VERSION}",
      "repo": "${GO_AGENT_REPO}",
      "binaries": ${GO_BINARIES_JSON}
    },
    "labtether-mac": {
      "version": "${MAC_VERSION}",
      "repo": "${MAC_REPO}",
      "type": "metadata-only",
      "binaries": {
        "darwin-universal": {
          "name": "${MAC_BINARY}",
          "sha256": "${MAC_SHA256_VAL}",
          "url": "${MAC_URL}"
        }
      }
    },
    "labtether-win": {
      "version": "${WIN_VERSION}",
      "repo": "${WIN_REPO}",
      "type": "metadata-only",
      "binaries": {
        "windows-x64": {
          "name": "${WIN_BINARY}",
          "sha256": "${WIN_SHA256_VAL}",
          "url": "${WIN_URL}"
        }
      }
    }
  }
}
MANIFEST

echo ""
echo "=== Manifest written to ${OUTPUT_DIR}/agent-manifest.json ==="
echo "Go agent binaries downloaded to ${OUTPUT_DIR}/"
ls -la "${OUTPUT_DIR}/"
