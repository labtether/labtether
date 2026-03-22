// cmd/labtether/apiv2_sysinfo.go
package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
)

// handleV2AssetNetwork handles GET /api/v2/assets/{id}/network
func (s *apiServer) handleV2AssetNetwork(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "network:read") {
		apiv2.WriteScopeForbidden(w, "network:read")
		return
	}
	r.URL.Path = "/network/" + assetID
	apiv2.WrapV1Handler(s.handleNetworks)(w, r)
}

// handleV2AssetDisks handles GET /api/v2/assets/{id}/disks
func (s *apiServer) handleV2AssetDisks(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "disks:read") {
		apiv2.WriteScopeForbidden(w, "disks:read")
		return
	}
	r.URL.Path = "/disks/" + assetID
	apiv2.WrapV1Handler(s.handleDisks)(w, r)
}

// handleV2AssetPackages handles GET/POST /api/v2/assets/{id}/packages[/subPath]
func (s *apiServer) handleV2AssetPackages(w http.ResponseWriter, r *http.Request, assetID, subPath string) {
	scope := "packages:read"
	if r.Method == http.MethodPost {
		scope = "packages:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	subPath = strings.TrimPrefix(subPath, "/")
	switch subPath {
	case "", "upgradable":
		r.URL.Path = "/packages/" + assetID
		if subPath == "upgradable" {
			r.URL.Path = "/packages/" + assetID + "/upgradable"
		}
	case "install", "update":
		r.URL.Path = "/packages/" + assetID + "/" + subPath
	default:
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "unknown packages sub-path")
		return
	}
	apiv2.WrapV1Handler(s.handlePackages)(w, r)
}

// handleV2AssetCron handles GET /api/v2/assets/{id}/cron
func (s *apiServer) handleV2AssetCron(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "cron:read") {
		apiv2.WriteScopeForbidden(w, "cron:read")
		return
	}
	r.URL.Path = "/cron/" + assetID
	apiv2.WrapV1Handler(s.handleCrons)(w, r)
}

// handleV2AssetUsers handles GET /api/v2/assets/{id}/users
func (s *apiServer) handleV2AssetUsers(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "users:read") {
		apiv2.WriteScopeForbidden(w, "users:read")
		return
	}
	r.URL.Path = "/users/" + assetID
	apiv2.WrapV1Handler(s.handleUsers)(w, r)
}

// handleV2AssetLogs handles GET /api/v2/assets/{id}/logs
func (s *apiServer) handleV2AssetLogs(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "logs:read") {
		apiv2.WriteScopeForbidden(w, "logs:read")
		return
	}
	r.URL.Path = "/logs/journal/" + assetID
	apiv2.WrapV1Handler(s.handleJournalLogs)(w, r)
}

// handleV2AssetMetrics handles GET /api/v2/assets/{id}/metrics
func (s *apiServer) handleV2AssetMetrics(w http.ResponseWriter, r *http.Request, assetID string) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "metrics:read") {
		apiv2.WriteScopeForbidden(w, "metrics:read")
		return
	}
	r.URL.Path = "/metrics/assets/" + assetID
	apiv2.WrapV1Handler(s.handleAssetMetrics)(w, r)
}

// handleV2AssetMetricsLatest handles GET /api/v2/assets/{id}/metrics/latest.
// It returns the single latest snapshot for one asset.
func (s *apiServer) handleV2AssetMetricsLatest(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "metrics:read") {
		apiv2.WriteScopeForbidden(w, "metrics:read")
		return
	}
	if s.telemetryStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "telemetry store unavailable")
		return
	}

	now := time.Now().UTC()

	// Prefer the richer dynamic snapshot when the store supports it.
	if dynStore, ok := s.telemetryStore.(persistence.TelemetryDynamicStore); ok {
		dyn, err := dynStore.DynamicSnapshotForAsset(assetID, now)
		if err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to query telemetry")
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]any{
			"asset_id":     assetID,
			"collected_at": now,
			"metrics":      dyn.Metrics,
			"snapshot":     dyn.ToLegacySnapshot(),
		})
		return
	}

	snapshot, err := s.telemetryStore.Snapshot(assetID, now)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to query telemetry")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"asset_id":     assetID,
		"collected_at": now,
		"snapshot":     snapshot,
	})
}

// handleV2AssetWake handles POST /api/v2/assets/{id}/wake
func (s *apiServer) handleV2AssetWake(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:power") {
		apiv2.WriteScopeForbidden(w, "assets:power")
		return
	}
	apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) { s.handleWakeOnLAN(w, r, assetID) })(w, r)
}

// handleV2AssetReboot handles POST /api/v2/assets/{id}/reboot
func (s *apiServer) handleV2AssetReboot(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:power") {
		apiv2.WriteScopeForbidden(w, "assets:power")
		return
	}
	result := s.v2ExecOnAsset(r, assetID, "reboot", 10)
	if result.Error != "" {
		apiv2.WriteError(w, http.StatusConflict, result.Error, result.Message)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "rebooting", "asset_id": assetID})
}

// handleV2AssetShutdown handles POST /api/v2/assets/{id}/shutdown
func (s *apiServer) handleV2AssetShutdown(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "assets:power") {
		apiv2.WriteScopeForbidden(w, "assets:power")
		return
	}
	result := s.v2ExecOnAsset(r, assetID, "shutdown -h now", 10)
	if result.Error != "" {
		apiv2.WriteError(w, http.StatusConflict, result.Error, result.Message)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "shutting_down", "asset_id": assetID})
}
