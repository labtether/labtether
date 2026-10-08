#!/usr/bin/env bash
# Read-only platform/process observations for performance reports.

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

find_process_pid() {
  local pattern=$1
  if command -v pgrep >/dev/null 2>&1; then
    pgrep -f "${pattern}" 2>/dev/null | head -1 || true
  else
    ps aux 2>/dev/null | awk -v pat="${pattern}" '$0 ~ pat && !/awk/ { print $2; exit }' || true
  fi
}
