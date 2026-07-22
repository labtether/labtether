#!/usr/bin/env bash
# generate-agent-manifest.sh — fetch and cryptographically verify coordinated
# agent releases before assembling the hub's agent-manifest.json.
#
# Usage: ./scripts/release/generate-agent-manifest.sh <exact-version-tag> <output-dir>

set -euo pipefail
set +x
set +a
umask 077

unset github_token
github_token="${GITHUB_TOKEN-}"
unset GITHUB_TOKEN
unset CURL_CA_BUNDLE SSL_CERT_FILE SSL_CERT_DIR SSLKEYLOGFILE
unset CURL_HOME NETRC CURL_SSL_BACKEND

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

RELEASE_TAG="${1:?Usage: $0 <exact-version-tag> <output-dir>}"
OUTPUT_DIR="${2:?Usage: $0 <exact-version-tag> <output-dir>}"
TRUSTED_PUBLIC_KEY_FILE="${LABTETHER_AGENT_RELEASE_TRUSTED_PUBLIC_KEY_FILE:-${SCRIPT_DIR}/agent-release-public-key.b64}"
GITHUB_API="https://api.github.com"
MAX_AGENT_BINARY_BYTES=$((100 * 1024 * 1024))
MAX_SMALL_ASSET_BYTES=$((1024 * 1024))
RELEASE_LOOKUP_ATTEMPTS="${LABTETHER_RELEASE_LOOKUP_ATTEMPTS:-60}"
RELEASE_LOOKUP_DELAY_SECONDS="${LABTETHER_RELEASE_LOOKUP_DELAY_SECONDS:-10}"

if [[ ! "${RELEASE_TAG}" =~ ^v[0-9A-Za-z][0-9A-Za-z._-]{0,127}$ ]]; then
  echo "generate-agent-manifest: release tag must start with v and contain only letters, digits, dot, underscore, or hyphen" >&2
  exit 2
fi
if [[ ! "${RELEASE_LOOKUP_ATTEMPTS}" =~ ^[0-9]+$ ]] || ((RELEASE_LOOKUP_ATTEMPTS < 1 || RELEASE_LOOKUP_ATTEMPTS > 120)); then
  echo "generate-agent-manifest: LABTETHER_RELEASE_LOOKUP_ATTEMPTS must be between 1 and 120" >&2
  exit 2
fi
if [[ ! "${RELEASE_LOOKUP_DELAY_SECONDS}" =~ ^[0-9]+$ ]] || ((RELEASE_LOOKUP_DELAY_SECONDS < 0 || RELEASE_LOOKUP_DELAY_SECONDS > 30)); then
  echo "generate-agent-manifest: LABTETHER_RELEASE_LOOKUP_DELAY_SECONDS must be between 0 and 30" >&2
  exit 2
fi
if [[ ! -f "${TRUSTED_PUBLIC_KEY_FILE}" || -L "${TRUSTED_PUBLIC_KEY_FILE}" ]]; then
  echo "generate-agent-manifest: trusted agent release public key must be a regular, non-symlink file" >&2
  exit 2
fi
key_owner="$(stat -f '%u' "${TRUSTED_PUBLIC_KEY_FILE}" 2>/dev/null || stat -c '%u' "${TRUSTED_PUBLIC_KEY_FILE}" 2>/dev/null || true)"
key_mode="$(stat -f '%Lp' "${TRUSTED_PUBLIC_KEY_FILE}" 2>/dev/null || stat -c '%a' "${TRUSTED_PUBLIC_KEY_FILE}" 2>/dev/null || true)"
if [[ "$key_owner" != "$(id -u)" && "$key_owner" != "0" ]]; then
  echo "generate-agent-manifest: trusted public key must be owned by the current user or root" >&2
  exit 2
fi
if [[ ! "$key_mode" =~ ^[0-7]{3,4}$ || "${key_mode: -2:1}" =~ [2367] || "${key_mode: -1}" =~ [2367] ]]; then
  echo "generate-agent-manifest: trusted public key must not be group/other writable" >&2
  exit 2
