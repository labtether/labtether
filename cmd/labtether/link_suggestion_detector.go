package main

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin stubs delegating to internal/hubapi/shared so that callers inside
// cmd/labtether/ keep compiling without a mass rename.

// detectLinkSuggestions runs the discovery engine over all known assets and
// creates or suggests edges in the edge store.
func (s *apiServer) detectLinkSuggestions() error {
	d := &shared.LinkSuggestionDeps{
		AssetStore: s.assetStore,
		EdgeStore:  s.edgeStore,
	}
	return d.DetectLinkSuggestions()
}

// normalizeMACAddress normalizes a MAC address to lowercase colon-separated form.
func normalizeMACAddress(raw string) string {
	return shared.NormalizeMACAddress(raw)
}
