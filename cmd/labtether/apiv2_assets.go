package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
)

// handleV2Assets handles GET /api/v2/assets (list) and POST /api/v2/assets (create).
func (s *apiServer) handleV2Assets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:read") {
			apiv2.WriteScopeForbidden(w, "assets:read")
			return
		}
		s.v2ListAssets(w, r)
	case http.MethodPost:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:write") {
			apiv2.WriteScopeForbidden(w, "assets:write")
			return
		}
		s.v2CreateAsset(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// handleV2AssetActions handles /api/v2/assets/{id} for GET/PUT/DELETE,
// and routes sub-paths like {id}/exec, {id}/files, {id}/processes, {id}/services.
func (s *apiServer) handleV2AssetActions(w http.ResponseWriter, r *http.Request) {
	assetID := strings.TrimPrefix(r.URL.Path, "/api/v2/assets/")
	if assetID == "" || assetID == r.URL.Path {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "asset id required")
		return
	}

	// Route sub-paths.
	if idx := strings.Index(assetID, "/"); idx != -1 {
		subPath := assetID[idx+1:]
		assetID = assetID[:idx]

		// Check asset allowlist with the real asset ID.
		if !apiv2.AssetCheck(allowedAssetsFromContext(r.Context()), assetID) {
			apiv2.WriteAssetForbidden(w, assetID)
			return
		}

		s.routeV2AssetSubPath(w, r, assetID, subPath)
		return
	}

	// Check asset allowlist.
	if !apiv2.AssetCheck(allowedAssetsFromContext(r.Context()), assetID) {
		apiv2.WriteAssetForbidden(w, assetID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:read") {
			apiv2.WriteScopeForbidden(w, "assets:read")
			return
		}
		s.v2GetAsset(w, r, assetID)
	case http.MethodPut, http.MethodPatch:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:write") {
			apiv2.WriteScopeForbidden(w, "assets:write")
			return
		}
		s.v2UpdateAsset(w, r, assetID)
	case http.MethodDelete:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:write") {
			apiv2.WriteScopeForbidden(w, "assets:write")
			return
		}
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "api.asset.deleted",
			ActorID:   principalActorID(r.Context()),
			Target:    assetID,
			Details:   map[string]any{"asset_id": assetID},
			Timestamp: time.Now().UTC(),
		}, "v2 asset delete on "+assetID)
		s.v2DeleteAsset(w, r, assetID)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// routeV2AssetSubPath dispatches asset sub-path requests to the appropriate handler.
// This routing function must stay in cmd/labtether/ because it dispatches across many packages.
func (s *apiServer) routeV2AssetSubPath(w http.ResponseWriter, r *http.Request, assetID, subPath string) {
	switch {
	case subPath == "exec":
		s.handleV2AssetExec(w, r, assetID)
	case strings.HasPrefix(subPath, "files"):
		s.handleV2AssetFiles(w, r, assetID, strings.TrimPrefix(subPath, "files"))
	case subPath == "processes":
		s.handleV2AssetProcesses(w, r, assetID)
	case strings.HasPrefix(subPath, "processes/"):
		s.handleV2AssetProcessActions(w, r, assetID, strings.TrimPrefix(subPath, "processes/"))
	case subPath == "services":
		s.handleV2AssetServices(w, r, assetID)
	case strings.HasPrefix(subPath, "services/"):
		s.handleV2AssetServiceActions(w, r, assetID, strings.TrimPrefix(subPath, "services/"))
	case subPath == "network":
		s.handleV2AssetNetwork(w, r, assetID)
	case subPath == "disks":
		s.handleV2AssetDisks(w, r, assetID)
	case strings.HasPrefix(subPath, "packages"):
		s.handleV2AssetPackages(w, r, assetID, strings.TrimPrefix(subPath, "packages"))
	case subPath == "cron":
		s.handleV2AssetCron(w, r, assetID)
	case subPath == "users":
		s.handleV2AssetUsers(w, r, assetID)
	case subPath == "logs":
		s.handleV2AssetLogs(w, r, assetID)
	case subPath == "metrics":
		s.handleV2AssetMetrics(w, r, assetID)
	case subPath == "metrics/latest":
		s.handleV2AssetMetricsLatest(w, r, assetID)
	case subPath == "wake":
		s.handleV2AssetWake(w, r, assetID)
	case subPath == "reboot":
		s.handleV2AssetReboot(w, r, assetID)
	case subPath == "shutdown":
		s.handleV2AssetShutdown(w, r, assetID)
	default:
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "unknown sub-path: "+subPath)
	}
}

// Thin stubs — behaviour lives in internal/hubapi/resources.

func (s *apiServer) v2ListAssets(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().V2ListAssets(w, r)
}

func (s *apiServer) v2GetAsset(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().V2GetAsset(w, r, assetID)
}

func (s *apiServer) v2CreateAsset(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().V2CreateAsset(w, r)
}

func (s *apiServer) v2UpdateAsset(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().V2UpdateAsset(w, r, assetID)
}

func (s *apiServer) v2DeleteAsset(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().V2DeleteAsset(w, r, assetID)
}
