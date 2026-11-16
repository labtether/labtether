package main

import (
	"time"

	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// status_aggregate_collectors.go — thin stub.
//
// All implementation has moved to internal/hubapi/statusagg/.

// statusLogSourcesCacheEntry and statusTelemetryOverviewCacheEntry are
// referenced by name in admin_handlers.go (via Invalidate) and nowhere
// else post-migration; no aliases required.
//
// statusRecentSourceLister is aliased in statusagg_bridge.go.

// Compile-time assertion that the shim matches the real interface.
var _ statusaggpkg.LogWatermarkReader = (*stubLogWatermark)(nil)

type stubLogWatermark struct{}

func (stubLogWatermark) LogEventsWatermark() (time.Time, error) { return time.Time{}, nil }
