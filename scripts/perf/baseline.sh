#!/usr/bin/env bash
# LabTether status-path baseline measurement.
#
# Measures both the console proxy and backend aggregate endpoints using either:
# - bearer token auth, or
# - local session login with username/password
#
# This script is intentionally aligned with the current local runtime:
# - console may be HTTPS with a self-signed cert
# - backend may be HTTP :8080 or HTTPS :8443
# - status endpoints require auth
#
# Usage:
#   ./scripts/perf/baseline.sh
#   ./scripts/perf/baseline.sh --samples 5 --console-base https://127.0.0.1:3000 --api-base https://127.0.0.1:8443
#   ./scripts/perf/baseline.sh --offline          # emit process/machine snapshot only, skip HTTP checks
#   ./scripts/perf/baseline.sh --skip-endpoints   # skip endpoint measurement, still auth-probes if services found

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

# Hard-required: always needed regardless of mode.
for cmd in awk curl date jq mktemp ps sed sort; do
  if ! require_command "${cmd}"; then
    exit 1
  fi
done
# Optional: pgrep is present on all supported platforms but degrade gracefully if missing.
# sysctl is macOS-only; /proc fallbacks are used on Linux.

usage() {
  cat <<'USAGE'
LabTether status-path baseline measurement

Options:
  --console-base URL     Console origin (auto-detect by default)
  --api-base URL         Backend API origin (auto-detect by default)
  --token TOKEN          Bearer token for backend and trusted console proxy routes
  --username USER        Login username when bearer token is not provided (default: admin)
  --password PASS        Login password when bearer token is not provided (default: password)
  --samples N            Calls per endpoint (default: 3)
  --output-root DIR      Artifact root (default: tmp/perf/runs)
  --connect-timeout N    Seconds before a probe/curl connection times out (default: 3)
  --offline              Emit machine/process snapshot only; skip all HTTP checks and endpoint
                         measurement. Exits 0. Useful for validating the harness without a
                         running backend.
  --skip-endpoints       Skip endpoint measurement but still detect services and auth.
                         Exits 0 when services are not found (warn-only, no hard fail).
  -h, --help             Show help

Environment fallbacks:
  LABTETHER_CONSOLE_BASE_URL
  LABTETHER_API_BASE_URL
  LABTETHER_API_TOKEN / LABTETHER_OWNER_TOKEN
  LABTETHER_ADMIN_USERNAME / LABTETHER_ADMIN_PASSWORD
  LABTETHER_PERF_CONNECT_TIMEOUT   (connect timeout in seconds, default: 3)
USAGE
}

CONSOLE_BASE="${LABTETHER_CONSOLE_BASE_URL:-}"
API_BASE="${LABTETHER_API_BASE_URL:-}"
AUTH_TOKEN="${LABTETHER_API_TOKEN:-${LABTETHER_OWNER_TOKEN:-}}"
LOGIN_USERNAME="${LABTETHER_ADMIN_USERNAME:-admin}"
LOGIN_PASSWORD="${LABTETHER_ADMIN_PASSWORD:-password}"
SAMPLES=3
OUTPUT_ROOT="tmp/perf/runs"
CONNECT_TIMEOUT="${LABTETHER_PERF_CONNECT_TIMEOUT:-3}"
OFFLINE=0
SKIP_ENDPOINTS=0

while (($# > 0)); do
  case "$1" in
    --console-base)
      CONSOLE_BASE="${2:-}"
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
    --samples)
      SAMPLES="${2:-}"
      shift 2
      ;;
    --output-root)
      OUTPUT_ROOT="${2:-}"
      shift 2
      ;;
    --connect-timeout)
      CONNECT_TIMEOUT="${2:-}"
      shift 2
      ;;
    --offline)
      OFFLINE=1
      shift 1
      ;;
    --skip-endpoints)
      SKIP_ENDPOINTS=1
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

if ! [[ "${SAMPLES}" =~ ^[0-9]+$ ]] || [[ "${SAMPLES}" -le 0 ]]; then
  log_fail "--samples must be a positive integer"
  exit 1
fi

if ! [[ "${CONNECT_TIMEOUT}" =~ ^[0-9]+$ ]] || [[ "${CONNECT_TIMEOUT}" -le 0 ]]; then
  log_fail "--connect-timeout must be a positive integer"
  exit 1
fi

timestamp="$(date +%Y%m%d-%H%M%S)"
OUTDIR="${OUTPUT_ROOT}/${timestamp}-status-baseline"
mkdir -p "${OUTDIR}"

