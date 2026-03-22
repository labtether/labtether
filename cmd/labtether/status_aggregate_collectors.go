package main

import (
	"time"

	"github.com/labtether/labtether/internal/logs"
	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// status_aggregate_collectors.go — thin stub.
//
// All implementation has moved to internal/hubapi/statusagg/.
// Type aliases and thin wrappers are provided so test files and other callers
// in cmd/labtether/ continue to compile without modification.

// statusAggregateLogWatermarkReader is an alias for the canonical interface.
type statusAggregateLogWatermarkReader = statusaggpkg.LogWatermarkReader

// statusAggregateLogSources wraps the exported package function for test
// callers that use the old package-level name.
func statusAggregateLogSources(events []logs.Event, limit int) []logs.SourceSummary {
	return statusaggpkg.AggregateLogSources(events, limit)
}

// statusLogSourcesCacheEntry and statusTelemetryOverviewCacheEntry are
// referenced by name in admin_handlers.go (via Invalidate) and nowhere
// else post-migration; no aliases required.
//
// statusRecentSourceLister is aliased in statusagg_bridge.go.

// Compile-time assertion that the shim matches the real interface.
var _ statusAggregateLogWatermarkReader = (*stubLogWatermark)(nil)

type stubLogWatermark struct{}

func (stubLogWatermark) LogEventsWatermark() (time.Time, error) { return time.Time{}, nil }
