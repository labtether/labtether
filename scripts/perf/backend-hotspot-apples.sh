#!/usr/bin/env bash
# Run repeatable high-concurrency backend hotspot profiling with an apples-to-apples
# status load shape and capture pprof + query-stat artifacts.
#
# Usage:
#   ./scripts/perf/backend-hotspot-apples.sh --label post9
#
# Auth options:
#   1) bearer token via LABTETHER_API_TOKEN/LABTETHER_OWNER_TOKEN (or --token)
#   2) session login via --username/--password (or LABTETHER_ADMIN_USERNAME/PASSWORD)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

for cmd in curl jq awk sort sed mktemp xargs date; do
  if ! require_command "${cmd}"; then
    exit 1
  fi
done

usage() {
  cat <<'USAGE'
Run repeatable backend hotspot profiling against status aggregate routes.

Options:
  --label NAME               Label suffix for run directory (default: backend-highconcurrency-apples)
  --api-base URL             Backend API base URL (default: http://localhost:8080)
  --token TOKEN              Bearer token (default: LABTETHER_API_TOKEN or LABTETHER_OWNER_TOKEN)
  --username USER            Login username when bearer token is not provided (default: admin)
  --password PASS            Login password when bearer token is not provided (default: password)
  --aggregate-calls N        Number of /status/aggregate calls (default: 2400)
  --live-calls N             Number of /status/aggregate/live calls (default: 4800)
  --concurrency N            Concurrent request workers per batch (default: 24)
  --cpu-seconds N            CPU profile duration seconds (default: 30)
  --output-root DIR          Output root (default: tmp/perf/runs)
  --compare-to PATH          Baseline summary.json or run directory to compare against
  --no-auto-compare          Disable automatic compare report generation
  --skip-pprof               Skip pprof captures
  -h, --help                 Show help

Environment fallbacks:
  LABTETHER_API_TOKEN / LABTETHER_OWNER_TOKEN
  LABTETHER_ADMIN_USERNAME / LABTETHER_ADMIN_PASSWORD
USAGE
}

LABEL="backend-highconcurrency-apples"
API_BASE="${LABTETHER_API_BASE_URL:-http://localhost:8080}"
AUTH_TOKEN="${LABTETHER_API_TOKEN:-${LABTETHER_OWNER_TOKEN:-}}"
LOGIN_USERNAME="${LABTETHER_ADMIN_USERNAME:-admin}"
LOGIN_PASSWORD="${LABTETHER_ADMIN_PASSWORD:-password}"
AGGREGATE_CALLS=2400
LIVE_CALLS=4800
CONCURRENCY=24
CPU_SECONDS=30
OUTPUT_ROOT="tmp/perf/runs"
SKIP_PPROF=0
COMPARE_TO=""
AUTO_COMPARE=1

while (($# > 0)); do
  case "$1" in
    --label)
      LABEL="${2:-}"
      shift 2
      ;;
    --api-base)
      API_BASE="${2:-}"
      shift 2
      ;;
    --token)
      AUTH_TOKEN="${2:-}"
      shift 2
      ;;
    --username)
      LOGIN_USERNAME="${2:-}"
      shift 2
      ;;
    --password)
      LOGIN_PASSWORD="${2:-}"
      shift 2
      ;;
    --aggregate-calls)
      AGGREGATE_CALLS="${2:-}"
      shift 2
      ;;
    --live-calls)
      LIVE_CALLS="${2:-}"
      shift 2
      ;;
    --concurrency)
      CONCURRENCY="${2:-}"
      shift 2
      ;;
    --cpu-seconds)
      CPU_SECONDS="${2:-}"
      shift 2
      ;;
    --output-root)
      OUTPUT_ROOT="${2:-}"
      shift 2
      ;;
    --compare-to)
      COMPARE_TO="${2:-}"
      shift 2
      ;;
    --no-auto-compare)
      AUTO_COMPARE=0
      shift 1
      ;;
    --skip-pprof)
      SKIP_PPROF=1
      shift 1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log_fail "unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if ! [[ "${AGGREGATE_CALLS}" =~ ^[0-9]+$ ]] || ! [[ "${LIVE_CALLS}" =~ ^[0-9]+$ ]] || ! [[ "${CONCURRENCY}" =~ ^[0-9]+$ ]] || ! [[ "${CPU_SECONDS}" =~ ^[0-9]+$ ]]; then
  log_fail "numeric options must be integers"
  exit 1
