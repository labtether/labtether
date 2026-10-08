package auth

import (
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
)

// HandleDesktopCredentials handles GET/POST/DELETE /assets/{id}/desktop/credentials.
// GET — returns { saved, username } indicating whether VNC credentials exist.
// POST — saves VNC credentials (encrypts password, creates profile, links config).
// DELETE — removes saved VNC credentials for this asset.
func (d *Deps) HandleDesktopCredentials(w http.ResponseWriter, r *http.Request) {
	if d.CredentialStore == nil || d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	// Extract asset ID from path: /assets/{id}/desktop/credentials
	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}
	if !d.ensureManagedDesktopCredentialAsset(w, assetID) {
		return
	}
	if !d.authorizeDesktopAssetAccess(w, r, assetID) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.handleGetDesktopCredentials(w, assetID)
	case http.MethodPost:
		d.handleSaveDesktopCredentials(w, r, assetID)
	case http.MethodDelete:
		d.handleDeleteDesktopCredentials(w, assetID)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) handleGetDesktopCredentials(w http.ResponseWriter, assetID string) {
	cfg, ok, err := d.CredentialStore.GetDesktopConfig(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok || cfg.CredentialProfileID == "" {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"saved": false})
		return
	}

	profile, found, err := d.CredentialStore.GetCredentialProfile(cfg.CredentialProfileID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"saved": false})
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"saved":    true,
		"username": profile.Username,
		"vnc_port": cfg.VNCPort,
	})
}

// HandleRetrieveDesktopCredentials handles POST /assets/{id}/desktop/credentials/retrieve.
// Returns decrypted VNC credentials for auto-fill in the browser VNC client.
func (d *Deps) HandleRetrieveDesktopCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireScope(w, r, "credentials:use") {
		return
	}
	if d.CredentialStore == nil || d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}
	if !d.ensureManagedDesktopCredentialAsset(w, assetID) {
		return
	}
	if !d.authorizeDesktopAssetAccess(w, r, assetID) {
		return
	}

	cfg, ok, err := d.CredentialStore.GetDesktopConfig(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok || cfg.CredentialProfileID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "no saved credentials")
		return
	}

	profile, found, err := d.CredentialStore.GetCredentialProfile(cfg.CredentialProfileID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		servicehttp.WriteError(w, http.StatusNotFound, "credential profile not found")
		return
	}

	password, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}

	_ = d.CredentialStore.MarkCredentialProfileUsed(profile.ID, time.Now().UTC())

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"username": profile.Username,
		"password": password,
	})
}

func (d *Deps) handleSaveDesktopCredentials(w http.ResponseWriter, r *http.Request, assetID string) {
	if !d.EnforceRateLimit(w, r, "desktop.credentials.save", 30, time.Minute) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
		VNCPort  int    `json:"vnc_port"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	// Check if a desktop config already exists — update profile if so.
	cfg, exists, err := d.CredentialStore.GetDesktopConfig(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load desktop config")
		return
	}
	var profileID string

	if exists && cfg.CredentialProfileID != "" {
		// Update existing profile secret.
		ciphertext, err := d.SecretsManager.EncryptString(req.Password, cfg.CredentialProfileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
		_, err = d.CredentialStore.UpdateCredentialProfileSecret(cfg.CredentialProfileID, ciphertext, "", nil)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update credentials")
			return
		}
		profileID = cfg.CredentialProfileID
	} else {
		// Create new credential profile.
		newProfileID := idgen.New("cred")
		ciphertext, err := d.SecretsManager.EncryptString(req.Password, newProfileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "encryption failed")
			return
		}
		profile := credentials.Profile{
			ID:               newProfileID,
			Name:             fmt.Sprintf("VNC — %s", assetID),
			Kind:             credentials.KindVNCPassword,
			Username:         strings.TrimSpace(req.Username),
			Description:      "Auto-saved VNC credentials",
			Status:           "active",
			SecretCiphertext: ciphertext,
		}
		created, err := d.CredentialStore.CreateCredentialProfile(profile)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create credential profile")
			return
		}
		profileID = created.ID
	}

	port := req.VNCPort
	if port <= 0 {
		port = 5900
	}
	_, err = d.CredentialStore.SaveDesktopConfig(credentials.AssetDesktopConfig{
		AssetID:             assetID,
		VNCPort:             port,
		CredentialProfileID: profileID,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save desktop config")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (d *Deps) handleDeleteDesktopCredentials(w http.ResponseWriter, assetID string) {
	// Delete the config (cascade will handle profile orphans later).
	cfg, ok, err := d.CredentialStore.GetDesktopConfig(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load desktop config")
		return
	}
	if ok {
		if err := d.CredentialStore.DeleteDesktopConfig(assetID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete desktop config")
			return
		}
		// Also delete the linked credential profile.
		if cfg.CredentialProfileID != "" {
			if err := d.CredentialStore.DeleteCredentialProfile(cfg.CredentialProfileID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete credential profile")
				return
			}
		}
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (d *Deps) ensureManagedDesktopCredentialAsset(w http.ResponseWriter, assetID string) bool {
	if d.AssetStore == nil {
		return true
	}
	_, ok, err := d.AssetStore.GetAsset(strings.TrimSpace(assetID))
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve asset")
		return false
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
		return false
	}
	return true
}

func (d *Deps) authorizeDesktopAssetAccess(w http.ResponseWriter, r *http.Request, assetID string) bool {
	actorID := ""
	if d.UserIDFromContext != nil {
		actorID = d.UserIDFromContext(r.Context())
	}
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: actorID,
		Target:  strings.TrimSpace(assetID),
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return false
	}
	return true
}
