#!/usr/bin/env bash
set -Eeuo pipefail
default_services="$(env -u COMPOSE_PROFILES docker compose config --services)"
if grep -Fxq qa-agent <<<"${default_services}"; then
  echo "qa-agent must not be part of the default Compose service set" >&2
  exit 1
fi
COMPOSE_PROFILES=ci-agent docker compose config --quiet
profile_services="$(COMPOSE_PROFILES=ci-agent docker compose config --services)"
grep -Fxq qa-agent <<<"${profile_services}"
COMPOSE_PROFILES=ci-agent docker compose config --format json |
  jq -e '
    .services["qa-agent"].environment.LABTETHER_API_TOKEN == "" and
    .services["qa-agent"].environment.LABTETHER_ENROLLMENT_TOKEN_FILE == "/run/labtether-enrollment/enrollment-token" and
    .services.labtether.environment.LABTETHER_ALLOW_LEGACY_SHARED_AGENT_AUTH == "false"
  ' >/dev/null