fi

if [[ "${CONCURRENCY}" -le 0 ]]; then
  log_fail "--concurrency must be > 0"
  exit 1
fi

timestamp="$(date +%Y%m%d-%H%M%S)"
outdir="${OUTPUT_ROOT}/${timestamp}-${LABEL}"
mkdir -p "${outdir}"

COOKIE_JAR=""
cleanup() {
  if [[ -n "${COOKIE_JAR}" && -f "${COOKIE_JAR}" ]]; then
    rm -f "${COOKIE_JAR}"
  fi
}
trap cleanup EXIT

log_info "=== Backend Hotspot Apples Harness ==="
log_info "Run dir: ${outdir}"
log_info "API base: ${API_BASE}"
log_info "Load: /status/aggregate=${AGGREGATE_CALLS}, /status/aggregate/live=${LIVE_CALLS}, concurrency=${CONCURRENCY}"

if [[ -n "${AUTH_TOKEN}" ]]; then
  log_info "Auth mode: bearer token"
else
  log_info "Auth mode: session login (${LOGIN_USERNAME})"
  COOKIE_JAR="$(mktemp)"
  login_payload="$(jq -n --arg username "${LOGIN_USERNAME}" --arg password "${LOGIN_PASSWORD}" '{username: $username, password: $password}')"
  login_code="$(curl -k -L --post301 --post302 --post303 -sS \
    -c "${COOKIE_JAR}" \
    -o "${outdir}/login-response.json" \
    -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -X POST \
    --data "${login_payload}" \
    "${API_BASE}/auth/login" || true)"
  if [[ "${login_code}" != "200" ]]; then
    log_fail "session login failed (${API_BASE}/auth/login -> ${login_code})"
    exit 1
  fi
fi

CURL_COMMON=(-sS -L -k)
if [[ -n "${AUTH_TOKEN}" ]]; then
  CURL_COMMON+=(-H "Authorization: Bearer ${AUTH_TOKEN}")
else
  CURL_COMMON+=(-b "${COOKIE_JAR}")
fi

health_code="$(curl "${CURL_COMMON[@]}" -o /dev/null -w '%{http_code}' "${API_BASE}/healthz" || true)"
if [[ "${health_code}" != "200" ]]; then
  log_fail "health check failed (${API_BASE}/healthz -> ${health_code})"
  exit 1
fi

pprof_code="$(curl "${CURL_COMMON[@]}" -o /dev/null -w '%{http_code}' "${API_BASE}/debug/pprof/" || true)"
if [[ "${SKIP_PPROF}" -eq 0 && "${pprof_code}" != "200" ]]; then
  log_fail "pprof endpoint unavailable (${API_BASE}/debug/pprof/ -> ${pprof_code})"
  log_fail "run backend in DEV_MODE with auth token and retry, or pass --skip-pprof"
  exit 1
fi

capture_worker_stats() {
  local out_file=$1
  curl "${CURL_COMMON[@]}" "${API_BASE}/worker/stats?query_limit=all" > "${out_file}"
}

