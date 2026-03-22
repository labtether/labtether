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
	// Delegate to the existing browser events WebSocket handler.
	// The existing handler handles WebSocket upgrade and event broadcasting.
	s.handleBrowserEvents(w, r)
}
