package main

import "github.com/labtether/labtether/internal/hubapi/shared"

// buildSearchDeps constructs the shared.SearchDeps from the apiServer's fields.
func (s *apiServer) buildSearchDeps() *shared.SearchDeps {
	return &shared.SearchDeps{
		AssetStore: s.assetStore,
		GroupStore: s.groupStore,
	}
}

// ensureSearchDeps returns the search deps, creating and caching on first call.
func (s *apiServer) ensureSearchDeps() *shared.SearchDeps {
	if s.searchDeps != nil {
		return s.searchDeps
	}
	d := s.buildSearchDeps()
	s.searchDeps = d
	return d
}
