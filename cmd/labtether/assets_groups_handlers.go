package main

import "net/http"

// handleAssets and handleAssetActions have been extracted to
// internal/hubapi/resources/assets_groups.go.
// These thin stubs delegate to the resources package via the cached Deps.

func (s *apiServer) handleAssets(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleAssets(w, r)
}

func (s *apiServer) handleAssetActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleAssetActions(w, r)
}
