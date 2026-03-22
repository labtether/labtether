package main

import "net/http"

func (s *apiServer) handleV2Search(w http.ResponseWriter, r *http.Request) {
	s.ensureSearchDeps().HandleV2Search(w, r)
}
