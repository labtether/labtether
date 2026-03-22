package agents

func agentBootstrapScriptTemplate() string {
	return `#!/bin/bash
set -euo pipefail

# LabTether Agent TLS Bootstrap
# Usage:
#   curl -kfsSL "%[1]s/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=%[2]s" | sudo bash
#   curl -kfsSL "%[1]s/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=%[2]s" | sudo bash -s -- --enrollment-token <token>

HUB_URL="%[1]s"
EXPECTED_CA_FINGERPRINT="%[2]s"
CA_DEST="/etc/labtether/ca.crt"
TMP_CA="$(mktemp)"
TMP_INSTALL="$(mktemp)"

cleanup() {
  rm -f "${TMP_CA}" "${TMP_INSTALL}"
}
trap cleanup EXIT

if [[ "${EUID}" -ne 0 ]]; then
  echo "Error: this script must be run as root (use sudo)." >&2
  exit 1
fi

DOWNLOAD_TOOL=""
if command -v curl >/dev/null 2>&1; then
  DOWNLOAD_TOOL="curl"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOAD_TOOL="wget"
else
  echo "Error: curl or wget is required." >&2
  exit 1
fi

download_insecure() {
  local url="$1"
  local out="$2"
  if [[ "${DOWNLOAD_TOOL}" == "curl" ]]; then
    curl -kfsSL --output "${out}" "${url}"
  else
    wget --no-check-certificate -q -O "${out}" "${url}"
  fi
}

download_with_ca() {
  local url="$1"
  local out="$2"
  if [[ "${DOWNLOAD_TOOL}" == "curl" ]]; then
    curl --cacert "${CA_DEST}" -fsSL --output "${out}" "${url}"
  else
    wget --ca-certificate="${CA_DEST}" -q -O "${out}" "${url}"
  fi
}

echo "Downloading LabTether CA certificate..."
download_insecure "${HUB_URL}/api/v1/ca.crt" "${TMP_CA}"

if ! command -v openssl >/dev/null 2>&1; then
  echo "Error: openssl is required for CA fingerprint verification." >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_CA_FINGERPRINT="$(openssl x509 -in "${TMP_CA}" -outform DER | sha256sum | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_CA_FINGERPRINT="$(openssl x509 -in "${TMP_CA}" -outform DER | shasum -a 256 | awk '{print $1}')"
else
  echo "Error: sha256sum or shasum is required for CA fingerprint verification." >&2
  exit 1
fi

ACTUAL_CA_FINGERPRINT="${ACTUAL_CA_FINGERPRINT,,}"
if [[ "${ACTUAL_CA_FINGERPRINT}" != "${EXPECTED_CA_FINGERPRINT}" ]]; then
  echo "Error: hub CA fingerprint mismatch." >&2
  echo "Expected: ${EXPECTED_CA_FINGERPRINT}" >&2
  echo "Actual:   ${ACTUAL_CA_FINGERPRINT}" >&2
  exit 1
fi

install -d -m 0755 /etc/labtether
install -m 0644 "${TMP_CA}" "${CA_DEST}"
echo "Saved CA certificate to ${CA_DEST}"

if command -v update-ca-certificates >/dev/null 2>&1; then
  install -m 0644 "${TMP_CA}" /usr/local/share/ca-certificates/labtether-ca.crt
  update-ca-certificates
elif command -v update-ca-trust >/dev/null 2>&1; then
  install -m 0644 "${TMP_CA}" /etc/pki/ca-trust/source/anchors/labtether-ca.crt
  update-ca-trust extract
fi

echo "Downloading trusted installer..."
download_with_ca "${HUB_URL}/install.sh" "${TMP_INSTALL}"
chmod 755 "${TMP_INSTALL}"

echo "Running installer with pinned CA trust..."
bash "${TMP_INSTALL}" --tls-ca-file "${CA_DEST}" "$@"

`
}
