package main

import (
	"time"

	"github.com/labtether/labtether/internal/logs"
	logspkg "github.com/labtether/labtether/internal/hubapi/logspkg"
	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// buildLogsDeps constructs the logspkg.Deps from the apiServer's fields.
func (s *apiServer) buildLogsDeps() *logspkg.Deps {
	saggDeps := s.buildStatusAggDeps()
	return &logspkg.Deps{
		LogStore:   s.logStore,
		AssetStore: s.assetStore,
		GroupStore: s.groupStore,
		// Wrap the statusagg cache function to bridge the two interface types.
		// Both declare the identical method set; the wrapper avoids a direct
		// assignment that Go's type system rejects for distinct named interfaces.
		ListRecentSourcesCached: func(rl logspkg.RecentSourceLister, limit int, windowStart time.Time) ([]logs.SourceSummary, bool, error) {
			return saggDeps.StatusListRecentSourcesCached(rl, limit, windowStart)
		},
	}
}

// ensureLogsDeps returns the logs deps, creating and caching on first call.
func (s *apiServer) ensureLogsDeps() *logspkg.Deps {
	if s.logsDeps != nil {
		return s.logsDeps
	}
	d := s.buildLogsDeps()
	s.logsDeps = d
	return d
}

// Compile-time check: logspkg.RecentSourceLister must be satisfied by
// statusaggpkg.RecentSourceLister (both mirror the same underlying interface).
var _ logspkg.RecentSourceLister = (statusaggpkg.RecentSourceLister)(nil)
