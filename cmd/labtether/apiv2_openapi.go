package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
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
	_, _ = w.Write([]byte(v2OpenAPISpec))
}

// v2OpenAPISpec joins the domain-owned path contracts into one static document.
// Each section preserves the documented endpoint order and response bytes.
const v2OpenAPISpec = v2OpenAPIDocumentHeader +
	v2OpenAPIAssetPaths +
	v2OpenAPIOperationPaths +
	v2OpenAPIContainerAndTransferPaths +
	v2OpenAPIAlertIncidentPaths +
	v2OpenAPIConnectorPaths +
	v2OpenAPIAccessAdministrationPaths +
	v2OpenAPIServiceNotificationPaths +
	v2OpenAPITopologyPaths +
	v2OpenAPIOperatorSettingsPaths
