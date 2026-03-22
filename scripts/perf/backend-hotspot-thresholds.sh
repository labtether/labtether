#!/usr/bin/env bash
# Validate backend hotspot apples summary against pass/fail thresholds.
# Usage:
#   ./scripts/perf/backend-hotspot-thresholds.sh --summary /path/to/summary.json

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

if ! require_command jq; then
  exit 1
fi
if ! require_command awk; then
  exit 1
fi

usage() {
  cat <<'USAGE'
Validate a backend hotspot apples summary against reliability/perf thresholds.

Required:
  --summary PATH                        Path to apples summary.json.

Optional:
  --baseline PATH                       Optional baseline summary.json for regression checks.
  --write-report PATH                   Optional output JSON report path.
  --max-aggregate-p95-ms FLOAT          Max /status/aggregate p95 latency (default: 700).
  --max-live-p95-ms FLOAT               Max /status/aggregate/live p95 latency (default: 450).
  --max-endpoint-fail-rate FLOAT        Max endpoint fail rate 0..1 (default: 0.01).
  --max-snapshotmany-mean-ms FLOAT      Max SnapshotMany lateral mean_ms_after (default: 12).
  --max-windowed-sources-mean-ms FLOAT  Max windowed sources mean_ms_after (default: 60).
  --max-full-fields-calls INT           Max full-fields QueryEvents calls_delta (default: 500).
  --max-aggregate-p95-regression-pct FLOAT  Max aggregate p95 regression vs baseline (default: 40).
  --max-live-p95-regression-pct FLOAT       Max live p95 regression vs baseline (default: 40).

Environment overrides:
  LT_BACKEND_HOTSPOT_MAX_AGGREGATE_P95_MS
  LT_BACKEND_HOTSPOT_MAX_LIVE_P95_MS
  LT_BACKEND_HOTSPOT_MAX_ENDPOINT_FAIL_RATE
  LT_BACKEND_HOTSPOT_MAX_SNAPSHOTMANY_MEAN_MS
  LT_BACKEND_HOTSPOT_MAX_WINDOWED_SOURCES_MEAN_MS
  LT_BACKEND_HOTSPOT_MAX_FULL_FIELDS_CALLS
  LT_BACKEND_HOTSPOT_MAX_AGGREGATE_P95_REGRESSION_PCT
  LT_BACKEND_HOTSPOT_MAX_LIVE_P95_REGRESSION_PCT
USAGE
}

SUMMARY=""
BASELINE=""
WRITE_REPORT=""
MAX_AGGREGATE_P95_MS="${LT_BACKEND_HOTSPOT_MAX_AGGREGATE_P95_MS:-700}"
MAX_LIVE_P95_MS="${LT_BACKEND_HOTSPOT_MAX_LIVE_P95_MS:-450}"
MAX_ENDPOINT_FAIL_RATE="${LT_BACKEND_HOTSPOT_MAX_ENDPOINT_FAIL_RATE:-0.01}"
MAX_SNAPSHOTMANY_MEAN_MS="${LT_BACKEND_HOTSPOT_MAX_SNAPSHOTMANY_MEAN_MS:-12}"
MAX_WINDOWED_SOURCES_MEAN_MS="${LT_BACKEND_HOTSPOT_MAX_WINDOWED_SOURCES_MEAN_MS:-60}"
MAX_FULL_FIELDS_CALLS="${LT_BACKEND_HOTSPOT_MAX_FULL_FIELDS_CALLS:-500}"
MAX_AGGREGATE_P95_REGRESSION_PCT="${LT_BACKEND_HOTSPOT_MAX_AGGREGATE_P95_REGRESSION_PCT:-40}"
MAX_LIVE_P95_REGRESSION_PCT="${LT_BACKEND_HOTSPOT_MAX_LIVE_P95_REGRESSION_PCT:-40}"

