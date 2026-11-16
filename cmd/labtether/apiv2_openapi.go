package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/apiv2/openapi"
)

// handleV2OpenAPI serves a static minimal OpenAPI 3.0 specification that
// documents the v2 endpoints. No authentication is required so that tools
// (curl, Swagger UI, code generators) can fetch the spec without credentials.
func (s *apiServer) handleV2OpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openapi.Document))
}
