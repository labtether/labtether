package main

import (
	"net/http"
)

// handleActionExecute forwards to actionspkg.Deps.
func (s *apiServer) handleActionExecute(w http.ResponseWriter, r *http.Request) {
	s.ensureActionRunDeps().HandleActionExecute(w, r)
}

// handleActionRuns forwards to actionspkg.Deps.
func (s *apiServer) handleActionRuns(w http.ResponseWriter, r *http.Request) {
	s.ensureActionRunDeps().HandleActionRuns(w, r)
}

// handleActionRunActions forwards to actionspkg.Deps.
func (s *apiServer) handleActionRunActions(w http.ResponseWriter, r *http.Request) {
	s.ensureActionRunDeps().HandleActionRunActions(w, r)
}
