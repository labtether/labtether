#!/usr/bin/env bash
set -Eeuo pipefail
set +x
umask 077

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE_COMPOSE="${PROJECT_ROOT}/deploy/compose/docker-compose.deploy.yml"
POSTGRES_IMAGE="postgres:18-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15"
LABTETHER_IMAGE="${1:-}"

if [[ -z "${LABTETHER_IMAGE}" ]]; then
  printf 'usage: %s <locally-available-labtether-image>\n' "$0" >&2
  exit 2
fi
if [[ "$LABTETHER_IMAGE" == -* || "$LABTETHER_IMAGE" =~ [[:space:]] ]]; then
  printf 'FAIL: invalid image reference\n' >&2
  exit 2
fi
for command_name in docker grep mktemp python3 sed; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf 'FAIL: required command not found: %s\n' "$command_name" >&2
    exit 1
  }
done

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/labtether-image-mount-contract.XXXXXX")"
project_name="labtether-mount-contract-$PPID-$$"
preflight_container="${project_name}-preflight"
compose=(docker compose --project-name "$project_name" --env-file /dev/null -f "$SOURCE_COMPOSE")
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

# `docker compose create` intentionally materializes the remaining dependencies
# without starting them. This exercises Docker's real image/Compose mount merge
# while keeping every dependency container in the non-running created state.
"${compose[@]}" create web-console console-ingress
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

printf 'PASS: deploy image runtime mount contract\n'
