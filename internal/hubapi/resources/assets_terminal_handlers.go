package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Validation constants matching cmd/labtether/main.go.
const (
	maxTargetLength  = 255
	maxActorIDLength = 64
	maxHostKeyLength = 2048
)

func (d *Deps) HandleAssetTerminalConfig(w http.ResponseWriter, r *http.Request, assetEntry assets.Asset) {
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, ok, err := d.CredentialStore.GetAssetTerminalConfig(assetEntry.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load terminal config")
			return
		}
		if !ok {
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"asset":           assetEntry,
				"terminal_config": nil,
			})
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"asset":           assetEntry,
			"terminal_config": cfg,
		})
	case http.MethodPut, http.MethodPatch:
		if !d.EnforceRateLimit(w, r, "assets.terminal_config.update", 120, time.Minute) {
			return
		}

		var req credentials.UpdateAssetTerminalConfigRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid terminal config payload")
			return
		}
		if err := shared.ValidateMaxLen("host", strings.TrimSpace(req.Host), maxTargetLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := shared.ValidateMaxLen("username", strings.TrimSpace(req.Username), maxActorIDLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := shared.ValidateMaxLen("credential_profile_id", strings.TrimSpace(req.CredentialProfileID), maxTargetLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := shared.ValidateMaxLen("host_key", strings.TrimSpace(req.HostKey), maxHostKeyLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		existing, _, err := d.CredentialStore.GetAssetTerminalConfig(assetEntry.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load existing terminal config")
			return
		}

		cfg := existing
		if r.Method == http.MethodPut {
			cfg = credentials.AssetTerminalConfig{AssetID: assetEntry.ID}
		}
		cfg.AssetID = assetEntry.ID
		if host := strings.TrimSpace(req.Host); host != "" || r.Method == http.MethodPut {
			cfg.Host = host
		}
		if req.Port > 0 || r.Method == http.MethodPut {
			cfg.Port = req.Port
		}
		if username := strings.TrimSpace(req.Username); username != "" || r.Method == http.MethodPut {
			cfg.Username = username
		}
		if req.StrictHostKey != nil {
			cfg.StrictHostKey = *req.StrictHostKey
		}
		if req.HostKey != "" || req.StrictHostKey != nil || r.Method == http.MethodPut {
			cfg.HostKey = strings.TrimSpace(req.HostKey)
		}
		if req.CredentialProfileID != "" || r.Method == http.MethodPut {
			cfg.CredentialProfileID = strings.TrimSpace(req.CredentialProfileID)
		}

		if strings.TrimSpace(cfg.Host) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "host is required")
			return
		}
		if cfg.Port <= 0 {
			cfg.Port = 22
		}
		if cfg.CredentialProfileID != "" {
			if _, ok, err := d.CredentialStore.GetCredentialProfile(cfg.CredentialProfileID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate credential profile")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusBadRequest, "credential_profile_id not found")
				return
			}
		}

		saved, err := d.CredentialStore.SaveAssetTerminalConfig(cfg)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save terminal config")
			return
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"asset":           assetEntry,
			"terminal_config": saved,
		})
	case http.MethodDelete:
		if !d.EnforceRateLimit(w, r, "assets.terminal_config.delete", 120, time.Minute) {
			return
		}
		if err := d.CredentialStore.DeleteAssetTerminalConfig(assetEntry.ID); err != nil {
			if err == persistence.ErrNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "terminal config not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete terminal config")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAuditEvents handles GET /audit/events.
func (d *Deps) HandleAuditEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuditStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "audit store unavailable")
		return
	}

	limit := parseLimit(r, 100)
	events, err := d.AuditStore.List(limit, shared.ParseOffset(r))
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}
