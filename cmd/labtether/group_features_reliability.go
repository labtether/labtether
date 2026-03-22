package main

import "net/http"

// handleGroupReliabilityCollection forwards to groupfeatures.Deps.
func (s *apiServer) handleGroupReliabilityCollection(w http.ResponseWriter, r *http.Request) {
	s.ensureGroupFeaturesDeps().HandleGroupReliabilityCollection(w, r)
}

// handleGroupReliabilityByID forwards to groupfeatures.Deps.
func (s *apiServer) handleGroupReliabilityByID(w http.ResponseWriter, r *http.Request, groupID string) {
	s.ensureGroupFeaturesDeps().HandleGroupReliabilityByID(w, r, groupID)
}