while (($# > 0)); do
  case "$1" in
    --summary)
      SUMMARY="${2:-}"
      shift 2
      ;;
    --baseline)
      BASELINE="${2:-}"
      shift 2
      ;;
    --write-report)
      WRITE_REPORT="${2:-}"
      shift 2
      ;;
    --max-aggregate-p95-ms)
      MAX_AGGREGATE_P95_MS="${2:-}"
      shift 2
      ;;
    --max-live-p95-ms)
      MAX_LIVE_P95_MS="${2:-}"
      shift 2
      ;;
    --max-endpoint-fail-rate)
      MAX_ENDPOINT_FAIL_RATE="${2:-}"
      shift 2
      ;;
    --max-snapshotmany-mean-ms)
      MAX_SNAPSHOTMANY_MEAN_MS="${2:-}"
      shift 2
      ;;
    --max-windowed-sources-mean-ms)
      MAX_WINDOWED_SOURCES_MEAN_MS="${2:-}"
      shift 2
      ;;
    --max-full-fields-calls)
      MAX_FULL_FIELDS_CALLS="${2:-}"
      shift 2
      ;;
    --max-aggregate-p95-regression-pct)
      MAX_AGGREGATE_P95_REGRESSION_PCT="${2:-}"
      shift 2
      ;;
    --max-live-p95-regression-pct)
      MAX_LIVE_P95_REGRESSION_PCT="${2:-}"
      shift 2
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

if [[ -z "${SUMMARY}" ]]; then
  log_fail "--summary is required"
  usage
  exit 1
fi
if [[ ! -f "${SUMMARY}" ]]; then
  log_fail "summary does not exist: ${SUMMARY}"
  exit 1
fi
if [[ -n "${BASELINE}" && ! -f "${BASELINE}" ]]; then
  log_fail "baseline does not exist: ${BASELINE}"
  exit 1
fi

float_le() {
  local lhs=$1
  local rhs=$2
  awk -v lhs="${lhs}" -v rhs="${rhs}" 'BEGIN { exit !(lhs + 0 <= rhs + 0) }'
}

float_pct_delta() {
  local baseline=$1
  local candidate=$2
  awk -v b="${baseline}" -v c="${candidate}" 'BEGIN {
    if (b == 0) {
      if (c == 0) {
        printf "0"
      } else {
        printf "999999"
      }
    } else {
      printf "%.2f", ((c - b) / b) * 100
    }
  }'
}

