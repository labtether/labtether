#!/usr/bin/env bash
set -euo pipefail
ARTIFACT_ROOT="/tmp/labtether-perf-artifacts"
RUN_ROOT="${ARTIFACT_ROOT}/runs"
PG_CONTAINER="labtether-perf-pg"
BACKEND_PID=""
mkdir -p "${ARTIFACT_ROOT}" "${RUN_ROOT}"

cleanup() {
  if [[ -n "${BACKEND_PID}" ]] && kill -0 "${BACKEND_PID}" >/dev/null 2>&1; then
    kill "${BACKEND_PID}" || true
    wait "${BACKEND_PID}" || true
  fi
  docker rm -f "${PG_CONTAINER}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker rm -f "${PG_CONTAINER}" >/dev/null 2>&1 || true
docker run -d --name "${PG_CONTAINER}" \
  -e POSTGRES_USER=labtether \
  -e POSTGRES_PASSWORD=labtether \
  -e POSTGRES_DB=labtether \
  -p 5432:5432 \
  postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873 \
  -c shared_preload_libraries=pg_stat_statements \
  -c pg_stat_statements.track=all >/dev/null

for ((attempt = 0; attempt < 60; attempt++)); do
  if docker exec "${PG_CONTAINER}" psql -U labtether -d labtether -tAc "SELECT 1" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "${PG_CONTAINER}" psql -U labtether -d labtether -tAc "SELECT 1" >/dev/null
docker exec "${PG_CONTAINER}" psql -U labtether -d labtether -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;" >/dev/null

go build -o build/labtether ./cmd/labtether
./build/labtether > "${ARTIFACT_ROOT}/backend.log" 2>&1 &
BACKEND_PID=$!
echo "${BACKEND_PID}" > "${ARTIFACT_ROOT}/backend.pid"

for ((attempt = 0; attempt < 60; attempt++)); do
  if curl -fsS --max-time 2 "http://localhost:8080/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl -fsS --max-time 2 "http://localhost:8080/healthz" >/dev/null

# Seed at least one asset so aggregate telemetry paths execute and
# pg_stat_statements captures the lateral SnapshotMany query shape.
curl -fsS -X POST "http://localhost:8080/assets/heartbeat" \
  -H "Authorization: Bearer ${LABTETHER_API_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"asset_id":"perf-ci-asset-01","type":"host","name":"Perf CI Asset 01","source":"perf-ci","status":"online","platform":"linux","metadata":{"cpu_percent":"12.5","memory_percent":"44.1"}}' >/dev/null

./scripts/perf/backend-hotspot-apples.sh \
  --label ci-hotspot \
  --api-base "http://localhost:8080" \
  --aggregate-calls 240 \
  --live-calls 480 \
  --concurrency 16 \
  --cpu-seconds 5 \
  --skip-pprof \
  --output-root "${RUN_ROOT}" \
  --no-auto-compare

# backend-hotspot-apples.sh appends mktemp's random suffix to the
# requested label (for example, ci-hotspot.ABC123).
RUN_DIR="$(find "${RUN_ROOT}" -mindepth 1 -maxdepth 1 -type d -name '*-ci-hotspot.*' -print -quit)"
test -n "${RUN_DIR}"
echo "RUN_DIR=${RUN_DIR}" >> "${GITHUB_ENV}"

./scripts/perf/backend-hotspot-thresholds.sh \
  --summary "${RUN_DIR}/summary.json" \
  --write-report "${RUN_DIR}/threshold-report.json"

./scripts/perf/backend-hotspot-explain.sh \
  --scenario projected-group \
  --dsn "${DATABASE_URL}" \
  --output "${RUN_DIR}/explain-projected-group.json"

./scripts/perf/backend-hotspot-explain.sh \
  --scenario sources-windowed \
  --dsn "${DATABASE_URL}" \
  --output "${RUN_DIR}/explain-sources-windowed.json"
