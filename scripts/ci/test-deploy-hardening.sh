#!/usr/bin/env bash
set -Eeuo pipefail
set +x
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_COMPOSE="${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml"
RELEASE_TEMPLATE="${PROJECT_ROOT}/deploy/release/docker-compose.deploy.yml.tmpl"
POSTGRES_IMAGE="postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
LABTETHER_IMAGE="ghcr.io/labtether/labtether:v0.0.0@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
GUACD_IMAGE="guacamole/guacd:1.6.0@sha256:8974eaa9ba32f713daf311e7cc8cd7e4cdfba1edea39eed75524e78ef4b08f4f"
REDIS_IMAGE="redis:7-alpine@sha256:6ab0b6e7381779332f97b8ca76193e45b0756f38d4c0dcda72dbb3c32061ab99"
AUTHENTIK_IMAGE="ghcr.io/goauthentik/server:2024.12@sha256:717323d68507fb76dd79f8958f42ce57f8ae0c10a55a7807efa1cfec5752b77c"
DEX_IMAGE="ghcr.io/dexidp/dex:v2.41.1@sha256:bc7cfce7c17f52864e2bb2a4dc1d2f86a41e3019f6d42e81d92a301fad0c8a1d"

for command_name in cmp diff docker env grep mktemp node python3 sed; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'FAIL: required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/labtether-deploy-hardening.XXXXXX")"
cleanup() {
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT INT TERM HUP

node --check "${PROJECT_ROOT}/web/console/scripts/console-ingress.mjs"
node --input-type=module - "${PROJECT_ROOT}/web/console/scripts/console-ingress.mjs" <<'JS'
import { pathToFileURL } from "node:url";

const { parseMaxConnections } = await import(pathToFileURL(process.argv[2]).href);
if (parseMaxConnections("1") !== 1 || parseMaxConnections("4096") !== 4096) process.exit(1);
for (const invalid of ["", "0", "4097", "1e3", "-1", "512;touch /tmp/nope"]) {
  if (parseMaxConnections(invalid) !== 512) process.exit(1);
}
JS
grep -F '/app/scripts/console-ingress.mjs /app/console/scripts/' "${PROJECT_ROOT}/build/Dockerfile" >/dev/null
# Image-level VOLUME declarations allocate anonymous volumes before Compose can
# mask them. Persistence must be explicit at each hub installation boundary.
if grep -Eiq '^[[:space:]]*VOLUME([[:space:]]|$)' "${PROJECT_ROOT}/build/Dockerfile"; then
  printf 'FAIL: shared deploy image must not declare implicit Docker volumes\n' >&2
  exit 1
fi
grep -Fx 'data-mount-check' "${PROJECT_ROOT}/build/s6/user/contents.d/data-mount-check" >/dev/null
grep -Fx 'data-mount-check' "${PROJECT_ROOT}/build/s6/s6-rc.d/postgres/dependencies.d/data-mount-check" >/dev/null

# Development, demo, and identity-provider fixtures are still executable
# software supply-chain inputs. External images must be immutable even when the
# stack is disposable. The sole exception is the explicitly local CI agent,
# which must retain pull_policy: never.
if grep -Eq '^[[:space:]]*LABTETHER_IMAGE[[:space:]]*=' "${PROJECT_ROOT}/.env.demo"; then
  printf 'FAIL: .env.demo must require an explicit immutable LabTether image\n' >&2
  exit 1
fi
grep -Fx "POSTGRES_IMAGE=${POSTGRES_IMAGE}" "${PROJECT_ROOT}/.env.demo" >/dev/null
if env -u LABTETHER_IMAGE "${PROJECT_ROOT}/scripts/demo-up.sh" --check >/dev/null 2>&1; then
  printf 'FAIL: demo launcher accepted a missing LabTether image\n' >&2
  exit 1
fi
if LABTETHER_IMAGE='ghcr.io/labtether/labtether:latest' \
  "${PROJECT_ROOT}/scripts/demo-up.sh" --check >/dev/null 2>&1; then
  printf 'FAIL: demo launcher accepted a mutable LabTether image\n' >&2
  exit 1
fi
LABTETHER_IMAGE="${LABTETHER_IMAGE}" \
  "${PROJECT_ROOT}/scripts/demo-up.sh" --check >/dev/null

export POSTGRES_IMAGE LABTETHER_IMAGE GUACD_IMAGE REDIS_IMAGE AUTHENTIK_IMAGE DEX_IMAGE
env -u GUACD_IMAGE docker compose \
  --env-file /dev/null \
  --profile ci-agent \
  --profile remote-desktop \
  -f "${PROJECT_ROOT}/docker-compose.yml" \
  config --format json >"${tmp_dir}/development-config.json"
env -u GUACD_IMAGE docker compose \
  --env-file "${PROJECT_ROOT}/.env.demo" \
  -f "$SOURCE_COMPOSE" \
  -f "${PROJECT_ROOT}/docker-compose.demo.yml" \
  config --format json >"${tmp_dir}/demo-config.json"
env -u GUACD_IMAGE docker compose \
  --env-file "${PROJECT_ROOT}/.env.demo" \
  -f "$SOURCE_COMPOSE" \
  -f "${PROJECT_ROOT}/deploy/compose/docker-compose.demo.yml" \
  config --format json >"${tmp_dir}/demo-deploy-config.json"
docker compose \
  -f "${PROJECT_ROOT}/deploy/testing/docker-compose.authentik.yml" \
  config --format json >"${tmp_dir}/authentik-config.json"
docker compose \
  -f "${PROJECT_ROOT}/deploy/testing/docker-compose.dex.yml" \
  config --format json >"${tmp_dir}/dex-config.json"

python3 - \
  "${tmp_dir}/development-config.json" \
  "${tmp_dir}/demo-config.json" \
  "${tmp_dir}/demo-deploy-config.json" \
  "${tmp_dir}/authentik-config.json" \
  "${tmp_dir}/dex-config.json" <<'PY'
import json
import os
import re
import sys

paths = dict(
    zip(
        ("development", "demo", "demo-deploy", "authentik", "dex"),
        sys.argv[1:],
        strict=True,
    )
)
configs = {}
for name, path in paths.items():
    with open(path, encoding="utf-8") as handle:
        configs[name] = json.load(handle)["services"]

digest = re.compile(r"@sha256:[0-9a-f]{64}$")
for config_name, services in configs.items():
    for service_name, service in services.items():
        image = service.get("image")
        if not image:
            continue
        if image == "labtether-agent:ci":
            if config_name != "development" or service.get("pull_policy") != "never":
                raise SystemExit("local CI agent image must remain development-only and pull_policy: never")
            continue
        if not digest.search(image):
            raise SystemExit(f"{config_name}/{service_name}: external image is not digest pinned: {image}")

expected = {
    ("development", "postgres"): os.environ["POSTGRES_IMAGE"],
    ("development", "guacd"): os.environ["GUACD_IMAGE"],
    ("demo", "labtether"): os.environ["LABTETHER_IMAGE"],
    ("demo", "web-console"): os.environ["LABTETHER_IMAGE"],
    ("demo", "console-ingress"): os.environ["LABTETHER_IMAGE"],
    ("demo", "postgres"): os.environ["POSTGRES_IMAGE"],
    ("demo", "labtether-bootstrap"): os.environ["LABTETHER_IMAGE"],
    ("demo", "demo-seed"): os.environ["POSTGRES_IMAGE"],
    ("authentik", "authentik-postgres"): os.environ["POSTGRES_IMAGE"],
    ("authentik", "authentik-redis"): os.environ["REDIS_IMAGE"],
    ("authentik", "authentik-server"): os.environ["AUTHENTIK_IMAGE"],
    ("authentik", "authentik-worker"): os.environ["AUTHENTIK_IMAGE"],
    ("dex", "dex"): os.environ["DEX_IMAGE"],
}
for service_name in ("labtether", "web-console", "console-ingress"):
    expected[("demo-deploy", service_name)] = os.environ["LABTETHER_IMAGE"]
for service_name in ("postgres", "demo-seed"):
    expected[("demo-deploy", service_name)] = os.environ["POSTGRES_IMAGE"]
expected[("demo-deploy", "labtether-bootstrap")] = os.environ["LABTETHER_IMAGE"]
for (config_name, service_name), expected_image in expected.items():
    actual = configs[config_name][service_name].get("image")
    if actual != expected_image:
        raise SystemExit(
            f"{config_name}/{service_name}: expected {expected_image}, got {actual}"
        )
PY

# The developer Compose file and release template are one security contract.
# Normalize only the three intentional image placeholders before comparing.
# shellcheck disable=SC2016 # These are literal Compose substitutions, not shell expansions.
sed \
  -e 's|${POSTGRES_IMAGE:?set POSTGRES_IMAGE in .env.deploy}|__POSTGRES_IMAGE__|g' \
  -e 's|${LABTETHER_IMAGE:?set LABTETHER_IMAGE in .env.deploy}|__LABTETHER_IMAGE__|g' \
  -e 's|${GUACD_IMAGE:-guacamole/guacd:1.6.0@sha256:8974eaa9ba32f713daf311e7cc8cd7e4cdfba1edea39eed75524e78ef4b08f4f}|__GUACD_IMAGE__|g' \
  "$SOURCE_COMPOSE" >"${tmp_dir}/normalized-source.yml"
if ! cmp -s "${tmp_dir}/normalized-source.yml" "$RELEASE_TEMPLATE"; then
  diff -u "${tmp_dir}/normalized-source.yml" "$RELEASE_TEMPLATE" >&2 || true
  printf 'FAIL: deploy source and release template drifted\n' >&2
  exit 1
fi

docker compose --profile remote-desktop -f "$SOURCE_COMPOSE" config --quiet
docker compose --profile remote-desktop -f "$SOURCE_COMPOSE" config --format json >"${tmp_dir}/source-config.json"
python3 - "${tmp_dir}/source-config.json" <<'PY'
import json
import re
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    config = json.load(handle)

services = config["services"]
networks = config["networks"]
required = {"labtether-bootstrap", "postgres", "labtether", "web-console", "console-ingress", "guacd"}
if set(services) != required:
    raise SystemExit(f"unexpected service set: {sorted(services)}")

for name in required:
    service = services[name]
    if service.get("read_only") is not True:
        raise SystemExit(f"{name}: root filesystem is not read-only")
    if "no-new-privileges:true" not in service.get("security_opt", []):
        raise SystemExit(f"{name}: no-new-privileges is missing")
    if "ALL" not in service.get("cap_drop", []):
        raise SystemExit(f"{name}: capabilities are not dropped")
    limits = service.get("deploy", {}).get("resources", {}).get("limits", {})
    if not isinstance(limits.get("pids"), int) or limits["pids"] <= 0:
        raise SystemExit(f"{name}: PID limit is missing")

if services["labtether-bootstrap"].get("network_mode") != "none":
    raise SystemExit("bootstrap: network isolation is missing")
if services["labtether-bootstrap"].get("user") != "0:0":
    raise SystemExit("bootstrap: explicit root UID/GID is required for secret ownership setup")
if services["labtether-bootstrap"].get("image") != services["labtether"].get("image"):
    raise SystemExit("bootstrap: must use the same volume-free image as labtether")
if set(services["labtether-bootstrap"].get("cap_add", [])) != {"CHOWN", "DAC_OVERRIDE", "FOWNER"}:
    raise SystemExit("bootstrap: capability allowlist changed")
if networks.get("console-plane", {}).get("internal") is not True:
    raise SystemExit("console-plane: secret-bearing network must remain internal")
if networks.get("ingress-plane", {}).get("internal") is True:
    raise SystemExit("ingress-plane: published-port network must remain host-reachable")
if set(services["postgres"].get("networks", {})) != {"data-plane"}:
    raise SystemExit("postgres: unexpected network access")
if set(services["web-console"].get("networks", {})) != {"console-plane"}:
    raise SystemExit("web-console: unexpected network access")
if services["web-console"].get("ports"):
    raise SystemExit("web-console: secret-bearing service must not publish ports directly")
for name in ("web-console", "console-ingress"):
    if any(entry.split(":", 1)[0] == "/data" for entry in services[name].get("tmpfs", [])):
        raise SystemExit(f"{name}: split service must not receive a writable /data tmpfs")
    if any(mount.get("target") == "/data" for mount in services[name].get("volumes", [])):
        raise SystemExit(f"{name}: split service must not receive persistent /data")
if set(services["console-ingress"].get("networks", {})) != {"console-plane", "ingress-plane"}:
    raise SystemExit("console-ingress: expected only console and published-ingress networks")
if services["console-ingress"].get("volumes"):
    raise SystemExit("console-ingress: no-secret sidecar must not mount volumes")
if services["console-ingress"].get("environment"):
    raise SystemExit("console-ingress: Compose environment must remain empty")
ingress_ports = services["console-ingress"].get("ports", [])
if len(ingress_ports) != 1 or ingress_ports[0].get("host_ip") != "127.0.0.1" or ingress_ports[0].get("published") != "3000" or ingress_ports[0].get("target") != 3000:
    raise SystemExit(f"console-ingress: loopback publication contract changed: {ingress_ports!r}")
if services["console-ingress"].get("command") != [
    "/usr/bin/env",
    "-i",
    "HOME=/tmp",
    "LABTETHER_CONSOLE_INGRESS_MAX_CONNECTIONS=512",
    "/usr/bin/node",
    "/app/console/scripts/console-ingress.mjs",
]:
    raise SystemExit("console-ingress: clean-environment fixed-target command changed")
if set(services["guacd"].get("networks", {})) != {"desktop-plane", "egress"}:
    raise SystemExit("guacd: target egress or desktop isolation is missing")
if set(services["labtether"].get("networks", {})) != {"data-plane", "console-plane", "desktop-plane", "egress"}:
    raise SystemExit("labtether: segmented network contract changed")
if services["guacd"].get("user") != "1000:1000":
    raise SystemExit("guacd: expected explicit unprivileged UID/GID")

postgres_health = services["postgres"].get("healthcheck", {}).get("test", [])
if postgres_health != ["CMD-SHELL", 'pg_isready -U "$${POSTGRES_USER}" -d "$${POSTGRES_DB}"']:
    raise SystemExit(f"postgres: unsafe or unexpected healthcheck: {postgres_health!r}")

hub_environment = services["labtether"].get("environment", {})
if "SSL_CERT_FILE" in hub_environment:
    raise SystemExit("labtether: SSL_CERT_FILE would replace public system roots")
if hub_environment.get("SSL_CERT_DIR") != "/etc/ssl/certs:/ca":
    raise SystemExit("labtether: combined system/private CA directory contract changed")

digest = re.compile(r"@sha256:[0-9a-f]{64}$")
for name in required - {"labtether-bootstrap"}:
    if not digest.search(services[name]["image"]):
        raise SystemExit(f"{name}: image is not digest pinned")
if not digest.search(services["labtether-bootstrap"]["image"]):
    raise SystemExit("bootstrap: image is not digest pinned")

hub_mounts = {mount["target"]: mount for mount in services["labtether"]["volumes"]}
if not hub_mounts.get("/bootstrap", {}).get("read_only"):
    raise SystemExit("labtether: PostgreSQL bootstrap volume must be read-only")
if hub_mounts.get("/bootstrap/auth", {}).get("read_only"):
    raise SystemExit("labtether: setup-token subvolume must remain consumable")
hub_data = hub_mounts.get("/data", {})
if hub_data.get("type") != "volume" or hub_data.get("source") != "labtether-data" or hub_data.get("read_only"):
    raise SystemExit("labtether: persistent /data must be an explicit writable named volume")
PY

if "${PROJECT_ROOT}/scripts/release/render-deploy-artifacts.sh" \
  --version v0.0.0 \
  --repository labtether/labtether \
  --output-dir "${tmp_dir}/missing-digest" >/dev/null 2>&1; then
  printf 'FAIL: release renderer accepted an unpinned LabTether image\n' >&2
  exit 1
fi

"${PROJECT_ROOT}/scripts/release/render-deploy-artifacts.sh" \
  --version v0.0.0 \
  --repository labtether/labtether \
  --image-digest sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --output-dir "${tmp_dir}/rendered"
(
  cd "${tmp_dir}/rendered"
  docker compose --profile remote-desktop -f docker-compose.deploy.yml config --quiet
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum --check SHA256SUMS
  else
    shasum -a 256 --check SHA256SUMS
  fi
)
grep -F '"guacd": "guacamole/guacd:1.6.0@sha256:' "${tmp_dir}/rendered/deploy-manifest.json" >/dev/null

printf 'PASS: deploy hardening contract\n'
