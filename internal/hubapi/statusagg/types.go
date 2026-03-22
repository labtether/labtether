package statusagg

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/runtimesettings"
	"golang.org/x/sync/singleflight"
)

// --- Watermark reader interfaces ---

// TelemetryWatermarkReader is satisfied by stores that expose a telemetry
// high-watermark for cache invalidation.
type TelemetryWatermarkReader interface {
	TelemetryWatermark() (time.Time, error)
}

// LogWatermarkReader is satisfied by stores that expose a log-events
// high-watermark for cache invalidation.
type LogWatermarkReader interface {
	LogEventsWatermark() (time.Time, error)
}

// ActionWatermarkReader is satisfied by stores that expose an action-runs
// high-watermark for cache invalidation.
type ActionWatermarkReader interface {
	ActionRunsWatermark() (time.Time, error)
}

// UpdateWatermarkReader is satisfied by stores that expose update-runs and
// update-plans high-watermarks for cache invalidation.
type UpdateWatermarkReader interface {
	UpdateRunsWatermark() (time.Time, error)
	UpdatePlansWatermark() (time.Time, error)
}

// CanonicalWatermarkReader is satisfied by stores that expose a canonical
// status high-watermark for cache invalidation.
type CanonicalWatermarkReader interface {
	CanonicalStatusWatermark() (time.Time, error)
}

// --- Log source listing ---

// RecentSourceLister is satisfied by log stores that can efficiently list
// recent log sources within a time window.
type RecentSourceLister interface {
	ListSourcesSince(limit int, from time.Time) ([]logs.SourceSummary, error)
}

// --- Endpoint types ---

// EndpointResult is the per-endpoint health probe result.
type EndpointResult struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Status    string `json:"status"`
	Code      int    `json:"code,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

// EndpointTarget is a probe target (name + URL pair).
type EndpointTarget struct {
	Name string
	URL  string
}

// endpointTarget is the unexported alias used internally within this package.
// Exported as EndpointTarget for test helpers in cmd/labtether.
type endpointTarget = EndpointTarget

// --- Cache entry types ---

// ETagCacheEntry caches the computed ETag for a given fingerprint.
type ETagCacheEntry struct {
	Fingerprint string
	ETag        string
}

// RoutingBaseURLCacheEntry caches the resolved routing base URLs.
type RoutingBaseURLCacheEntry struct {
	ExpiresAt    time.Time
	APIBaseURL   ResolvedRoutingURL
	AgentBaseURL ResolvedRoutingURL
}

// ResolvedRoutingURL holds a resolved URL and the settings source it came from.
type ResolvedRoutingURL struct {
	URL    string
	Source runtimesettings.Source
}

// EndpointProbeCacheEntry caches the results of endpoint health probes.
type EndpointProbeCacheEntry struct {
	TargetsFingerprint string
	ExpiresAt          time.Time
	Results            []EndpointResult
}

// DeadLetterCacheEntry caches a dead-letter event snapshot.
type DeadLetterCacheEntry struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Watermark   time.Time
	Snapshot    DeadLetterSnapshot
}

// DeadLetterSnapshot holds the full dead-letter result set and analytics.
type DeadLetterSnapshot struct {
	Events    []shared.DeadLetterEventResponse
	Total     int
	Analytics shared.DeadLetterAnalyticsResponse
}

// TelemetryOverviewCacheEntry caches the per-asset telemetry overview.
type TelemetryOverviewCacheEntry struct {
	AssetFingerprint string
	ExpiresAt        time.Time
	Overview         []shared.AssetTelemetryOverview
}

// LogSourcesCacheEntry caches the recent log sources list.
type LogSourcesCacheEntry struct {
	Limit       int
	WindowStart time.Time
	Watermark   time.Time
	Sources     []logs.SourceSummary
}

// CanonicalPayloadCacheEntry caches a canonical status payload.
// CanonicalPayload is defined in payloads.go alongside the other response types.
type CanonicalPayloadCacheEntry struct {
	Key     string
	Payload CanonicalPayload
}

// --- StatusCache ---

// StatusCache bundles all status-aggregate caching state. It was previously
// defined in cmd/labtether/server_types.go and is now the canonical home.
type StatusCache struct {
	Generation atomic.Uint64

	CanonicalCacheMu  sync.RWMutex
	CanonicalCache    [4]CanonicalPayloadCacheEntry
	CanonicalCacheIdx int

	LiveBuildGroup singleflight.Group
	FullBuildGroup singleflight.Group

	RoutingBaseURLCacheMu sync.RWMutex
	RoutingBaseURLCache   RoutingBaseURLCacheEntry

	EndpointProbeCacheMu sync.RWMutex
	EndpointProbeCache   EndpointProbeCacheEntry
	EndpointProbeGroup   singleflight.Group

	DeadLetterCacheMu sync.RWMutex
	DeadLetterCache   DeadLetterCacheEntry

	TelemetryOverviewCacheMu sync.RWMutex
	TelemetryOverviewCache   TelemetryOverviewCacheEntry

	LogSourcesCacheMu    sync.RWMutex
	LogSourcesCache      LogSourcesCacheEntry
	LogSourcesQueryGroup singleflight.Group
}

// Invalidate zeroes all TTL-based status caches so the next request rebuilds
// them from the database. Called from the admin reset handler.
func (sc *StatusCache) Invalidate() {
	sc.CanonicalCacheMu.Lock()
	sc.CanonicalCache = [4]CanonicalPayloadCacheEntry{}
	sc.CanonicalCacheIdx = 0
	sc.CanonicalCacheMu.Unlock()

	sc.RoutingBaseURLCacheMu.Lock()
	sc.RoutingBaseURLCache = RoutingBaseURLCacheEntry{}
	sc.RoutingBaseURLCacheMu.Unlock()

	sc.EndpointProbeCacheMu.Lock()
	sc.EndpointProbeCache = EndpointProbeCacheEntry{}
	sc.EndpointProbeCacheMu.Unlock()

	sc.DeadLetterCacheMu.Lock()
	sc.DeadLetterCache = DeadLetterCacheEntry{}
	sc.DeadLetterCacheMu.Unlock()

	sc.TelemetryOverviewCacheMu.Lock()
	sc.TelemetryOverviewCache = TelemetryOverviewCacheEntry{}
	sc.TelemetryOverviewCacheMu.Unlock()

	sc.LogSourcesCacheMu.Lock()
	sc.LogSourcesCache = LogSourcesCacheEntry{}
	sc.LogSourcesCacheMu.Unlock()
}