CONSOLE_COOKIE_JAR=""
BACKEND_COOKIE_JAR=""
cleanup() {
  if [[ -n "${CONSOLE_COOKIE_JAR}" && -f "${CONSOLE_COOKIE_JAR}" ]]; then
    rm -f "${CONSOLE_COOKIE_JAR}"
  fi
  if [[ -n "${BACKEND_COOKIE_JAR}" && -f "${BACKEND_COOKIE_JAR}" ]]; then
    rm -f "${BACKEND_COOKIE_JAR}"
  fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Machine summary: cross-platform (macOS sysctl vs Linux /proc)
# ---------------------------------------------------------------------------
machine_cpu_count() {
  if command -v sysctl >/dev/null 2>&1 && sysctl -n hw.ncpu >/dev/null 2>&1; then
    sysctl -n hw.ncpu
  elif [[ -f /proc/cpuinfo ]]; then
    awk '/^processor/ { count++ } END { print count+0 }' /proc/cpuinfo
  else
    printf 'unknown'
  fi
}

machine_mem_gb() {
  if command -v sysctl >/dev/null 2>&1 && sysctl -n hw.memsize >/dev/null 2>&1; then
    sysctl -n hw.memsize | awk '{printf "%.0f GB", $0/1073741824}'
  elif [[ -f /proc/meminfo ]]; then
    awk '/^MemTotal:/ {printf "%.0f GB", $2/1048576}' /proc/meminfo
  else
    printf 'unknown'
  fi
}

# ---------------------------------------------------------------------------
# Process thread count: macOS uses ps -M, Linux uses ps -o nlwp=
# ---------------------------------------------------------------------------
process_thread_count() {
  local pid=$1
  local count
  # macOS: ps -M lists one line per thread; subtract 1 for the header.
  # Linux: ps -o nlwp= prints the thread count directly.
  if ps -M -p "${pid}" >/dev/null 2>&1; then
    count="$(ps -M -p "${pid}" 2>/dev/null | wc -l | tr -d ' ')"
    # subtract header line
    printf '%d' "$((count - 1))"
  elif ps -o nlwp= -p "${pid}" >/dev/null 2>&1; then
    ps -o nlwp= -p "${pid}" 2>/dev/null | tr -d ' '
  else
    printf '?'
  fi
}

machine_summary="$(machine_cpu_count) cores, $(machine_mem_gb)"

# ---------------------------------------------------------------------------
# Service detection helpers — silent probes, no curl noise on stderr
# ---------------------------------------------------------------------------
is_reachable() {
  local url=$1
  local expected_kind=$2
  local status
  if [[ "${expected_kind}" == "console" ]]; then
    status="$(curl -k -s --max-time "${CONNECT_TIMEOUT}" -o /dev/null -w '%{http_code}' \
      "${url}/api/auth/bootstrap/status" 2>/dev/null || true)"
    [[ "${status}" != "000" && "${status}" != "404" && -n "${status}" ]]
    return
  fi

  status="$(curl -k -s --max-time "${CONNECT_TIMEOUT}" -o /dev/null -w '%{http_code}' \
    "${url}/healthz" 2>/dev/null || true)"
  [[ "${status}" == "200" ]]
}

auto_pick_base() {
  local kind=$1
  shift
  local candidate=""
  for candidate in "$@"; do
    if [[ -z "${candidate}" ]]; then
      continue
    fi
    if is_reachable "${candidate}" "${kind}"; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 1
}

trimmed_or_empty() {
  local value=${1:-}
  printf '%s' "${value#"${value%%[![:space:]]*}"}" | sed 's/[[:space:]]*$//'
}

# ---------------------------------------------------------------------------
# Process discovery: gracefully degrade when pgrep is absent
# ---------------------------------------------------------------------------
find_process_pid() {
  local pattern=$1
  if command -v pgrep >/dev/null 2>&1; then
    pgrep -f "${pattern}" 2>/dev/null | head -1 || true
  else
    ps aux 2>/dev/null | awk -v pat="${pattern}" '$0 ~ pat && !/awk/ { print $2; exit }' || true
  fi
}

backend_pid="$(find_process_pid 'build/labtether|cmd/labtether|bin/labtether')"
frontend_pid="$(find_process_pid 'next-server|next dev')"

log_info "=== LabTether Performance Baseline ==="
log_info "Timestamp: $(date -Iseconds 2>/dev/null || date '+%Y-%m-%dT%H:%M:%S%z')"
log_info "Machine: ${machine_summary}"
log_info "Samples per endpoint: ${SAMPLES}"
log_info "Connect timeout: ${CONNECT_TIMEOUT}s"
if [[ "${OFFLINE}" -eq 1 ]]; then
  log_info "Mode: offline (process snapshot only)"
fi
if [[ "${SKIP_ENDPOINTS}" -eq 1 && "${OFFLINE}" -eq 0 ]]; then
  log_info "Mode: skip-endpoints (service detection only)"
fi
log_info "Artifact dir: ${OUTDIR}"
log_info ""

printf "%-20s %-8s %-8s %-10s %-8s\n" "Process" "PID" "CPU%" "RSS(MB)" "Threads"
printf '%s\n' "------------------------------------------------------------------------"
for label_pid in "backend:${backend_pid}" "frontend:${frontend_pid}"; do
  label="${label_pid%%:*}"
  pid="${label_pid##*:}"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
    cpu="$(ps -o %cpu= -p "${pid}" 2>/dev/null | tr -d ' ' || printf '?')"
    rss="$(ps -o rss= -p "${pid}" 2>/dev/null | awk '{printf "%.0f", $1/1024}' || printf '?')"
    threads="$(process_thread_count "${pid}")"
    printf "%-20s %-8s %-8s %-10s %-8s\n" "${label}" "${pid}" "${cpu}" "${rss}" "${threads}"
  else
    printf "%-20s %-8s (not running)\n" "${label}" "${pid:-?}"
  fi
done

# Offline mode: exit cleanly after the process snapshot.
if [[ "${OFFLINE}" -eq 1 ]]; then
  log_info ""
  log_info "Offline mode: skipping service detection and endpoint measurement."
  exit 0
fi

# ---------------------------------------------------------------------------
# Service detection
# ---------------------------------------------------------------------------
CONSOLE_BASE="$(trimmed_or_empty "${CONSOLE_BASE}")"
API_BASE="$(trimmed_or_empty "${API_BASE}")"

if [[ -z "${CONSOLE_BASE}" ]]; then
  CONSOLE_BASE="$(auto_pick_base console \
    "https://127.0.0.1:3000" \
    "https://localhost:3000" \
    "http://127.0.0.1:3000" \
    "http://localhost:3000" || true)"
