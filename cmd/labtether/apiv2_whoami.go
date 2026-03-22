package main

import "net/http"

// handleV2Whoami handles GET /api/v2/whoami.
func (s *apiServer) handleV2Whoami(w http.ResponseWriter, r *http.Request) {
	s.ensureWhoamiDeps().HandleV2Whoami(w, r)
}
