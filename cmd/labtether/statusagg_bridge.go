package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
	"github.com/labtether/labtether/internal/logs"
)

// --- Type aliases for test compatibility ---

// statusDeadLetterSnapshot is an alias for the canonical type in statusagg.
type statusDeadLetterSnapshot = statusaggpkg.DeadLetterSnapshot

// buildStatusAggDeps constructs the statusagg.Deps from the apiServer's fields,
// without worker-lifecycle references. Used by live/read-only paths.
func (s *apiServer) buildStatusAggDeps() *statusaggpkg.Deps {
	return &statusaggpkg.Deps{
		AssetStore:            s.assetStore,
		GroupStore:            s.groupStore,
		TelemetryStore:        s.telemetryStore,
		LogStore:              s.logStore,
		ActionStore:           s.actionStore,
		UpdateStore:           s.updateStore,
		CanonicalStore:        s.canonicalStore,
		RuntimeStore:          s.runtimeStore,
		AuditStore:            s.auditStore,
		TerminalStore:         s.terminalStore,
		ConnectorRegistry:     s.connectorRegistry,
		WebServiceCoordinator: s.webServiceCoordinator,
		DemoMode:              s.demoMode,
		Cache:                 &s.statusCache,
	}
}

// buildStatusAggDepsWithWorker constructs the statusagg.Deps that includes the
// worker-lifecycle references (retention tracker, processed-jobs counter).
func (s *apiServer) buildStatusAggDepsWithWorker(
	retentionTracker *retentionState,
	processed *atomic.Uint64,
) *statusaggpkg.Deps {
	d := s.buildStatusAggDeps()
	d.RetentionTracker = retentionTracker
	d.ProcessedJobs = processed
	return d
}

// --- HTTP handler forwarding methods ---

// handleStatusAggregate returns the http.HandlerFunc for GET /status/aggregate.
// The retentionTracker and processed counter are closure params threaded from
// the worker bootstrap and wired once at server startup.
func (s *apiServer) handleStatusAggregate(
	retentionTracker *retentionState,
	processed *atomic.Uint64,
) http.HandlerFunc {
	return s.buildStatusAggDepsWithWorker(retentionTracker, processed).HandleStatusAggregate()
}

func (s *apiServer) handleStatusAggregateLive() http.HandlerFunc {
	return s.buildStatusAggDeps().HandleStatusAggregateLive()
}

// --- Response builder forwarding methods ---
// These keep callers in cmd/labtether/ (e.g. MCP bridge, SSE broadcaster)
// compiling without changes.

func (s *apiServer) buildStatusAggregateLiveResponse(ctx context.Context, gf string) statusaggpkg.LiveResponse {
	return s.buildStatusAggDeps().BuildLiveResponse(ctx, gf)
}

func (s *apiServer) buildStatusAggregateResponse(ctx context.Context, gf string) statusaggpkg.Response {
	return s.buildStatusAggDeps().BuildResponse(ctx, gf)
}

// --- Log source cache forwarding ---

// statusRecentSourceLister is a type alias so log_handlers.go continues to
// compile with the interface name it already uses.
type statusRecentSourceLister = statusaggpkg.RecentSourceLister

// statusListRecentSourcesCached forwards the cached log-source listing to the
// statusagg package. log_handlers.go uses this to share the cache with the
// status aggregate endpoint.
func (s *apiServer) statusListRecentSourcesCached(
	recentLister statusRecentSourceLister,
	limit int,
	windowStart time.Time,
) ([]logs.SourceSummary, bool, error) {
	return s.buildStatusAggDeps().StatusListRecentSourcesCached(recentLister, limit, windowStart)
}

// --- Endpoint probe forwarding methods used by tests ---

func (s *apiServer) statusRoutingBaseURLs() (statusResolvedRoutingURL, statusResolvedRoutingURL) {
	return s.buildStatusAggDeps().RoutingBaseURLs()
}

func (s *apiServer) statusProbeEndpointsCached(ctx context.Context, targets []statusEndpointTarget) []statusEndpointResult {
	// Use the local stub variable so that test overrides of statusProbeEndpointFunc
	// take effect. Mirror the override to the canonical package variable, then
	// delegate to the package implementation.
	statusaggpkg.ProbeEndpointFunc = statusProbeEndpointFunc
	return s.buildStatusAggDeps().ProbeEndpointsCached(ctx, targets)
}

// --- Forwarding methods used by tests ---
// These exist because tests in cmd/labtether/ call private apiServer methods
// directly. The forwarding methods delegate to the statusagg.Deps to keep
// the implementation centralised.

func (s *apiServer) statusLoadDeadLetters() statusDeadLetterSnapshot {
	return s.buildStatusAggDeps().LoadDeadLetters()
}

func (s *apiServer) statusListLogSources(
	groupFilter string,
	assetGroup map[string]string,
	caller string,
) []logs.SourceSummary {
	return s.buildStatusAggDeps().ListLogSources(groupFilter, assetGroup, caller)
}

// --- Cache invalidation ---

// invalidateStatusCaches delegates to StatusCache.Invalidate so the admin
// reset handler keeps a single, clear call site.
func (s *apiServer) invalidateStatusCaches() {
	s.statusCache.Invalidate()
}