fi

if [[ -z "${API_BASE}" ]]; then
  API_BASE="$(auto_pick_base backend \
    "http://127.0.0.1:8080" \
    "http://localhost:8080" \
    "https://127.0.0.1:8443" \
    "https://localhost:8443" || true)"
fi

if [[ -z "${CONSOLE_BASE}" ]]; then
  if [[ "${SKIP_ENDPOINTS}" -eq 1 ]]; then
    log_warn "console not reachable; skipping endpoint measurement (--skip-endpoints mode)"
    log_info "Pass --console-base <url> to target a specific origin."
    exit 0
  fi
  log_fail "could not auto-detect a reachable console base; pass --console-base"
  log_info "Tip: pass --offline to capture a process snapshot without a running backend."
  exit 1
fi
if [[ -z "${API_BASE}" ]]; then
  if [[ "${SKIP_ENDPOINTS}" -eq 1 ]]; then
    log_warn "backend not reachable; skipping endpoint measurement (--skip-endpoints mode)"
    log_info "Pass --api-base <url> to target a specific origin."
    exit 0
  fi
  log_fail "could not auto-detect a reachable backend API base; pass --api-base"
  log_info "Tip: pass --offline to capture a process snapshot without a running backend."
  exit 1
fi

log_info "Console base: ${CONSOLE_BASE}"
log_info "API base: ${API_BASE}"
log_info ""

# Skip-endpoints: service info logged, exit cleanly.
if [[ "${SKIP_ENDPOINTS}" -eq 1 ]]; then
  log_info "Services detected; skipping endpoint measurement (--skip-endpoints mode)."
  exit 0
fi

login_with_session() {
  local kind=$1
  local base=$2
  local cookie_jar=$3
  local path=$4
  local response_body
  response_body="$(mktemp)"
  local payload
  payload="$(jq -n --arg username "${LOGIN_USERNAME}" --arg password "${LOGIN_PASSWORD}" '{username: $username, password: $password}')"
  local code
  code="$(curl -k -sS -L \
    --max-time "$((CONNECT_TIMEOUT * 5))" \
    -c "${cookie_jar}" \
    -o "${response_body}" \
    -w '%{http_code}' \
    -H 'Content-Type: application/json' \
    -X POST \
    --data "${payload}" \
    "${base}${path}" || true)"
  if [[ "${code}" != "200" ]]; then
    log_fail "${kind} login failed (${base}${path} -> ${code})"
    cat "${response_body}" > "${OUTDIR}/${kind}-login-error.json"
    rm -f "${response_body}"
    exit 1
  fi
  if jq -e '.requires_2fa == true' "${response_body}" >/dev/null 2>&1; then
    log_fail "${kind} login requires 2FA; pass --token or use a local non-2FA baseline account"
    cat "${response_body}" > "${OUTDIR}/${kind}-login-error.json"
    rm -f "${response_body}"
    exit 1
  fi
  rm -f "${response_body}"
}

