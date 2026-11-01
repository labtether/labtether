package main

import (
	"log"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const hubSSHKeySettingAuditType = "asset.hub_ssh_key.updated"

type hubSSHKeySettingRequest struct {
	Enabled *bool `json:"enabled"`
}

type hubSSHKeySettingResponse struct {
	AssetID        string `json:"asset_id"`
	Enabled        bool   `json:"enabled"`
	RemovalPending bool   `json:"removal_pending"`
	AgentConnected bool   `json:"agent_connected"`
	InstallSent    bool   `json:"install_sent"`
	RemovalSent    bool   `json:"removal_sent"`
}

// handleAssetHubSSHKey serves GET and PUT /assets/{id}/hub-ssh-key, and the v2
// route through handleV2AssetHubSSHKey. Callers have already checked that the
// device exists and that the caller may see it.
func (s *apiServer) handleAssetHubSSHKey(w http.ResponseWriter, r *http.Request, asset assets.Asset) {
	if s.hubSSHKeyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH key settings are unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !apiv2.RequireScope(w, r, "assets:read") {
			return
		}
		setting, ok := s.loadHubSSHKeySetting(asset.ID)
		if !ok {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load hub SSH key setting")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, s.hubSSHKeySettingPayload(setting))
	case http.MethodPut:
		s.updateAssetHubSSHKey(w, r, asset)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// updateAssetHubSSHKey turns the Hub SSH key on or off for one device. Turning
// it on lets the Hub log in as root, so only admins and owners may change it,
// and API keys also need the hub:admin scope.
func (s *apiServer) updateAssetHubSSHKey(w http.ResponseWriter, r *http.Request, asset assets.Asset) {
	if !apiv2.RequireScope(w, r, "hub:admin") {
		return
	}
	if !auth.HasAdminPrivileges(userRoleFromContext(r.Context())) {
		servicehttp.WriteError(w, http.StatusForbidden, "only an admin or owner can change the Hub SSH key")
		return
	}
	if !s.enforceRateLimit(w, r, "assets.hub_ssh_key", 60, time.Minute) {
		return
	}
	var req hubSSHKeySettingRequest
	if err := decodeJSONBody(w, r, &req); err != nil || req.Enabled == nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "enabled must be true or false")
		return
	}
	if *req.Enabled && s.currentHubSSHIdentity() == nil {
		if _, err := s.ensureCurrentHubSSHIdentity(); err != nil {
			log.Printf("hub-ssh-key: hub SSH identity unavailable: %v", err)
		}
	}
	change, err := s.applyHubSSHKeySetting(asset, *req.Enabled)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save hub SSH key setting")
		return
	}
	s.auditHubSSHKeySetting(r, asset.ID, change)

	payload := s.hubSSHKeySettingPayload(change.Setting)
	payload.InstallSent = change.InstallSent
	payload.RemovalSent = change.RemovalSent
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

func (s *apiServer) hubSSHKeySettingPayload(setting persistence.HubSSHKeySetting) hubSSHKeySettingResponse {
	_, connected := s.connectedAgent(setting.AssetID)
	return hubSSHKeySettingResponse{
		AssetID:        setting.AssetID,
		Enabled:        setting.Enabled,
		RemovalPending: setting.RemovalPending,
		AgentConnected: connected,
	}
}

func (s *apiServer) auditHubSSHKeySetting(r *http.Request, assetID string, change hubSSHKeyChange) {
	event := audit.NewEvent(hubSSHKeySettingAuditType)
	event.ActorID = principalActorID(r.Context())
	event.Target = assetID
	event.Decision = "applied"
	event.Details = map[string]any{
		"enabled":          change.Setting.Enabled,
		"previous_enabled": change.Previous.Enabled,
		"removal_pending":  change.Setting.RemovalPending,
		"install_sent":     change.InstallSent,
		"removal_sent":     change.RemovalSent,
	}
	s.appendAuditEventBestEffort(event, "api warning: failed to append hub SSH key setting audit event")
}

// handleV2AssetHubSSHKey serves /api/v2/assets/{id}/hub-ssh-key with the v1
// handler in the v2 envelope. handleV2AssetActions checks the asset allowlist.
func (s *apiServer) handleV2AssetHubSSHKey(w http.ResponseWriter, r *http.Request, assetID string) {
	if s.assetStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "asset store unavailable")
		return
	}
	asset, ok, err := s.assetStore.GetAsset(assetID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to load asset")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "asset_not_found", "no asset with id: "+assetID)
		return
	}
	apiv2.WrapV1Handler(func(w http.ResponseWriter, r *http.Request) {
		s.handleAssetHubSSHKey(w, r, asset)
	})(w, r)
}