fi
if [[ -e "${OUTPUT_DIR}" && -L "${OUTPUT_DIR}" ]]; then
  echo "generate-agent-manifest: output directory must not be a symlink" >&2
  exit 2
fi

for required_tool in awk curl go install jq mktemp stat; do
  if ! command -v "${required_tool}" >/dev/null 2>&1; then
    echo "generate-agent-manifest: required tool is missing: ${required_tool}" >&2
    exit 2
  fi
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "generate-agent-manifest: required SHA-256 command is missing: install sha256sum or shasum" >&2
  exit 2
fi

STAGING_DIR="$(mktemp -d "${TMPDIR:-/tmp}/labtether-agent-release.XXXXXX")"
cleanup() {
  rm -rf "${STAGING_DIR}"
}
trap cleanup EXIT

mkdir -p "${OUTPUT_DIR}"
if [[ ! -d "${OUTPUT_DIR}" || -L "${OUTPUT_DIR}" ]]; then
  echo "generate-agent-manifest: output path must remain a non-symlink directory" >&2
  exit 2
fi
output_owner="$(stat -f '%u' "${OUTPUT_DIR}" 2>/dev/null || stat -c '%u' "${OUTPUT_DIR}" 2>/dev/null || true)"
output_mode="$(stat -f '%Lp' "${OUTPUT_DIR}" 2>/dev/null || stat -c '%a' "${OUTPUT_DIR}" 2>/dev/null || true)"
if [[ -z "$output_owner" || "$output_owner" != "$(id -u)" ]]; then
  echo "generate-agent-manifest: output directory must be owned by the current user" >&2
  exit 2
fi
if [[ ! "$output_mode" =~ ^[0-7]{3,4}$ || "${output_mode: -2:1}" =~ [2367] || "${output_mode: -1}" =~ [2367] ]]; then
  echo "generate-agent-manifest: output directory must not be group/other writable" >&2
  exit 2
fi

TRUSTED_PUBLIC_KEY_COPY="${STAGING_DIR}/trusted-agent-release-public-key.b64"
install -m 0600 "${TRUSTED_PUBLIC_KEY_FILE}" "${TRUSTED_PUBLIC_KEY_COPY}"

VERIFIER="${STAGING_DIR}/verify-agent-release"
(
  unset GOOS GOARCH
  go build -trimpath -o "${VERIFIER}" "${ROOT_DIR}/scripts/release/verify-agent-release"
)

curl_common=(
  --disable
  --fail
  --silent
  --show-error
  --proto '=https'
  --proto-redir '=https'
  --tlsv1.2
  --retry 3
  --retry-delay 1
  --retry-all-errors
  --connect-timeout 10
  --max-time 600
)
case "$(printf '%s' "${LABTETHER_ALLOW_PROXY:-0}" | tr '[:upper:]' '[:lower:]')" in
  1|true|yes|on) ;;
  *) curl_common+=(--noproxy '*') ;;
