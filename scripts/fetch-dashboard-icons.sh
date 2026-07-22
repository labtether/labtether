#!/usr/bin/env bash
set -euo pipefail
umask 077

# Fetch a pinned dashboard-icons archive, verify its digest, validate every
# extracted SVG as passive XML, and normalize it into the console bundle.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=/dev/null
source "${REPO_ROOT}/scripts/lib/script-common.sh"

ICONS_DIR="$REPO_ROOT/web/console/public/service-icons"
DASHBOARD_ICONS_COMMIT="${DASHBOARD_ICONS_COMMIT:-}"
DASHBOARD_ICONS_SHA256="${DASHBOARD_ICONS_SHA256:-}"

if [[ ! "$DASHBOARD_ICONS_COMMIT" =~ ^[0-9a-fA-F]{40}$ ]]; then
  log_fail "set DASHBOARD_ICONS_COMMIT to an exact 40-character upstream commit"
  exit 1
fi
if [[ ! "$DASHBOARD_ICONS_SHA256" =~ ^[0-9a-fA-F]{64}$ ]]; then
  log_fail "set DASHBOARD_ICONS_SHA256 to the expected SHA-256 of that commit archive"
  exit 1
fi
DASHBOARD_ICONS_COMMIT=$(printf '%s' "$DASHBOARD_ICONS_COMMIT" | tr '[:upper:]' '[:lower:]')
DASHBOARD_ICONS_SHA256=$(printf '%s' "$DASHBOARD_ICONS_SHA256" | tr '[:upper:]' '[:lower:]')

for cmd in awk curl install jq mktemp python3 sort stat; do
  require_command "$cmd" || exit 1
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  log_fail "required SHA-256 command not found: install sha256sum or shasum"
  exit 1
fi

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/labtether-dashboard-icons.XXXXXX")
cleanup() {
  rm -rf -- "$TMP_DIR"
}
trap cleanup EXIT INT TERM HUP

if [[ ! -d "$ICONS_DIR" || -L "$ICONS_DIR" ]]; then
  log_fail "icon destination must be a non-symlink directory: $ICONS_DIR"
  exit 1
fi
icons_owner=$(labtether_file_owner_uid "$ICONS_DIR" 2>/dev/null || true)
icons_mode=$(labtether_file_mode "$ICONS_DIR" 2>/dev/null || true)
if [[ "$icons_owner" != "$(id -u)" || ! "$icons_mode" =~ ^[0-7]{3,4}$ || "${icons_mode: -2:1}" =~ [2367] || "${icons_mode: -1}" =~ [2367] ]]; then
  log_fail "icon destination must be caller-owned and not group/other writable: $ICONS_DIR"
  exit 1
fi

archive="$TMP_DIR/dashboard-icons.tar.gz"
archive_url="https://github.com/walkxcode/dashboard-icons/archive/${DASHBOARD_ICONS_COMMIT}.tar.gz"
echo "Downloading pinned dashboard-icons archive ${DASHBOARD_ICONS_COMMIT}..."
labtether_build_curl_request_args "$archive_url" 0 || exit 1
labtether_curl "${LABTETHER_CURL_REQUEST_ARGS[@]}" --fail --silent --show-error --location \
  --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 \
  --max-filesize $((200 * 1024 * 1024)) "$archive_url" -o "$archive"

if command -v sha256sum >/dev/null 2>&1; then
  archive_sha256=$(sha256sum "$archive" | awk '{print $1}')
else
  archive_sha256=$(shasum -a 256 "$archive" | awk '{print $1}')
fi
if [[ "$archive_sha256" != "$DASHBOARD_ICONS_SHA256" ]]; then
  log_fail "dashboard-icons archive digest mismatch"
  exit 1
fi

SRC_DIR="$TMP_DIR/validated-svg"
mkdir -m 700 "$SRC_DIR"
python3 - "$archive" "$SRC_DIR" "$DASHBOARD_ICONS_COMMIT" <<'PY'
import os
import posixpath
import re
import sys
import tarfile
import xml.etree.ElementTree as ET

archive, destination, commit = sys.argv[1:]
expected_prefix = f"dashboard-icons-{commit}/svg/"
safe_name = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._ -]{0,199}\.svg$")
active_attribute = re.compile(r"^on[a-z]+$", re.I)
active_scheme = re.compile(r"^(?:javascript|vbscript|data):", re.I)
max_icon_bytes = 2 * 1024 * 1024
max_total_bytes = 200 * 1024 * 1024
max_archive_bytes = 2 * 1024 * 1024 * 1024
max_icons = 20000
max_members = 100000
total_bytes = 0
archive_bytes = 0
member_count = 0
seen = set()

def local_name(value):
    return value.rsplit("}", 1)[-1].lower()

