// cmd/labtether/apiv2_resources.go
package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

// handleV2AssetFiles routes /api/v2/assets/{id}/files[/action]
func (s *apiServer) handleV2AssetFiles(w http.ResponseWriter, r *http.Request, assetID, subPath string) {
	// Determine required scope based on method.
	requiredScope := "files:read"
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		requiredScope = "files:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), requiredScope) {
		apiv2.WriteScopeForbidden(w, requiredScope)
		return
	}

	subPath = strings.TrimPrefix(subPath, "/")

	// Route to existing file handlers. These use the v1 response format,
	// but the underlying logic is correct. In a future iteration these can
	// be updated to use the v2 envelope.
	switch {
	case subPath == "" && r.Method == http.MethodGet:
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileList(w, r, assetID) })(w, r)
	case subPath == "read":
		// File download is binary/streaming — bypass WrapV1Handler to avoid
		// buffering the entire response in memory (can be up to 512 MiB).
		s.handleFileDownload(w, r, assetID)
	case subPath == "write":
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileUpload(w, r, assetID) })(w, r)
	case subPath == "mkdir":
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileMkdir(w, r, assetID) })(w, r)
	case subPath == "rename":
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileRename(w, r, assetID) })(w, r)
	case subPath == "copy":
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileCopy(w, r, assetID) })(w, r)
	case subPath == "" && r.Method == http.MethodDelete:
		apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleFileDelete(w, r, assetID) })(w, r)
	default:
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "unknown files sub-path: "+subPath)
	}
}

// handleV2AssetProcesses routes GET /api/v2/assets/{id}/processes
func (s *apiServer) handleV2AssetProcesses(w http.ResponseWriter, r *http.Request, assetID string) {
	if apiv2.IsMutatingMethod(r.Method) {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "processes:read") {
		apiv2.WriteScopeForbidden(w, "processes:read")
		return
	}
	// Delegate to existing process list handler via the resources bridge.
	// Rewrite the URL path so the existing handler can parse the asset ID.
	r.URL.Path = "/processes/" + assetID
	apiv2.WrapV1Handler(s.handleProcesses)(w, r)
}

// handleV2AssetProcessActions routes POST /api/v2/assets/{id}/processes/kill
func (s *apiServer) handleV2AssetProcessActions(w http.ResponseWriter, r *http.Request, assetID, subPath string) {
	if subPath == "kill" {
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "processes:kill") {
			apiv2.WriteScopeForbidden(w, "processes:kill")
			return
		}
		r.URL.Path = "/processes/" + assetID + "/kill"
		apiv2.WrapV1Handler(s.handleProcesses)(w, r)
		return
	}
	apiv2.WriteError(w, http.StatusNotFound, "not_found", "unknown process action: "+subPath)
}

// handleV2AssetServices routes GET /api/v2/assets/{id}/services
func (s *apiServer) handleV2AssetServices(w http.ResponseWriter, r *http.Request, assetID string) {
	if apiv2.IsMutatingMethod(r.Method) {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "services:read") {
		apiv2.WriteScopeForbidden(w, "services:read")
		return
	}
	r.URL.Path = "/services/" + assetID
	apiv2.WrapV1Handler(s.handleServices)(w, r)
}

// handleV2AssetServiceActions routes POST /api/v2/assets/{id}/services/{name}/{action}
func (s *apiServer) handleV2AssetServiceActions(w http.ResponseWriter, r *http.Request, assetID, subPath string) {
	// subPath is "{name}/{action}" e.g. "nginx/restart"
	parts := strings.SplitN(subPath, "/", 2)
	if len(parts) != 2 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "expected /services/{name}/{action}")
		return
	}
	action := parts[1]
	switch action {
	case "start", "stop", "restart":
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "services:write") {
			apiv2.WriteScopeForbidden(w, "services:write")
			return
		}
	default:
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid action: "+action)
		return
	}
	r.URL.Path = "/services/" + assetID + "/" + subPath
	apiv2.WrapV1Handler(s.handleServices)(w, r)
}