endpoint_metric() {
  local file=$1
  local endpoint=$2
  local expr=$3
  jq -r --arg endpoint "${endpoint}" --arg expr "${expr}" '
    ((.endpoints // [])[] | select(.endpoint == $endpoint) | .) as $ep
    | if $ep == null then null else
        if $expr == "p95" then ($ep.latency_ms.p95 // null)
        elif $expr == "fail_rate" then
          if (($ep.total_count // 0) <= 0) then 1 else (($ep.fail_count // 0) / ($ep.total_count // 1)) end
        else null end
      end
  ' "${file}"
}

query_metric() {
  local file=$1
  local key=$2
  local metric=$3
  jq -r --arg key "${key}" --arg metric "${metric}" '
    (.key_deltas[$key][$metric] // null)
  ' "${file}"
}

aggregate_p95="$(endpoint_metric "${SUMMARY}" "/status/aggregate" "p95")"
live_p95="$(endpoint_metric "${SUMMARY}" "/status/aggregate/live" "p95")"
aggregate_fail_rate="$(endpoint_metric "${SUMMARY}" "/status/aggregate" "fail_rate")"
live_fail_rate="$(endpoint_metric "${SUMMARY}" "/status/aggregate/live" "fail_rate")"
query_stats_enabled="$(jq -r 'if .query_stats_enabled != null then .query_stats_enabled else ((.top10 | length) > 0) end' "${SUMMARY}")"
query_stats_error="$(jq -r '.query_stats_error // ""' "${SUMMARY}")"
snapshotmany_mean="$(query_metric "${SUMMARY}" "batch_snapshot_lateral" "mean_ms_after")"
snapshot_metric_key="batch_snapshot_lateral"
if [[ "${snapshotmany_mean}" == "null" || -z "${snapshotmany_mean}" ]]; then
  snapshotmany_mean="$(query_metric "${SUMMARY}" "snapshot_single_distinct" "mean_ms_after")"
  snapshot_metric_key="snapshot_single_distinct"
fi
windowed_sources_mean="$(query_metric "${SUMMARY}" "sources_groupby_windowed" "mean_ms_after")"
full_fields_calls="$(query_metric "${SUMMARY}" "queryevents_full_fields" "calls_delta")"
if [[ "${full_fields_calls}" == "null" || -z "${full_fields_calls}" ]]; then
  full_fields_calls=0
fi

failures=()

if [[ "${aggregate_p95}" == "null" || -z "${aggregate_p95}" ]]; then
  failures+=("missing /status/aggregate p95 in summary")
elif ! float_le "${aggregate_p95}" "${MAX_AGGREGATE_P95_MS}"; then
  failures+=("/status/aggregate p95 ${aggregate_p95}ms exceeds ${MAX_AGGREGATE_P95_MS}ms")
fi

if [[ "${live_p95}" == "null" || -z "${live_p95}" ]]; then
  failures+=("missing /status/aggregate/live p95 in summary")
elif ! float_le "${live_p95}" "${MAX_LIVE_P95_MS}"; then
  failures+=("/status/aggregate/live p95 ${live_p95}ms exceeds ${MAX_LIVE_P95_MS}ms")
fi

if [[ "${aggregate_fail_rate}" == "null" || -z "${aggregate_fail_rate}" ]]; then
  failures+=("missing /status/aggregate fail_rate in summary")
elif ! float_le "${aggregate_fail_rate}" "${MAX_ENDPOINT_FAIL_RATE}"; then
  failures+=("/status/aggregate fail rate ${aggregate_fail_rate} exceeds ${MAX_ENDPOINT_FAIL_RATE}")
fi

if [[ "${live_fail_rate}" == "null" || -z "${live_fail_rate}" ]]; then
  failures+=("missing /status/aggregate/live fail_rate in summary")
elif ! float_le "${live_fail_rate}" "${MAX_ENDPOINT_FAIL_RATE}"; then
  failures+=("/status/aggregate/live fail rate ${live_fail_rate} exceeds ${MAX_ENDPOINT_FAIL_RATE}")
fi

if [[ "${query_stats_enabled}" != "true" ]]; then
  if [[ -n "${query_stats_error}" ]]; then
    failures+=("query stats unavailable: ${query_stats_error}")
  else
    failures+=("query stats unavailable: summary.query_stats_enabled=false (ensure pg_stat_statements is enabled)")
  fi
else
  if [[ "${snapshotmany_mean}" == "null" || -z "${snapshotmany_mean}" ]]; then
    failures+=("missing snapshot query mean_ms_after (expected key_deltas.batch_snapshot_lateral or key_deltas.snapshot_single_distinct)")
  elif ! float_le "${snapshotmany_mean}" "${MAX_SNAPSHOTMANY_MEAN_MS}"; then
    failures+=("${snapshot_metric_key} mean ${snapshotmany_mean}ms exceeds ${MAX_SNAPSHOTMANY_MEAN_MS}ms")
  fi

  if [[ "${windowed_sources_mean}" == "null" || -z "${windowed_sources_mean}" ]]; then
    failures+=("missing key_deltas.sources_groupby_windowed.mean_ms_after (windowed sources query contract)")
  elif ! float_le "${windowed_sources_mean}" "${MAX_WINDOWED_SOURCES_MEAN_MS}"; then
    failures+=("windowed sources mean ${windowed_sources_mean}ms exceeds ${MAX_WINDOWED_SOURCES_MEAN_MS}ms")
  fi

  if ! float_le "${full_fields_calls}" "${MAX_FULL_FIELDS_CALLS}"; then
    failures+=("full-fields QueryEvents calls_delta ${full_fields_calls} exceeds ${MAX_FULL_FIELDS_CALLS}")
  fi
fi

aggregate_p95_regression_pct=""
live_p95_regression_pct=""
if [[ -n "${BASELINE}" ]]; then
  baseline_aggregate_p95="$(endpoint_metric "${BASELINE}" "/status/aggregate" "p95")"
  baseline_live_p95="$(endpoint_metric "${BASELINE}" "/status/aggregate/live" "p95")"

  if [[ "${baseline_aggregate_p95}" != "null" && -n "${baseline_aggregate_p95}" ]]; then
    aggregate_p95_regression_pct="$(float_pct_delta "${baseline_aggregate_p95}" "${aggregate_p95}")"
    if ! float_le "${aggregate_p95_regression_pct}" "${MAX_AGGREGATE_P95_REGRESSION_PCT}"; then
      failures+=("/status/aggregate p95 regression ${aggregate_p95_regression_pct}% exceeds ${MAX_AGGREGATE_P95_REGRESSION_PCT}%")
    fi
  fi

  if [[ "${baseline_live_p95}" != "null" && -n "${baseline_live_p95}" ]]; then
    live_p95_regression_pct="$(float_pct_delta "${baseline_live_p95}" "${live_p95}")"
    if ! float_le "${live_p95_regression_pct}" "${MAX_LIVE_P95_REGRESSION_PCT}"; then
      failures+=("/status/aggregate/live p95 regression ${live_p95_regression_pct}% exceeds ${MAX_LIVE_P95_REGRESSION_PCT}%")
    fi
  fi
fi

passed=1
if [[ ${#failures[@]} -gt 0 ]]; then
  passed=0
fi

log_info "=== Backend Hotspot Threshold Check ==="
log_info "Summary: ${SUMMARY}"
if [[ -n "${BASELINE}" ]]; then
  log_info "Baseline: ${BASELINE}"
fi
log_info ""
log_info "Metrics:"
log_info "  /status/aggregate p95 ms: ${aggregate_p95}"
log_info "  /status/aggregate/live p95 ms: ${live_p95}"
log_info "  /status/aggregate fail rate: ${aggregate_fail_rate}"
log_info "  /status/aggregate/live fail rate: ${live_fail_rate}"
log_info "  query_stats_enabled: ${query_stats_enabled}"
if [[ -n "${query_stats_error}" ]]; then
  log_info "  query_stats_error: ${query_stats_error}"
fi
log_info "  snapshot_query_key: ${snapshot_metric_key}"
log_info "  SnapshotMany lateral mean ms: ${snapshotmany_mean}"
log_info "  Windowed sources mean ms: ${windowed_sources_mean}"
log_info "  QueryEvents full-fields calls_delta: ${full_fields_calls}"
if [[ -n "${aggregate_p95_regression_pct}" ]]; then
  log_info "  /status/aggregate p95 regression %: ${aggregate_p95_regression_pct}"
fi
if [[ -n "${live_p95_regression_pct}" ]]; then
  log_info "  /status/aggregate/live p95 regression %: ${live_p95_regression_pct}"
fi
log_info ""
log_info "Thresholds:"
log_info "  max_aggregate_p95_ms=${MAX_AGGREGATE_P95_MS}"
log_info "  max_live_p95_ms=${MAX_LIVE_P95_MS}"
log_info "  max_endpoint_fail_rate=${MAX_ENDPOINT_FAIL_RATE}"
log_info "  max_snapshotmany_mean_ms=${MAX_SNAPSHOTMANY_MEAN_MS}"
log_info "  max_windowed_sources_mean_ms=${MAX_WINDOWED_SOURCES_MEAN_MS}"
log_info "  max_full_fields_calls=${MAX_FULL_FIELDS_CALLS}"
if [[ -n "${BASELINE}" ]]; then
  log_info "  max_aggregate_p95_regression_pct=${MAX_AGGREGATE_P95_REGRESSION_PCT}"
  log_info "  max_live_p95_regression_pct=${MAX_LIVE_P95_REGRESSION_PCT}"
fi

if [[ -n "${WRITE_REPORT}" ]]; then
  mkdir -p "$(dirname "${WRITE_REPORT}")"
  jq -n \
    --arg summary "${SUMMARY}" \
    --arg baseline "${BASELINE}" \
    --argjson passed "${passed}" \
    --argjson aggregate_p95 "${aggregate_p95:-0}" \
    --argjson live_p95 "${live_p95:-0}" \
    --argjson aggregate_fail_rate "${aggregate_fail_rate:-0}" \
    --argjson live_fail_rate "${live_fail_rate:-0}" \
    --argjson query_stats_enabled "${query_stats_enabled}" \
    --arg query_stats_error "${query_stats_error}" \
    --arg snapshot_metric_key "${snapshot_metric_key}" \
    --argjson snapshotmany_mean "${snapshotmany_mean:-0}" \
    --argjson windowed_sources_mean "${windowed_sources_mean:-0}" \
    --argjson full_fields_calls "${full_fields_calls:-0}" \
    --arg aggregate_p95_regression_pct "${aggregate_p95_regression_pct}" \
    --arg live_p95_regression_pct "${live_p95_regression_pct}" \
    --argjson failures "$(printf '%s\n' "${failures[@]:-}" | jq -R -s -c 'split("\n") | map(select(length>0))')" \
    --argjson max_aggregate_p95_ms "${MAX_AGGREGATE_P95_MS}" \
    --argjson max_live_p95_ms "${MAX_LIVE_P95_MS}" \
    --argjson max_endpoint_fail_rate "${MAX_ENDPOINT_FAIL_RATE}" \
    --argjson max_snapshotmany_mean_ms "${MAX_SNAPSHOTMANY_MEAN_MS}" \
    --argjson max_windowed_sources_mean_ms "${MAX_WINDOWED_SOURCES_MEAN_MS}" \
    --argjson max_full_fields_calls "${MAX_FULL_FIELDS_CALLS}" \
    --argjson max_aggregate_p95_regression_pct "${MAX_AGGREGATE_P95_REGRESSION_PCT}" \
    --argjson max_live_p95_regression_pct "${MAX_LIVE_P95_REGRESSION_PCT}" \
    '{
      captured_at: now | todate,
      summary: $summary,
      baseline: (if $baseline == "" then null else $baseline end),
      passed: ($passed == 1),
      metrics: {
        aggregate_p95_ms: $aggregate_p95,
        live_p95_ms: $live_p95,
        aggregate_fail_rate: $aggregate_fail_rate,
        live_fail_rate: $live_fail_rate,
        query_stats_enabled: $query_stats_enabled,
        query_stats_error: (if $query_stats_error == "" then null else $query_stats_error end),
        snapshot_metric_key: $snapshot_metric_key,
        snapshotmany_mean_ms: $snapshotmany_mean,
        windowed_sources_mean_ms: $windowed_sources_mean,
        full_fields_calls_delta: $full_fields_calls,
        aggregate_p95_regression_pct: (if $aggregate_p95_regression_pct == "" then null else ($aggregate_p95_regression_pct | tonumber) end),
        live_p95_regression_pct: (if $live_p95_regression_pct == "" then null else ($live_p95_regression_pct | tonumber) end)
      },
      thresholds: {
        max_aggregate_p95_ms: $max_aggregate_p95_ms,
        max_live_p95_ms: $max_live_p95_ms,
        max_endpoint_fail_rate: $max_endpoint_fail_rate,
        max_snapshotmany_mean_ms: $max_snapshotmany_mean_ms,
        max_windowed_sources_mean_ms: $max_windowed_sources_mean_ms,
        max_full_fields_calls: $max_full_fields_calls,
        max_aggregate_p95_regression_pct: $max_aggregate_p95_regression_pct,
        max_live_p95_regression_pct: $max_live_p95_regression_pct
      },
      failures: $failures
    }' > "${WRITE_REPORT}"
  log_info "Report: ${WRITE_REPORT}"
fi

if [[ ${#failures[@]} -gt 0 ]]; then
  for failure in "${failures[@]}"; do
    log_fail "${failure}"
  done
  log_fail "backend hotspot threshold check failed"
  exit 1
fi

log_pass "backend hotspot threshold check passed"