with tarfile.open(archive, "r|gz") as bundle:
    for member in bundle:
        member_count += 1
        if member_count > max_members:
            raise SystemExit("dashboard-icons archive contains too many members")
        normalized = posixpath.normpath(member.name)
        if member.name.startswith("/") or normalized == ".." or normalized.startswith("../"):
            raise SystemExit(f"unsafe archive path: {member.name!r}")
        if member.issym() or member.islnk() or member.isdev() or member.isfifo():
            raise SystemExit(f"unsupported archive member type: {member.name!r}")
        if member.isfile():
            archive_bytes += member.size
            if archive_bytes > max_archive_bytes:
                raise SystemExit("dashboard-icons archive exceeds the uncompressed size limit")
        if not member.isfile() or not member.name.startswith(expected_prefix):
            continue
        relative = member.name[len(expected_prefix):]
        if "/" in relative or not safe_name.fullmatch(relative):
            raise SystemExit(f"unsafe SVG filename: {relative!r}")
        if member.size <= 0 or member.size > max_icon_bytes:
            raise SystemExit(f"invalid SVG size for {relative!r}")
        normalized_name = relative.lower().replace("_", "-").replace(" ", "-")
        if normalized_name in seen:
            raise SystemExit(f"normalized SVG filename collision: {normalized_name!r}")
        seen.add(normalized_name)
        total_bytes += member.size
        if len(seen) > max_icons or total_bytes > max_total_bytes:
            raise SystemExit("dashboard-icons archive exceeds extraction limits")
        source = bundle.extractfile(member)
        if source is None:
            raise SystemExit(f"could not read {member.name!r}")
        data = source.read(max_icon_bytes + 1)
        if len(data) != member.size or len(data) > max_icon_bytes:
            raise SystemExit(f"SVG changed size while reading: {member.name!r}")
        lowered = data.lower()
        if b"<!doctype" in lowered or b"<!entity" in lowered or b"<?xml-stylesheet" in lowered:
            raise SystemExit(f"DTD/entity/stylesheet declarations are forbidden: {member.name!r}")
        try:
            root = ET.fromstring(data)
        except ET.ParseError as exc:
            raise SystemExit(f"invalid SVG XML in {member.name!r}: {exc}") from exc
        if local_name(root.tag) != "svg":
            raise SystemExit(f"root element is not svg: {member.name!r}")
        for element in root.iter():
            if local_name(element.tag) in {
                "script", "style", "foreignobject", "iframe", "object", "embed", "audio", "video",
                "animate", "animatemotion", "animatetransform", "set",
            }:
                raise SystemExit(f"active SVG element in {member.name!r}")
            for attribute, value in element.attrib.items():
                attr_name = local_name(attribute)
                text = value.strip()
                if active_attribute.fullmatch(attr_name):
                    raise SystemExit(f"active SVG event attribute in {member.name!r}")
                if attr_name in {"href", "src"} and text and not text.startswith("#"):
                    raise SystemExit(f"external SVG reference in {member.name!r}")
                if active_scheme.match(text):
                    raise SystemExit(f"active SVG URL scheme in {member.name!r}")
                if "url(" in text.lower() and not re.fullmatch(r"url\(\s*#[A-Za-z0-9_.:-]+\s*\)", text, re.I):
                    raise SystemExit(f"external SVG CSS reference in {member.name!r}")

        output = os.path.join(destination, normalized_name)
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        descriptor = os.open(output, flags, 0o600)
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(data)

if not seen:
    raise SystemExit("archive contained no direct svg/ icon files")
PY

echo "Copying validated icons..."
count=0
for svg in "$SRC_DIR"/*.svg; do
  [[ -f "$svg" && ! -L "$svg" ]] || continue
  name=$(basename "$svg")
  destination="$ICONS_DIR/$name"
  if [[ -L "$destination" ]]; then
    log_fail "refusing to replace symlinked icon destination: $destination"
    exit 1
  fi
  temporary=$(mktemp "$ICONS_DIR/.${name}.tmp.XXXXXX")
  install -m 0644 "$svg" "$temporary"
  mv -f "$temporary" "$destination"
  count=$((count + 1))
done

echo "Generating index.json..."
index_tmp=$(mktemp "$ICONS_DIR/.index.json.tmp.XXXXXX")
for svg in "$ICONS_DIR"/*.svg; do
  [[ -f "$svg" && ! -L "$svg" ]] || continue
  key=$(basename "$svg" .svg)
  [[ "$key" == "_default" ]] && continue
  printf '%s\n' "$key"
done | LC_ALL=C sort | jq -R . | jq -s . >"$index_tmp"
chmod 0644 "$index_tmp"
mv -f "$index_tmp" "$ICONS_DIR/index.json"

total=$(jq 'length' "$ICONS_DIR/index.json")
echo "Done. Copied $count validated SVGs, $total icons indexed."