if [[ -n "${AUTH_TOKEN}" ]]; then
  log_info "Auth mode: bearer token"
else
  log_info "Auth mode: session login (${LOGIN_USERNAME})"
  CONSOLE_COOKIE_JAR="$(mktemp)"
  BACKEND_COOKIE_JAR="$(mktemp)"
  login_with_session "console" "${CONSOLE_BASE}" "${CONSOLE_COOKIE_JAR}" "/api/auth/login"
  login_with_session "backend" "${API_BASE}" "${BACKEND_COOKIE_JAR}" "/auth/login"
fi

summarize_body() {
  local body_file=$1
  jq -c '{
    summary: {
      assetCount: (.summary.assetCount // null),
      groupCount: (.summary.groupCount // null),
      connectorCount: (.summary.connectorCount // null),
      sessionCount: (.summary.sessionCount // null),
      staleAssetCount: (.summary.staleAssetCount // null)
    },
    counts: {
      endpoints: ((.endpoints // []) | length),
      assets: ((.assets // []) | length),
      telemetryOverview: ((.telemetryOverview // []) | length),
      recentLogs: ((.recentLogs // []) | length),
      sessions: ((.sessions // []) | length),
      updateRuns: ((.updateRuns // []) | length)
    }
  }' "${body_file}" 2>/dev/null || printf '{}\n'
}

run_samples() {
  local label=$1
  local base=$2
  local path=$3
  local auth_mode=$4
  local cookie_jar=$5
  local raw_file="${OUTDIR}/${label}.tsv"
  local summary_file="${OUTDIR}/${label}.summary.json"
  local first_success_body=""

  : > "${raw_file}"
  for i in $(seq 1 "${SAMPLES}"); do
    local headers_file body_file curl_output status size ttfb total etag
    headers_file="$(mktemp)"
    body_file="$(mktemp)"
    if [[ "${auth_mode}" == "token" ]]; then
      curl_output="$(curl -k -sS -D "${headers_file}" -o "${body_file}" \
        --max-time "$((CONNECT_TIMEOUT * 10))" \
        -H "Authorization: Bearer ${AUTH_TOKEN}" \
        -w '%{http_code}\t%{size_download}\t%{time_starttransfer}\t%{time_total}' \
        "${base}${path}" || true)"
    else
      curl_output="$(curl -k -sS -D "${headers_file}" -o "${body_file}" \
        --max-time "$((CONNECT_TIMEOUT * 10))" \
        -b "${cookie_jar}" \
        -w '%{http_code}\t%{size_download}\t%{time_starttransfer}\t%{time_total}' \
        "${base}${path}" || true)"
    fi

    status="$(printf '%s' "${curl_output}" | awk -F '\t' '{print $1}')"
    size="$(printf '%s' "${curl_output}" | awk -F '\t' '{print $2}')"
    ttfb="$(printf '%s' "${curl_output}" | awk -F '\t' '{print $3}')"
    total="$(printf '%s' "${curl_output}" | awk -F '\t' '{print $4}')"
    etag="$(awk 'BEGIN{IGNORECASE=1} /^etag:/ { sub(/\r$/, "", $2); print $2; exit }' "${headers_file}")"
    printf "%s\t%s\t%s\t%s\t%s\t%s\n" "${i}" "${status}" "${size}" "${ttfb}" "${total}" "${etag:-}" >> "${raw_file}"

    if [[ -z "${first_success_body}" && "${status}" == "200" ]]; then
      first_success_body="${OUTDIR}/${label}.body.json"
      mv "${body_file}" "${first_success_body}"
    else
      rm -f "${body_file}"
    fi
    rm -f "${headers_file}"
  done

  local ok_count fail_count mean_ms p50_ms p95_ms max_ms
  ok_count="$(awk -F '\t' '$2 == "200" { c++ } END { print c + 0 }' "${raw_file}")"
  fail_count="$((SAMPLES - ok_count))"
  if [[ "${ok_count}" -eq 0 ]]; then
    log_warn "${label}: no successful responses captured"
    jq -n \
      --arg label "${label}" \
      --arg path "${path}" \
      --arg base "${base}" \
      --argjson samples "${SAMPLES}" \
      --argjson ok_count "${ok_count}" \
      --argjson fail_count "${fail_count}" \
      '{label: $label, base: $base, path: $path, samples: $samples, ok_count: $ok_count, fail_count: $fail_count}' \
      > "${summary_file}"
    return
  fi

  local lat_file
  lat_file="$(mktemp)"
  awk -F '\t' '$2 == "200" { printf "%.3f\n", $5 * 1000.0 }' "${raw_file}" | sort -n > "${lat_file}"
  mean_ms="$(awk -F '\t' '$2 == "200" { total += ($5 * 1000.0); count++ } END { if (count == 0) print 0; else printf "%.3f", total / count }' "${raw_file}")"
  p50_ms="$(awk '{
    vals[NR]=$1
    count=NR
  } END {
    if (count == 0) {
      print 0
      exit
    }
    idx = int((count * 50 + 99) / 100)
    if (idx < 1) idx = 1
    if (idx > count) idx = count
    print vals[idx]
  }' "${lat_file}")"
  p95_ms="$(awk '{
    vals[NR]=$1
    count=NR
  } END {
    if (count == 0) {
      print 0
      exit
    }
    idx = int((count * 95 + 99) / 100)
    if (idx < 1) idx = 1
    if (idx > count) idx = count
    print vals[idx]
  }' "${lat_file}")"
  max_ms="$(awk 'END { if (NR == 0) print 0; else print $1 }' "${lat_file}")"
  rm -f "${lat_file}"

  jq -n \
    --arg label "${label}" \
    --arg path "${path}" \
    --arg base "${base}" \
    --argjson samples "${SAMPLES}" \
    --argjson ok_count "${ok_count}" \
    --argjson fail_count "${fail_count}" \
    --arg mean_ms "${mean_ms}" \
    --arg p50_ms "${p50_ms}" \
    --arg p95_ms "${p95_ms}" \
    --arg max_ms "${max_ms}" \
    --arg payload_summary "$(if [[ -n "${first_success_body}" ]]; then summarize_body "${first_success_body}"; else printf '{}'; fi)" \
    '{
      label: $label,
      base: $base,
      path: $path,
      samples: $samples,
      ok_count: $ok_count,
      fail_count: $fail_count,
      mean_ms: ($mean_ms | tonumber),
      p50_ms: ($p50_ms | tonumber),
      p95_ms: ($p95_ms | tonumber),
      max_ms: ($max_ms | tonumber),
      payload_summary: ($payload_summary | fromjson)
    }' > "${summary_file}"

  printf "%-20s code=%-4s ok=%-3s mean=%-9sms p50=%-9sms p95=%-9sms max=%-9sms sample=%s\n" \
    "${label}" "200" "${ok_count}/${SAMPLES}" "${mean_ms}" "${p50_ms}" "${p95_ms}" "${max_ms}" "${summary_file}"
}

log_info ""
log_info "=== Endpoint Baseline ==="
auth_mode="token"
if [[ -z "${AUTH_TOKEN}" ]]; then
  auth_mode="session"
fi
run_samples "console-status" "${CONSOLE_BASE}" "/api/status" "${auth_mode}" "${CONSOLE_COOKIE_JAR}"
run_samples "console-status-live" "${CONSOLE_BASE}" "/api/status/live" "${auth_mode}" "${CONSOLE_COOKIE_JAR}"
run_samples "backend-status-aggregate" "${API_BASE}" "/status/aggregate" "${auth_mode}" "${BACKEND_COOKIE_JAR}"
run_samples "backend-status-live" "${API_BASE}" "/status/aggregate/live" "${auth_mode}" "${BACKEND_COOKIE_JAR}"

jq -n \
  --arg timestamp "$(date -Iseconds 2>/dev/null || date '+%Y-%m-%dT%H:%M:%S%z')" \
  --arg console_base "${CONSOLE_BASE}" \
  --arg api_base "${API_BASE}" \
  --argjson samples "${SAMPLES}" \
  --slurpfile console_status "${OUTDIR}/console-status.summary.json" \
  --slurpfile console_status_live "${OUTDIR}/console-status-live.summary.json" \
  --slurpfile backend_status "${OUTDIR}/backend-status-aggregate.summary.json" \
  --slurpfile backend_status_live "${OUTDIR}/backend-status-live.summary.json" \
  '{
    timestamp: $timestamp,
    console_base: $console_base,
    api_base: $api_base,
    samples: $samples,
    endpoints: [
      $console_status[0],
      $console_status_live[0],
      $backend_status[0],
      $backend_status_live[0]
    ]
  }' > "${OUTDIR}/summary.json"

log_info ""
log_info "Summary written to ${OUTDIR}/summary.json"
