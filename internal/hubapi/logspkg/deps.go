package logspkg

import (
	"time"

	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

// RecentSourceLister is satisfied by log stores that can efficiently list
// recent log sources within a time window. It mirrors the interface in
// statusaggpkg to avoid an import cycle.
type RecentSourceLister interface {
	ListSourcesSince(limit int, from time.Time) ([]logs.SourceSummary, error)
}

// Deps holds the dependencies for log handlers.
type Deps struct {
	LogStore   persistence.LogStore
	AssetStore persistence.AssetStore
	GroupStore persistence.GroupStore

	// ListRecentSourcesCached is an optional function injected from the statusagg
	// package so that the log sources endpoint can share the cached recent-sources
	// path with the status aggregate endpoint. If nil the handler falls back to
	// the all-time ListSources path.
	ListRecentSourcesCached func(recentLister RecentSourceLister, limit int, windowStart time.Time) ([]logs.SourceSummary, bool, error)
}