run_endpoint_batch() {
  local endpoint=$1
  local calls=$2
  local slug=$3
  local raw_file="${outdir}/${slug}.tsv"
  local summary_file="${outdir}/${slug}.json"

  local request_dir
  request_dir="$(mktemp -d)"
  export LT_BATCH_REQUEST_DIR="${request_dir}"
  export LT_BATCH_ENDPOINT="${endpoint}"
  export LT_BATCH_API_BASE="${API_BASE}"
  export LT_BATCH_AUTH_TOKEN="${AUTH_TOKEN}"
  export LT_BATCH_COOKIE_JAR="${COOKIE_JAR}"

  local started_at
  started_at="$(date +%s)"

  seq 1 "${calls}" | xargs -P "${CONCURRENCY}" -I{} bash -c '
    idx="$1"
    outfile="${LT_BATCH_REQUEST_DIR}/${idx}.tsv"
    auth_args=()
    if [[ -n "${LT_BATCH_AUTH_TOKEN}" ]]; then
      auth_args=(-H "Authorization: Bearer ${LT_BATCH_AUTH_TOKEN}")
    elif [[ -n "${LT_BATCH_COOKIE_JAR}" ]]; then
      auth_args=(-b "${LT_BATCH_COOKIE_JAR}")
    fi
    result="$(curl -k -L -sS -o /dev/null -w "%{http_code}\t%{time_total}" "${auth_args[@]}" "${LT_BATCH_API_BASE}${LT_BATCH_ENDPOINT}" 2>/dev/null || true)"
    if [[ -z "${result}" ]]; then
      result="000\t0"
    fi
    printf "%s\n" "${result}" > "${outfile}"
  ' _ {}

  cat "${request_dir}"/*.tsv > "${raw_file}"

  local finished_at
  finished_at="$(date +%s)"
  local wall_seconds=$((finished_at - started_at))

  local ok_count
  ok_count="$(awk -F '\t' '$1 == "200" { c++ } END { print c + 0 }' "${raw_file}")"
  local total_count
  total_count="$(wc -l < "${raw_file}" | tr -d '[:space:]')"
  local fail_count=$((total_count - ok_count))
  if [[ "${ok_count}" -eq 0 ]]; then
    log_fail "batch ${endpoint} returned no successful responses (auth/config issue likely)"
    return 1
  fi

  local lat_file
  lat_file="$(mktemp)"
  awk -F '\t' '$1 == "200" { printf "%.6f\n", $2 * 1000.0 }' "${raw_file}" | sort -n > "${lat_file}"

  local p50=0
  local p95=0
  local p99=0
  local mean=0
  local max=0
  local min=0
  local count
  count="$(wc -l < "${lat_file}" | tr -d '[:space:]')"

  if [[ "${count}" -gt 0 ]]; then
    local p50_idx=$(( (50 * count + 99) / 100 ))
    local p95_idx=$(( (95 * count + 99) / 100 ))
    local p99_idx=$(( (99 * count + 99) / 100 ))
    p50="$(sed -n "${p50_idx}p" "${lat_file}")"
    p95="$(sed -n "${p95_idx}p" "${lat_file}")"
    p99="$(sed -n "${p99_idx}p" "${lat_file}")"
    min="$(head -1 "${lat_file}")"
    max="$(tail -1 "${lat_file}")"
    mean="$(awk '{ sum += $1 } END { if (NR > 0) printf "%.3f", sum / NR; else print 0 }' "${lat_file}")"
  fi

  jq -n \
    --arg endpoint "${endpoint}" \
    --argjson calls "${calls}" \
    --argjson total_count "${total_count}" \
    --argjson ok_count "${ok_count}" \
    --argjson fail_count "${fail_count}" \
    --argjson concurrency "${CONCURRENCY}" \
    --argjson wall_seconds "${wall_seconds}" \
    --argjson p50_ms "${p50}" \
    --argjson p95_ms "${p95}" \
    --argjson p99_ms "${p99}" \
    --argjson mean_ms "${mean}" \
    --argjson min_ms "${min}" \
    --argjson max_ms "${max}" \
    '{
      endpoint: $endpoint,
      requested_calls: $calls,
      total_count: $total_count,
      ok_count: $ok_count,
      fail_count: $fail_count,
      concurrency: $concurrency,
      wall_seconds: $wall_seconds,
      latency_ms: {
        min: $min_ms,
        p50: $p50_ms,
        p95: $p95_ms,
        p99: $p99_ms,
        mean: $mean_ms,
        max: $max_ms
      }
    }' > "${summary_file}"

  rm -f "${lat_file}"
  rm -rf "${request_dir}"

  log_info "Batch ${endpoint}: ok=${ok_count}/${total_count}, p95=${p95}ms"
}

resolve_compare_summary() {
  if [[ -n "${COMPARE_TO}" ]]; then
    if [[ -d "${COMPARE_TO}" && -f "${COMPARE_TO}/summary.json" ]]; then
      printf '%s\n' "${COMPARE_TO}/summary.json"
      return
    fi
    if [[ -f "${COMPARE_TO}" ]]; then
      printf '%s\n' "${COMPARE_TO}"
      return
    fi
    return
  fi

  if [[ "${AUTO_COMPARE}" -eq 0 ]]; then
    return
  fi

  local candidate
  for candidate in "${OUTPUT_ROOT}"/*-"${LABEL}"/summary.json; do
    if [[ "${candidate}" == "${outdir}/summary.json" ]]; then
      continue
    fi
    if [[ -f "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return
    fi
  done

  for candidate in "${OUTPUT_ROOT}"/*/summary.json; do
    if [[ "${candidate}" == "${outdir}/summary.json" ]]; then
      continue
    fi
    if [[ -f "${candidate}" ]]; then
      printf '%s\n' "${candidate}"
      return
    fi
  done
}

