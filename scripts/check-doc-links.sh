#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

check_file() {
  case "$1" in
    README.md|docs/README.md|docs/USER_GUIDE.md|docs/wiki/*) return 0 ;;
    *) return 1 ;;
  esac
}

failures=0

while IFS= read -r file; do
  if ! check_file "$file"; then
    continue
  fi

  dir="${file%/*}"
  if [[ "$dir" == "$file" ]]; then
    dir="."
  fi

  while IFS= read -r target; do
    target="${target#<}"
    target="${target%>}"

    case "$target" in
      ""|http://*|https://*|mailto:*|tel:*|data:*|javascript:*|\#*)
        continue
        ;;
    esac

    # strip anchor/query for file existence checks
    target="${target%%#*}"
    target="${target%%\?*}"

    if [[ -z "$target" ]]; then
      continue
    fi

    if [[ "$target" == /* ]]; then
      resolved=".${target}"
    else
      resolved="${dir}/${target}"
    fi

    if [[ ! -e "$resolved" ]]; then
      echo "Broken link: ${file} -> ${target} (resolved: ${resolved})"
      failures=1
    fi
  done < <(perl -ne 'while(/\[[^\]]+\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g){print "$1\n"}' "$file")
done < <(git ls-files '*.md')

if [[ "$failures" -ne 0 ]]; then
  echo "Docs link check failed."
  exit 1
fi

echo "Docs local links OK."