esac
gh_headers=(-H "Accept: application/vnd.github+json")
download_headers=(-H "Accept: application/octet-stream")
github_auth_args=()
if [[ -n "$github_token" ]]; then
  if [[ "$github_token" =~ [[:cntrl:]] ]]; then
    echo "generate-agent-manifest: GITHUB_TOKEN contains an invalid control character" >&2
    exit 2
  fi
  github_token_escaped=${github_token//\\/\\\\}
  github_token_escaped=${github_token_escaped//\"/\\\"}
  github_auth_config="${STAGING_DIR}/github-auth.curlrc"
  printf 'header = "Authorization: Bearer %s"\n' "$github_token_escaped" >"$github_auth_config"
  chmod 600 "$github_auth_config"
  github_auth_args=(--config "$github_auth_config")
  unset github_token github_token_escaped
fi

gh_api() {
  local url="$1"
  curl "${curl_common[@]}" \
    "${github_auth_args[@]}" \
    --max-filesize $((10 * 1024 * 1024)) \
    "${gh_headers[@]}" \
    -- "${url}"
}

download_url() {
  local url="$1"
  local destination="$2"
  local max_bytes="$3"
  curl "${curl_common[@]}" \
    "${github_auth_args[@]}" \
    --location \
    --max-redirs 5 \
    --max-filesize "${max_bytes}" \
    "${download_headers[@]}" \
    -o "${destination}" \
    -- "${url}"
}

sha256_file() {
  sha256sum "$1" 2>/dev/null | awk '{print $1}' || shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
}

file_size() {
  stat -f%z "$1" 2>/dev/null || stat -c%s "$1" 2>/dev/null
}

fetch_exact_release() {
  local repo="$1"
  local destination="$2"
  local label="$3"
  shift 3
  local required_assets=("$@")
  local candidate="${destination}.candidate"
  local api_url="${GITHUB_API}/repos/${repo}/releases/tags/${RELEASE_TAG}"
  local required_assets_json
  required_assets_json="$(printf '%s\n' "${required_assets[@]}" | jq -R . | jq -sc .)"

  for ((attempt = 1; attempt <= RELEASE_LOOKUP_ATTEMPTS; attempt++)); do
    if gh_api "${api_url}" >"${candidate}" && \
      jq -e --arg tag "${RELEASE_TAG}" --argjson required_assets "${required_assets_json}" '
        . as $release
        | ($release.tag_name == $tag)
          and ($release.draft == false)
          and ($release.assets | type == "array")
          and ($required_assets | all(. as $name |
            ([$release.assets[] | select(.name == $name and .state == "uploaded")] | length) == 1
            and ([$release.assets[] | select(.name == $name)] | length) == 1
          ))
      ' "${candidate}" >/dev/null; then
      mv "${candidate}" "${destination}"
      echo "generate-agent-manifest: found complete exact ${label} release ${RELEASE_TAG}" >&2
      return 0
    fi
    rm -f "${candidate}"
    if ((attempt < RELEASE_LOOKUP_ATTEMPTS)); then
      echo "generate-agent-manifest: waiting for complete exact ${label} release ${RELEASE_TAG} (${attempt}/${RELEASE_LOOKUP_ATTEMPTS})" >&2
      sleep "${RELEASE_LOOKUP_DELAY_SECONDS}"
    fi
  done

  echo "generate-agent-manifest: exact ${label} release ${RELEASE_TAG} is unavailable or incomplete; refusing to use latest/another tag" >&2
  return 1
}

asset_info() {
  local release_file="$1"
  local repo="$2"
  local asset_name="$3"
  local max_bytes="$4"
  local record
  record="$(jq -cer --arg name "${asset_name}" '
    [.assets[] | select(.name == $name and .state == "uploaded")]
    | if length == 1 then .[0] else error("release asset must occur exactly once") end
    | [.browser_download_url, (.size | tostring), .digest]
    | @tsv
  ' "${release_file}")"

  local url size digest
  IFS=$'\t' read -r url size digest <<<"${record}"
  local expected_url="https://github.com/${repo}/releases/download/${RELEASE_TAG}/${asset_name}"
  if [[ "${url}" != "${expected_url}" ]]; then
    echo "generate-agent-manifest: unexpected download URL for ${asset_name}" >&2
    return 1
  fi
  if [[ ! "${size}" =~ ^[0-9]+$ ]] || ((size < 1 || size > max_bytes)); then
    echo "generate-agent-manifest: invalid or excessive release asset size for ${asset_name}" >&2
    return 1
  fi
  if [[ ! "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    echo "generate-agent-manifest: missing or invalid GitHub SHA-256 digest for ${asset_name}" >&2
    return 1
  fi
  printf '%s\t%s\t%s\n' "${url}" "${size}" "${digest}"
}

download_verified_api_asset() {
  local url="$1"
  local expected_size="$2"
  local expected_digest="$3"
  local destination="$4"
  local max_bytes="$5"
  download_url "${url}" "${destination}" "${max_bytes}"
  local actual_size actual_digest
  actual_size="$(file_size "${destination}")"
  actual_digest="sha256:$(sha256_file "${destination}")"
  if [[ "${actual_size}" != "${expected_size}" || "${actual_digest}" != "${expected_digest}" ]]; then
    echo "generate-agent-manifest: downloaded release asset does not match GitHub digest and size" >&2
    return 1
  fi
}

read_single_checksum() {
  local checksum_file="$1"
  local expected_name="$2"
  local nonempty_lines
  nonempty_lines="$(awk 'NF { count++ } END { print count + 0 }' "${checksum_file}")"
  if [[ "${nonempty_lines}" != "1" ]]; then
    echo "generate-agent-manifest: checksum file for ${expected_name} must contain exactly one entry" >&2
    return 1
  fi
  local digest filename extra
  read -r digest filename extra < <(awk 'NF { print; exit }' "${checksum_file}")
  filename="${filename#\*}"
  digest="$(printf '%s' "${digest}" | tr '[:upper:]' '[:lower:]')"
  if [[ -n "${extra:-}" || "${filename}" != "${expected_name}" || ! "${digest}" =~ ^[0-9a-f]{64}$ ]]; then
    echo "generate-agent-manifest: invalid checksum entry for ${expected_name}" >&2
    return 1
  fi
  printf '%s\n' "${digest}"
}

publish_file() {
  local source="$1"
  local name="$2"
  local mode="$3"
  local temporary
  local destination="${OUTPUT_DIR}/${name}"
  if [[ ( -e "$destination" || -L "$destination" ) && ( ! -f "$destination" || -L "$destination" ) ]]; then
    echo "generate-agent-manifest: refusing unsafe existing output destination: ${destination}" >&2
    return 1
  fi
  temporary=$(mktemp "${OUTPUT_DIR}/.${name}.tmp.XXXXXX")
  install -m "${mode}" "${source}" "${temporary}"
  mv -f "${temporary}" "${destination}"
}

GO_REPO="labtether/labtether-agent"
MAC_REPO="labtether/labtether-mac"
WIN_REPO="labtether/labtether-win"
GO_RELEASE_FILE="${STAGING_DIR}/go-release.json"
MAC_RELEASE_FILE="${STAGING_DIR}/mac-release.json"
WIN_RELEASE_FILE="${STAGING_DIR}/win-release.json"

GO_PLATFORM_SPECS=(
  "linux-amd64:labtether-agent-linux-amd64:linux:amd64"
  "linux-arm64:labtether-agent-linux-arm64:linux:arm64"
  "windows-amd64:labtether-agent-windows-amd64.exe:windows:amd64"
  "windows-arm64:labtether-agent-windows-arm64.exe:windows:arm64"
)
GO_REQUIRED_ASSETS=()
for spec in "${GO_PLATFORM_SPECS[@]}"; do
  IFS=: read -r _ binary_name _ _ <<<"${spec}"
  GO_REQUIRED_ASSETS+=(
    "${binary_name}"
    "${binary_name}.sha256"
    "${binary_name}-${RELEASE_TAG}.sig"
    "${binary_name}-${RELEASE_TAG}.metadata.json"
  )
done

MAC_BINARY="labtether-agent-macos-universal.tar.gz"
WIN_BINARY="labtether-agent-win-x64.zip"

echo "generate-agent-manifest: resolving coordinated release ${RELEASE_TAG}" >&2
fetch_exact_release "${GO_REPO}" "${GO_RELEASE_FILE}" "Go agent" "${GO_REQUIRED_ASSETS[@]}" &
go_fetch_pid=$!
fetch_exact_release "${MAC_REPO}" "${MAC_RELEASE_FILE}" "macOS agent" "${MAC_BINARY}" "${MAC_BINARY}.sha256" &
mac_fetch_pid=$!
fetch_exact_release "${WIN_REPO}" "${WIN_RELEASE_FILE}" "Windows agent" "${WIN_BINARY}" "${WIN_BINARY}.sha256" &
win_fetch_pid=$!

fetch_failed=0
wait "${go_fetch_pid}" || fetch_failed=1
wait "${mac_fetch_pid}" || fetch_failed=1
wait "${win_fetch_pid}" || fetch_failed=1
if ((fetch_failed != 0)); then
  exit 1
fi

GO_BINARIES_JSON='{}'
for spec in "${GO_PLATFORM_SPECS[@]}"; do
  IFS=: read -r platform binary_name target_os target_arch <<<"${spec}"
  checksum_name="${binary_name}.sha256"
  signature_name="${binary_name}-${RELEASE_TAG}.sig"
  metadata_name="${binary_name}-${RELEASE_TAG}.metadata.json"

  IFS=$'\t' read -r binary_url binary_size binary_digest < <(asset_info "${GO_RELEASE_FILE}" "${GO_REPO}" "${binary_name}" "${MAX_AGENT_BINARY_BYTES}")
  IFS=$'\t' read -r checksum_url checksum_size checksum_digest < <(asset_info "${GO_RELEASE_FILE}" "${GO_REPO}" "${checksum_name}" "${MAX_SMALL_ASSET_BYTES}")
  IFS=$'\t' read -r signature_url signature_size signature_digest < <(asset_info "${GO_RELEASE_FILE}" "${GO_REPO}" "${signature_name}" "${MAX_SMALL_ASSET_BYTES}")
  IFS=$'\t' read -r metadata_url metadata_size metadata_digest < <(asset_info "${GO_RELEASE_FILE}" "${GO_REPO}" "${metadata_name}" "${MAX_SMALL_ASSET_BYTES}")

  binary_path="${STAGING_DIR}/${binary_name}"
  checksum_path="${STAGING_DIR}/${checksum_name}"
  signature_path="${STAGING_DIR}/${signature_name}"
  metadata_path="${STAGING_DIR}/${metadata_name}"
  download_verified_api_asset "${binary_url}" "${binary_size}" "${binary_digest}" "${binary_path}" "${MAX_AGENT_BINARY_BYTES}"
  download_verified_api_asset "${checksum_url}" "${checksum_size}" "${checksum_digest}" "${checksum_path}" "${MAX_SMALL_ASSET_BYTES}"
  download_verified_api_asset "${signature_url}" "${signature_size}" "${signature_digest}" "${signature_path}" "${MAX_SMALL_ASSET_BYTES}"
  download_verified_api_asset "${metadata_url}" "${metadata_size}" "${metadata_digest}" "${metadata_path}" "${MAX_SMALL_ASSET_BYTES}"

  verified_metadata="$(
    "${VERIFIER}" \
      --public-key-file "${TRUSTED_PUBLIC_KEY_COPY}" \
      --binary "${binary_path}" \
      --checksum "${checksum_path}" \
      --signature "${signature_path}" \
      --metadata "${metadata_path}" \
      --version "${RELEASE_TAG}" \
      --os "${target_os}" \
      --arch "${target_arch}" \
      --api-digest "${binary_digest}" \
      --api-size "${binary_size}"
  )"

  entry="$(jq -cn \
    --arg name "${binary_name}" \
    --arg sha256 "$(jq -r '.sha256' <<<"${verified_metadata}")" \
    --argjson size_bytes "$(jq -r '.size_bytes' <<<"${verified_metadata}")" \
    --arg url "${binary_url}" \
    --arg signature "$(jq -r '.signature' <<<"${verified_metadata}")" \
    '{name:$name,sha256:$sha256,size_bytes:$size_bytes,url:$url,signature:$signature}')"
  GO_BINARIES_JSON="$(jq -c --arg platform "${platform}" --argjson entry "${entry}" '. + {($platform): $entry}' <<<"${GO_BINARIES_JSON}")"
  echo "generate-agent-manifest: verified ${binary_name} checksum, metadata, and Ed25519 signature" >&2
done

build_native_entry() {
  local release_file="$1"
  local repo="$2"
  local binary_name="$3"
  local checksum_name="${binary_name}.sha256"

  local binary_url binary_size binary_digest
  local checksum_url checksum_size checksum_digest
  IFS=$'\t' read -r binary_url binary_size binary_digest < <(asset_info "${release_file}" "${repo}" "${binary_name}" "${MAX_AGENT_BINARY_BYTES}")
  IFS=$'\t' read -r checksum_url checksum_size checksum_digest < <(asset_info "${release_file}" "${repo}" "${checksum_name}" "${MAX_SMALL_ASSET_BYTES}")
  local checksum_path="${STAGING_DIR}/${repo##*/}-${checksum_name}"
  download_verified_api_asset "${checksum_url}" "${checksum_size}" "${checksum_digest}" "${checksum_path}" "${MAX_SMALL_ASSET_BYTES}"
  local published_checksum
  published_checksum="$(read_single_checksum "${checksum_path}" "${binary_name}")"
  if [[ "sha256:${published_checksum}" != "${binary_digest}" ]]; then
    echo "generate-agent-manifest: ${binary_name} checksum does not match the GitHub asset digest" >&2
    return 1
  fi
  jq -cn \
    --arg name "${binary_name}" \
    --arg sha256 "${published_checksum}" \
    --argjson size_bytes "${binary_size}" \
    --arg url "${binary_url}" \
    '{name:$name,sha256:$sha256,size_bytes:$size_bytes,url:$url}'
}

MAC_ENTRY="$(build_native_entry "${MAC_RELEASE_FILE}" "${MAC_REPO}" "${MAC_BINARY}")"
WIN_ENTRY="$(build_native_entry "${WIN_RELEASE_FILE}" "${WIN_REPO}" "${WIN_BINARY}")"

GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
jq -n \
  --arg generated_at "${GENERATED_AT}" \
  --arg hub_version "${RELEASE_TAG}" \
  --arg go_repo "${GO_REPO}" \
  --arg mac_repo "${MAC_REPO}" \
  --arg win_repo "${WIN_REPO}" \
  --argjson go_binaries "${GO_BINARIES_JSON}" \
  --argjson mac_entry "${MAC_ENTRY}" \
  --argjson win_entry "${WIN_ENTRY}" \
  '{
    schema_version: 1,
    generated_at: $generated_at,
    hub_version: $hub_version,
    agents: {
      "labtether-agent": {
        version: $hub_version,
        repo: $go_repo,
        binaries: $go_binaries
      },
      "labtether-mac": {
        version: $hub_version,
        repo: $mac_repo,
        type: "metadata-only",
        binaries: {
          "darwin-universal": $mac_entry
        }
      },
      "labtether-win": {
        version: $hub_version,
        repo: $win_repo,
        type: "metadata-only",
        binaries: {
          "windows-x64": $win_entry
        }
      }
    }
  }' >"${STAGING_DIR}/agent-manifest.json"

jq -e '
  .schema_version == 1
  and (.agents["labtether-agent"].binaries | length == 4)
  and ([.agents["labtether-agent"].binaries[] | has("signature") and (.sha256 | length == 64)] | all)
  and (.agents["labtether-mac"].binaries["darwin-universal"].sha256 | length == 64)
  and (.agents["labtether-win"].binaries["windows-x64"].sha256 | length == 64)
' "${STAGING_DIR}/agent-manifest.json" >/dev/null

for spec in "${GO_PLATFORM_SPECS[@]}"; do
  IFS=: read -r _ binary_name _ _ <<<"${spec}"
  publish_file "${STAGING_DIR}/${binary_name}" "${binary_name}" 0755
done
publish_file "${STAGING_DIR}/agent-manifest.json" "agent-manifest.json" 0644

echo "generate-agent-manifest: verified manifest written to ${OUTPUT_DIR}/agent-manifest.json" >&2
