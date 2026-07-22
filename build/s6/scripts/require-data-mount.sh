#!/bin/sh
set -eu

if [ ! -r /proc/self/mountinfo ] || ! awk '
  $5 == "/data" {
    for (field = 6; field < NF; field++) {
      if ($field == "-") {
        if ($(field + 1) != "tmpfs" && $(field + 1) != "ramfs") persistent = 1
        break
      }
    }
  }
  END { exit persistent ? 0 : 1 }
' /proc/self/mountinfo; then
  echo "labtether: /data must be an explicit persistent mount; refusing ephemeral all-in-one startup" >&2
  exit 1
fi
