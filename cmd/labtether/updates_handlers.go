package main

import (
	"net/http"
)

// handleUpdatePlans forwards to updatespkg.Deps.
func (s *apiServer) handleUpdatePlans(w http.ResponseWriter, r *http.Request) {
	s.ensureUpdatesDeps().HandleUpdatePlans(w, r)
}

// handleUpdatePlanActions forwards to updatespkg.Deps.
func (s *apiServer) handleUpdatePlanActions(w http.ResponseWriter, r *http.Request) {
	s.ensureUpdatesDeps().HandleUpdatePlanActions(w, r)
}

// handleUpdateRuns forwards to updatespkg.Deps.
func (s *apiServer) handleUpdateRuns(w http.ResponseWriter, r *http.Request) {
	s.ensureUpdatesDeps().HandleUpdateRuns(w, r)
}

// handleUpdateRunActions forwards to updatespkg.Deps.
func (s *apiServer) handleUpdateRunActions(w http.ResponseWriter, r *http.Request) {
	s.ensureUpdatesDeps().HandleUpdateRunActions(w, r)
}
