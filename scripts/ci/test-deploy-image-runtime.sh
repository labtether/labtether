#!/usr/bin/env bash
set -Eeuo pipefail
set +x
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_COMPOSE="${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml"
POSTGRES_IMAGE="postgres:18-alpine@sha256:10a5c69a83035ab5239f5620d708520e8b28f5aa1f2b20143e6024532edadfb5"
LABTETHER_IMAGE="${1:-}"

if [[ -z "${LABTETHER_IMAGE}" ]]; then
  printf 'usage: %s <locally-available-labtether-image>\n' "$0" >&2
  exit 2
fi
if [[ "$LABTETHER_IMAGE" == -* || "$LABTETHER_IMAGE" =~ [[:space:]] ]]; then
  printf 'FAIL: invalid image reference\n' >&2
  exit 2
fi
for command_name in curl docker grep mktemp python3 sed; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'FAIL: required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/labtether-image-mount-contract.XXXXXX")"
project_name="labtether-mount-contract-$PPID-$$"
preflight_container="${project_name}-preflight"
compose=(docker compose --project-name "$project_name" --env-file /dev/null -f "$SOURCE_COMPOSE")
dump_logs() {
  "${compose[@]}" logs --no-color --tail 80 \
    labtether-bootstrap labtether web-console console-ingress >&2 2>&1 || true
}
cleanup() {
  local container_id="" volume_name=""
  docker rm -f "$preflight_container" >/dev/null 2>&1 || true
  : >"${tmp_dir}/attached-volumes"
  for container_id in $("${compose[@]}" ps -aq 2>/dev/null || true); do
    docker inspect --format '{{range .Mounts}}{{if .Name}}{{println .Name}}{{end}}{{end}}' \
      "$container_id" >>"${tmp_dir}/attached-volumes" 2>/dev/null || true
  done
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  while IFS= read -r volume_name; do
    [[ -z "$volume_name" ]] || docker volume rm "$volume_name" >/dev/null 2>&1 || true
  done <"${tmp_dir}/attached-volumes"
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

docker image inspect "$LABTETHER_IMAGE" >"${tmp_dir}/image.json"
python3 - "${tmp_dir}/image.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    image = json.load(handle)[0]
volumes = image.get("Config", {}).get("Volumes") or {}
if volumes:
    raise SystemExit(f"deploy image declares implicit volumes: {sorted(volumes)}")
PY

docker run --detach --name "$preflight_container" "$LABTETHER_IMAGE" >/dev/null
for _ in 1 2 3 4 5 6 7 8 9 10; do
  [[ "$(docker inspect --format '{{.State.Running}}' "$preflight_container")" == false ]] && break
  sleep 1
done
docker logs "$preflight_container" >"${tmp_dir}/preflight.log" 2>&1 || true
if [[ "$(docker inspect --format '{{.State.Running}}' "$preflight_container")" != false ]] || \
   [[ "$(docker inspect --format '{{.State.ExitCode}}' "$preflight_container")" == 0 ]] || \
   ! grep -Fq '/data must be an explicit persistent mount' "${tmp_dir}/preflight.log"; then
  printf 'FAIL: all-in-one image did not fail closed without an explicit /data mount\n' >&2
  sed -n '1,120p' "${tmp_dir}/preflight.log" >&2
  exit 1
fi
docker rm "$preflight_container" >/dev/null

export LABTETHER_IMAGE POSTGRES_IMAGE
# Upgrade from LabTether v1.4.0: that Hub ran as root and left root-owned files
# (secrets.json and state.json with mode 0600) in the volumes the Hub now uses
# as 65532:65532. Seed that shape, plus a symlink to a root-owned secret outside
# those volumes, so the bootstrap must hand the files over without following it.
"${compose[@]}" run --rm --no-deps --no-TTY --entrypoint /bin/sh labtether-bootstrap -ec '
  umask 077
  mkdir /hub-volumes/data/install /hub-volumes/agent-cache/linux-amd64
  printf "{\"schema_version\":1}\n" >/hub-volumes/data/install/state.json
  printf "{}\n" >/hub-volumes/data/install/secrets.json
  ln -s /bootstrap/postgres/password /hub-volumes/data/install/password-link
  printf "stale\n" >/hub-volumes/ca/ca.crt
  chmod 0644 /hub-volumes/ca/ca.crt
  printf "stale\n" >/hub-volumes/service-auth/api-token
' >/dev/null
# Run the no-network bootstrap in its isolated project to prove the LabTether
# image contains the narrowly required secret-generation tools. The completed
# container is then inspected below before the disposable project is removed.
"${compose[@]}" up --no-deps --abort-on-container-exit \
  --exit-code-from labtether-bootstrap labtether-bootstrap >/dev/null
bootstrap_id="$("${compose[@]}" ps -aq labtether-bootstrap)"
if [[ -z "$bootstrap_id" ]]; then
  printf 'FAIL: Compose did not create labtether-bootstrap\n' >&2
  exit 1
fi
docker inspect "$bootstrap_id" >"${tmp_dir}/labtether-bootstrap.json"
expected_image_id="$(docker image inspect --format '{{.Id}}' "$LABTETHER_IMAGE")"
python3 - \
  "$LABTETHER_IMAGE" \
  "$expected_image_id" \
  "$project_name" \
  "${tmp_dir}/labtether-bootstrap.json" <<'PY'
import json
import sys

expected_ref, expected_id, project_name, inspect_path = sys.argv[1:]
with open(inspect_path, encoding="utf-8") as handle:
    container = json.load(handle)[0]

if container.get("Config", {}).get("Image") != expected_ref:
    raise SystemExit(
        f"bootstrap: expected image ref {expected_ref}, got "
        f"{container.get('Config', {}).get('Image')}"
    )
if container.get("Image") != expected_id:
    raise SystemExit(
        f"bootstrap: expected image ID {expected_id}, got {container.get('Image')}"
    )

expected_mounts = {
    "/bootstrap": f"{project_name}_labtether-bootstrap",
    "/bootstrap/auth": f"{project_name}_labtether-auth-bootstrap",
    "/postgres-root": f"{project_name}_postgres-data",
    "/hub-volumes/data": f"{project_name}_labtether-data",
    "/hub-volumes/ca": f"{project_name}_labtether-ca",
    "/hub-volumes/service-auth": f"{project_name}_labtether-service-auth",
    "/hub-volumes/agent-cache": f"{project_name}_labtether-agent-cache",
}
mounts = container.get("Mounts", [])
actual_destinations = {mount.get("Destination") for mount in mounts}
if actual_destinations != set(expected_mounts):
    raise SystemExit(
        f"bootstrap: unexpected runtime mount destinations: {sorted(actual_destinations)}"
    )
for mount in mounts:
    destination = mount.get("Destination")
    expected_name = expected_mounts[destination]
    if mount.get("Type") != "volume" or mount.get("Name") != expected_name:
        rendered = {
            key: mount.get(key)
            for key in ("Type", "Name", "Source", "Destination", "RW")
        }
        raise SystemExit(
            f"bootstrap: anonymous or unexpected runtime mount: {rendered}"
        )
    if mount.get("RW") is not True:
        raise SystemExit(f"bootstrap: required mount is not writable: {destination}")

host_config = container.get("HostConfig", {})
if host_config.get("ReadonlyRootfs") is not True:
    raise SystemExit("bootstrap: root filesystem is not read-only at runtime")
if host_config.get("NetworkMode") != "none":
    raise SystemExit("bootstrap: runtime network isolation is missing")
if not any(
    option in {"no-new-privileges", "no-new-privileges:true"}
    for option in (host_config.get("SecurityOpt") or [])
):
    raise SystemExit("bootstrap: runtime no-new-privileges is missing")
if container.get("Config", {}).get("User") != "0:0":
    raise SystemExit("bootstrap: runtime UID/GID changed")
if host_config.get("CapDrop") != ["ALL"]:
    raise SystemExit(f"bootstrap: runtime capability drop changed: {host_config.get('CapDrop')}")
if set(host_config.get("CapAdd") or []) != {"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER"}:
    raise SystemExit(f"bootstrap: runtime capability allowlist changed: {host_config.get('CapAdd')}")
PY

# The bootstrap must have left no root-owned entry in the Hub volumes and must
# not have followed the symlink to the root-owned database password.
# shellcheck disable=SC2016 # Expanded by the container shell.
if ! "${compose[@]}" run --rm --no-deps --no-TTY --entrypoint /bin/sh labtether-bootstrap -ec '
  left="$(find /hub-volumes/data /hub-volumes/ca /hub-volumes/service-auth /hub-volumes/agent-cache -xdev -user 0)"
  if [ -n "$left" ]; then
    printf "root-owned entries remain:\n%s\n" "$left" >&2
    exit 1
  fi
  test "$(stat -c %u:%g /hub-volumes/data/install/secrets.json)" = 65532:65532
  test "$(stat -c %u:%g /bootstrap/postgres/password)" = 0:0
'; then
  printf 'FAIL: bootstrap did not safely hand root-owned Hub files to the Hub user\n' >&2
  exit 1
fi

# Start the default stack from the deploy file. Compose starts web-console only
# after the Hub health check passes, and console-ingress only after
# web-console's. The console must then answer through the published port with
# a reply it fetched from the Hub over the verified internal TLS route.
if ! "${compose[@]}" up --detach >/dev/null; then
  printf 'FAIL: the deploy stack did not start\n' >&2
  dump_logs
  exit 1
fi
deadline=$((SECONDS + 180))
for service_name in labtether web-console console-ingress; do
  health=""
  until [[ "$health" == healthy ]]; do
    if (( SECONDS >= deadline )); then
      printf 'FAIL: %s is not healthy (%s)\n' "$service_name" "${health:-not running}" >&2
      dump_logs
      exit 1
    fi
    sleep 3
    container_id="$("${compose[@]}" ps -q "$service_name")"
    health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
  done
done
console_url="http://127.0.0.1:3000"
setup_status=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  setup_status="$(curl --fail --silent --max-time 10 "${console_url}/api/auth/bootstrap/status")" && break
  setup_status=""
  sleep 3
done
if ! python3 -c 'import json, sys; sys.exit(not isinstance(json.loads(sys.argv[1]).get("setup_required"), bool))' "$setup_status" 2>/dev/null; then
  printf 'FAIL: the console did not return setup status from the Hub\n' >&2
  dump_logs
  exit 1
fi
login_code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 20 --location --max-redirs 3 "${console_url}/login" || true)"
if [[ "$login_code" != 200 ]]; then
  printf 'FAIL: the console login page returned HTTP %s\n' "${login_code:-nothing}" >&2
  dump_logs
  exit 1
fi

for service_name in web-console console-ingress; do
  container_id="$("${compose[@]}" ps -aq "$service_name")"
  if [[ -z "$container_id" ]]; then
    printf 'FAIL: Compose did not create %s\n' "$service_name" >&2
    exit 1
  fi
  docker inspect "$container_id" >"${tmp_dir}/${service_name}.json"
  python3 - "$service_name" "${tmp_dir}/${service_name}.json" <<'PY'
import json
import sys

service_name = sys.argv[1]
with open(sys.argv[2], encoding="utf-8") as handle:
    container = json.load(handle)[0]
data_mounts = [
    mount for mount in container.get("Mounts", [])
    if mount.get("Destination") == "/data"
]
if data_mounts:
    rendered = [
        {key: mount.get(key) for key in ("Type", "Name", "Source", "Destination", "RW")}
        for mount in data_mounts
    ]
    raise SystemExit(f"{service_name}: unexpected runtime /data mount: {rendered}")
if "/data" in (container.get("HostConfig", {}).get("Tmpfs") or {}):
    raise SystemExit(f"{service_name}: unexpected runtime /data tmpfs")
PY
done

printf 'PASS: deploy image runtime contract, v1.4.0 file ownership and console start\n'
