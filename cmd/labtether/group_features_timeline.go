package main

import "net/http"

// handleGroupTimeline forwards to groupfeatures.Deps.
func (s *apiServer) handleGroupTimeline(w http.ResponseWriter, r *http.Request, groupID string) {
	s.ensureGroupFeaturesDeps().HandleGroupTimeline(w, r, groupID)
}
