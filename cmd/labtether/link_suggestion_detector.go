package main

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// detectLinkSuggestions runs the discovery engine over all known assets and
// creates or suggests edges in the edge store.
func (s *apiServer) detectLinkSuggestions() error {
	d := &shared.LinkSuggestionDeps{
		AssetStore: s.assetStore,
		EdgeStore:  s.edgeStore,
	}
	return d.DetectLinkSuggestions()
}
