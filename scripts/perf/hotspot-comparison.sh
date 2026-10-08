#!/usr/bin/env bash
# Read baseline summaries and render comparative performance reports.

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
  local newest=""
  for candidate in \
    "${OUTPUT_ROOT}"/*-"${LABEL}"/summary.json \
    "${OUTPUT_ROOT}"/*-"${LABEL}".*/summary.json; do
    if [[ "${candidate}" == "${outdir}/summary.json" ]]; then
      continue
    fi
    if [[ -f "${candidate}" && ( -z "$newest" || "$candidate" -nt "$newest" ) ]]; then
      newest="$candidate"
    fi
  done
  if [[ -n "$newest" ]]; then
    printf '%s\n' "$newest"
    return
  fi

  newest=""
  for candidate in "${OUTPUT_ROOT}"/*/summary.json; do
    if [[ "${candidate}" == "${outdir}/summary.json" ]]; then
      continue
    fi
    if [[ -f "${candidate}" && ( -z "$newest" || "$candidate" -nt "$newest" ) ]]; then
      newest="$candidate"
    fi
  done
  [[ -z "$newest" ]] || printf '%s\n' "$newest"
}

build_compare_report() {
  local baseline_summary=$1
  local compare_file
  compare_file="${outdir}/compare-vs-$(basename "$(dirname "${baseline_summary}")").md"
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

  get_snapshot_metric() {
    local file=$1
    local metric=$2
    jq -r --arg metric "${metric}" '
      (.key_deltas.batch_snapshot_distinct[$metric]
       // .key_deltas.batch_snapshot_lateral[$metric]
       // .key_deltas.batch_snapshot_new_lateral[$metric]
       // .key_deltas.snapshot_single_distinct[$metric]
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
  base_batch_total="$(get_snapshot_metric "${baseline_summary}" "total_ms_delta")"
  cand_batch_total="$(get_snapshot_metric "${candidate_summary}" "total_ms_delta")"
  base_batch_mean="$(get_snapshot_metric "${baseline_summary}" "mean_ms_after")"
  cand_batch_mean="$(get_snapshot_metric "${candidate_summary}" "mean_ms_after")"

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
