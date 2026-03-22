#!/usr/bin/env bash
set -euo pipefail

# Fetch dashboard-icons SVGs from walkxcode/dashboard-icons and normalize into
# web/console/public/service-icons/.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ICONS_DIR="$REPO_ROOT/web/console/public/service-icons"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading dashboard-icons archive..."
curl -sL "https://github.com/walkxcode/dashboard-icons/archive/refs/heads/main.tar.gz" \
  -o "$TMP_DIR/dashboard-icons.tar.gz"

echo "Extracting SVGs..."
tar xzf "$TMP_DIR/dashboard-icons.tar.gz" -C "$TMP_DIR"

SRC_DIR="$TMP_DIR/dashboard-icons-main/svg"
if [ ! -d "$SRC_DIR" ]; then
  echo "ERROR: SVG directory not found at $SRC_DIR"
  exit 1
fi

echo "Copying and normalizing icons..."
count=0
for svg in "$SRC_DIR"/*.svg; do
  [ -f "$svg" ] || continue
  basename=$(basename "$svg")
  # Normalize: lowercase, replace underscores/spaces with hyphens
  normalized=$(echo "$basename" | tr '[:upper:]' '[:lower:]' | tr '_' '-' | tr ' ' '-')
  cp "$svg" "$ICONS_DIR/$normalized"
  count=$((count + 1))
done

echo "Generating index.json..."
# Build JSON array of icon keys (filename without .svg extension)
(
  echo "["
  first=true
  for svg in "$ICONS_DIR"/*.svg; do
    [ -f "$svg" ] || continue
    key=$(basename "$svg" .svg)
    [ "$key" = "_default" ] && continue
    if [ "$first" = true ]; then
      first=false
    else
      echo ","
    fi
    printf '  "%s"' "$key"
  done
  echo ""
  echo "]"
) > "$ICONS_DIR/index.json"

total=$(grep -c '"' "$ICONS_DIR/index.json" || true)
echo "Done. Copied $count SVGs, $total icons indexed."