build_compare_report() {
  local baseline_summary=$1
  local compare_file="${outdir}/compare-vs-$(basename "$(dirname "${baseline_summary}")").md"
  local candidate_summary="${outdir}/summary.json"

  get_query_metric() {
    local file=$1
    local key_primary=$2
    local key_secondary=$3
    local metric=$4
    jq -r --arg key_primary "${key_primary}" --arg key_secondary "${key_secondary}" --arg metric "${metric}" '
      (.key_deltas[$key_primary][$metric]
       // .key_deltas[$key_secondary][$metric]
       // null)
    ' "${file}"
  }

  get_endpoint_metric() {
    local file=$1
    local endpoint=$2
    local metric=$3
    jq -r --arg endpoint "${endpoint}" --arg metric "${metric}" '
      ((.endpoints // [])[] | select(.endpoint == $endpoint) | .latency_ms[$metric]) // null
    ' "${file}"
  }

  format_delta() {
    local baseline=$1
    local candidate=$2
    if [[ "${baseline}" == "null" || "${candidate}" == "null" || -z "${baseline}" || -z "${candidate}" ]]; then
      printf 'n/a'
      return
    fi
    local diff pct
    diff="$(awk -v b="${baseline}" -v c="${candidate}" 'BEGIN { printf "%.3f", c-b }')"
    pct="$(awk -v b="${baseline}" -v c="${candidate}" 'BEGIN { if (b == 0) { printf "n/a" } else { printf "%.1f%%", ((c-b)/b)*100 } }')"
    printf '%s (%s)' "${diff}" "${pct}"
  }

  local base_batch_total cand_batch_total base_batch_mean cand_batch_mean
  base_batch_total="$(get_query_metric "${baseline_summary}" "batch_snapshot_lateral" "batch_snapshot_new_lateral" "total_ms_delta")"
  cand_batch_total="$(get_query_metric "${candidate_summary}" "batch_snapshot_lateral" "batch_snapshot_new_lateral" "total_ms_delta")"
  base_batch_mean="$(get_query_metric "${baseline_summary}" "batch_snapshot_lateral" "batch_snapshot_new_lateral" "mean_ms_after")"
  cand_batch_mean="$(get_query_metric "${candidate_summary}" "batch_snapshot_lateral" "batch_snapshot_new_lateral" "mean_ms_after")"

  local base_sources_total cand_sources_total base_sources_mean cand_sources_mean
  base_sources_total="$(get_query_metric "${baseline_summary}" "sources_groupby_windowed" "sources_groupby_windowed" "total_ms_delta")"
  cand_sources_total="$(get_query_metric "${candidate_summary}" "sources_groupby_windowed" "sources_groupby_windowed" "total_ms_delta")"
  base_sources_mean="$(get_query_metric "${baseline_summary}" "sources_groupby_windowed" "sources_groupby_windowed" "mean_ms_after")"
  cand_sources_mean="$(get_query_metric "${candidate_summary}" "sources_groupby_windowed" "sources_groupby_windowed" "mean_ms_after")"

  local base_siteproj_total cand_siteproj_total base_siteproj_mean cand_siteproj_mean
  base_siteproj_total="$(get_query_metric "${baseline_summary}" "queryevents_siteid_projected" "queryevents_siteid_projected" "total_ms_delta")"
  cand_siteproj_total="$(get_query_metric "${candidate_summary}" "queryevents_siteid_projected" "queryevents_siteid_projected" "total_ms_delta")"
  base_siteproj_mean="$(get_query_metric "${baseline_summary}" "queryevents_siteid_projected" "queryevents_siteid_projected" "mean_ms_after")"
  cand_siteproj_mean="$(get_query_metric "${candidate_summary}" "queryevents_siteid_projected" "queryevents_siteid_projected" "mean_ms_after")"

  local base_agg_p95 cand_agg_p95 base_live_p95 cand_live_p95
  base_agg_p95="$(get_endpoint_metric "${baseline_summary}" "/status/aggregate" "p95")"
  cand_agg_p95="$(get_endpoint_metric "${candidate_summary}" "/status/aggregate" "p95")"
  base_live_p95="$(get_endpoint_metric "${baseline_summary}" "/status/aggregate/live" "p95")"
  cand_live_p95="$(get_endpoint_metric "${candidate_summary}" "/status/aggregate/live" "p95")"

  cat > "${compare_file}" <<EOF
# Backend Hotspot A/B Compare

- Baseline: ${baseline_summary}
- Candidate: ${candidate_summary}

| Metric | Baseline | Candidate | Delta |
|---|---:|---:|---:|
| /status/aggregate p95 ms | ${base_agg_p95} | ${cand_agg_p95} | $(format_delta "${base_agg_p95}" "${cand_agg_p95}") |
| /status/aggregate/live p95 ms | ${base_live_p95} | ${cand_live_p95} | $(format_delta "${base_live_p95}" "${cand_live_p95}") |
| SnapshotMany lateral total_ms_delta | ${base_batch_total} | ${cand_batch_total} | $(format_delta "${base_batch_total}" "${cand_batch_total}") |
| SnapshotMany lateral mean_ms_after | ${base_batch_mean} | ${cand_batch_mean} | $(format_delta "${base_batch_mean}" "${cand_batch_mean}") |
| Windowed sources total_ms_delta | ${base_sources_total} | ${cand_sources_total} | $(format_delta "${base_sources_total}" "${cand_sources_total}") |
| Windowed sources mean_ms_after | ${base_sources_mean} | ${cand_sources_mean} | $(format_delta "${base_sources_mean}" "${cand_sources_mean}") |
| Projected group query total_ms_delta | ${base_siteproj_total} | ${cand_siteproj_total} | $(format_delta "${base_siteproj_total}" "${cand_siteproj_total}") |
| Projected group query mean_ms_after | ${base_siteproj_mean} | ${cand_siteproj_mean} | $(format_delta "${base_siteproj_mean}" "${cand_siteproj_mean}") |
EOF

  log_info "Compare report: ${compare_file}"
}

capture_worker_stats "${outdir}/worker-stats-before.json"
run_endpoint_batch "/status/aggregate" "${AGGREGATE_CALLS}" "status-aggregate"
run_endpoint_batch "/status/aggregate/live" "${LIVE_CALLS}" "status-aggregate-live"
capture_worker_stats "${outdir}/worker-stats-after.json"

jq -n \
  --slurpfile before "${outdir}/worker-stats-before.json" \
  --slurpfile after "${outdir}/worker-stats-after.json" '
  def indexed($rows):
    reduce ($rows // [])[] as $row ({}; .[$row.query_id] = $row);

  ($before[0] // {}) as $beforeDoc |
  ($after[0] // {}) as $afterDoc |
  (indexed($beforeDoc.performance.top_queries)) as $beforeMap |
  [($afterDoc.performance.top_queries // [])[]
    | . as $afterRow
    | ($beforeMap[$afterRow.query_id] // {}) as $beforeRow
    | {
        query_id: $afterRow.query_id,
        calls_delta: (($afterRow.calls // 0) - ($beforeRow.calls // 0)),
        total_ms_delta: (($afterRow.total_exec_time_ms // 0) - ($beforeRow.total_exec_time_ms // 0)),
        mean_ms_after: ($afterRow.mean_exec_time_ms // 0),
        query: ($afterRow.query // "")
      }
  ]
  | sort_by(.total_ms_delta)
  | reverse
' > "${outdir}/worker-top-queries-delta.json"

if [[ "${SKIP_PPROF}" -eq 0 ]]; then
  log_info "Capturing pprof artifacts..."
  curl "${CURL_COMMON[@]}" "${API_BASE}/debug/pprof/goroutine?debug=1" > "${outdir}/goroutines.txt"
  curl "${CURL_COMMON[@]}" "${API_BASE}/debug/pprof/heap" > "${outdir}/heap.pb.gz"
  curl "${CURL_COMMON[@]}" "${API_BASE}/debug/pprof/profile?seconds=${CPU_SECONDS}" > "${outdir}/cpu.pb.gz"

  if has_command go; then
    go tool pprof -sample_index=alloc_space -top "${outdir}/heap.pb.gz" > "${outdir}/heap-alloc-top.txt" || true
    go tool pprof -sample_index=inuse_space -top "${outdir}/heap.pb.gz" > "${outdir}/heap-inuse-top.txt" || true
    go tool pprof -top "${outdir}/cpu.pb.gz" > "${outdir}/cpu-top.txt" || true
  fi
fi

jq -n \
  --arg outdir "${outdir}" \
  --arg label "${LABEL}" \
  --arg api_base "${API_BASE}" \
  --arg captured_at "$(date -Iseconds)" \
  --argjson aggregate_calls "${AGGREGATE_CALLS}" \
  --argjson live_calls "${LIVE_CALLS}" \
  --argjson concurrency "${CONCURRENCY}" \
  --argjson cpu_seconds "${CPU_SECONDS}" \
  --argjson skip_pprof "${SKIP_PPROF}" \
  --slurpfile aggregate "${outdir}/status-aggregate.json" \
  --slurpfile live "${outdir}/status-aggregate-live.json" \
  --slurpfile worker_before "${outdir}/worker-stats-before.json" \
  --slurpfile worker_after "${outdir}/worker-stats-after.json" \
  --slurpfile top_delta "${outdir}/worker-top-queries-delta.json" '
  ($aggregate[0] // {}) as $aggregateDoc |
  ($live[0] // {}) as $liveDoc |
  ($worker_before[0] // {}) as $beforeDoc |
  ($worker_after[0] // {}) as $afterDoc |
  ($top_delta[0] // []) as $delta |

  def first_match($pattern):
    ([ $delta[] | select((.query // "") | test($pattern; "i")) ] | .[0]);

  {
    outdir: $outdir,
    label: $label,
    captured_at: $captured_at,
    api_base: $api_base,
    load: {
      aggregate_calls: $aggregate_calls,
      live_calls: $live_calls,
      concurrency: $concurrency,
      cpu_seconds: $cpu_seconds,
      pprof_enabled: ($skip_pprof == 0)
    },
    endpoints: [$aggregateDoc, $liveDoc],
    query_stats_enabled: ($afterDoc.performance.pg_stat_statements_enabled // false),
    query_stats_error: ($afterDoc.performance.pg_stat_statements_error // null),
    source_query_aggregates: ($afterDoc.performance.source_queries_top // []),
    top10: ($delta[:10]),
    key_deltas: {
      batch_snapshot_lateral: first_match("WITH asset_ids AS.*LATERAL"),
      snapshot_single_distinct: first_match("SELECT DISTINCT ON \\(metric\\) metric, value FROM metric_samples WHERE asset_id = \\$1"),
      queryevents_groupid_projected: first_match("projected_group_id"),
      queryevents_full_fields: first_match("SELECT id, asset_id, source, level, message, fields, timestamp FROM log_events WHERE timestamp >= \\$1 AND timestamp <= \\$2 ORDER BY"),
      sources_groupby_windowed: first_match("SELECT source, COUNT\\(\\*\\).*timestamp >="),
      sources_groupby_alltime: first_match("SELECT source, COUNT\\(\\*\\).*GROUP BY source ORDER BY")
    }
  }
' > "${outdir}/summary.json"

compare_summary="$(resolve_compare_summary || true)"
if [[ -n "${compare_summary}" && -f "${compare_summary}" ]]; then
  build_compare_report "${compare_summary}"
fi

log_pass "apples run complete"
log_info "Summary: ${outdir}/summary.json"
