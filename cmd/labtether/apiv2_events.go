package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2EventStream(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "events:subscribe") {
		apiv2.WriteScopeForbidden(w, "events:subscribe")
		return
	}
	// withAuth already accepted this caller (session, API key or owner token),
	// so stream without asking for a second credential.
	s.ensureBrowserEventsDeps().ServeAuthenticatedBrowserEvents(w, r)
}
