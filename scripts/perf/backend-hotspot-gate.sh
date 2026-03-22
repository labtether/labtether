#!/usr/bin/env bash
# Deterministic perf-contract gate for backend hotspot mitigations.
# Runs focused tests that protect query-shape and cache behaviors tied to
# status aggregate/logs hotspot reductions.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=/dev/null
source "${PROJECT_ROOT}/scripts/lib/script-common.sh"

if ! require_command go; then
  exit 1
fi

log_info "=== Backend Hotspot Perf Gate ==="

log_info "Running persistence projection/caching perf-contract tests..."
go test ./internal/persistence -count=1 -run 'TestMemory(LogStoreQueryEventsExcludeFields|LogStoreQueryEventsFieldKeysProjection|LogStoreQueryEventsSiteFilterWithAssetFallback|TelemetryStoreSnapshotManyReturnsLatestValues)$'

log_info "Running status aggregate hotspot perf-contract tests..."
go test ./cmd/labtether -count=1 -run 'Test(StatusLogSourcesSiteFilterUsesProjectedSiteField|HandleLogSourcesSiteFilterUsesProjectedSiteField|SiteReliabilityComputationUsesProjectedSiteField|MetricsOverviewUsesBatchTelemetrySnapshots|StatusTelemetryOverviewUsesBatchTelemetrySnapshots|StatusTelemetryOverviewBatchCacheReusesSnapshots|StatusTelemetryOverviewBatchCacheInvalidatesOnAssetSetChange)$'

log_pass "backend hotspot perf gate passed"
