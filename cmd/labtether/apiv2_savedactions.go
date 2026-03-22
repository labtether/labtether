package main

import "net/http"

// handleV2SavedActions handles GET /api/v2/actions and POST /api/v2/actions.
func (s *apiServer) handleV2SavedActions(w http.ResponseWriter, r *http.Request) {
	s.ensureActionsDeps().HandleV2SavedActions(w, r)
}

// handleV2SavedActionActions handles /api/v2/actions/{id} for GET/DELETE and
// /api/v2/actions/{id}/run for POST.
func (s *apiServer) handleV2SavedActionActions(w http.ResponseWriter, r *http.Request) {
	s.ensureActionsDeps().HandleV2SavedActionActions(w, r)
}
