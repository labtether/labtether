package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	maxCredentialNameLength = 120
	maxCredentialKindLength = 32
	maxCredentialSecretLen  = 16384
	maxActorIDLength        = 64
	maxCommandLength        = 4096
)

// HandleCredentialProfiles handles GET/POST /credentials/profiles.
func (d *Deps) HandleCredentialProfiles(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/credentials/profiles" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		profiles, err := d.CredentialStore.ListCredentialProfiles(shared.ParseLimit(r, 100))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list credential profiles")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "credentials.profile.create", 60, time.Minute) {
			return
		}
		if d.SecretsManager == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}

		var req credentials.CreateProfileRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid credential profile payload")
			return
		}
		if err := validateCreateProfileRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		profileID := idgen.New("cred")

		secretCiphertext, err := d.SecretsManager.EncryptString(strings.TrimSpace(req.Secret), profileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt credential secret")
			return
		}

		passphraseCiphertext := ""
		if strings.TrimSpace(req.Passphrase) != "" {
			passphraseCiphertext, err = d.SecretsManager.EncryptString(strings.TrimSpace(req.Passphrase), profileID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt passphrase")
				return
			}
		}

		profile := credentials.Profile{
			ID:                   profileID,
			Name:                 strings.TrimSpace(req.Name),
			Kind:                 strings.TrimSpace(req.Kind),
			Username:             strings.TrimSpace(req.Username),
			Description:          strings.TrimSpace(req.Description),
			Status:               "active",
			Metadata:             cloneMetadata(req.Metadata),
			SecretCiphertext:     secretCiphertext,
			PassphraseCiphertext: passphraseCiphertext,
			ExpiresAt:            cloneTimePtr(req.ExpiresAt),
		}

		created, err := d.CredentialStore.CreateCredentialProfile(profile)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create credential profile")
			return
		}

		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"profile": created})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleCredentialProfileActions handles GET/DELETE /credentials/profiles/{id}
// and POST /credentials/profiles/{id}/rotate.
func (d *Deps) HandleCredentialProfileActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/credentials/profiles/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "credential profile path not found")
		return
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	profileID := strings.TrimSpace(parts[0])
	if profileID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "credential profile path not found")
		return
	}

	profile, ok, err := d.CredentialStore.GetCredentialProfile(profileID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load credential profile")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "credential profile not found")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"profile": profile})
		case http.MethodDelete:
			if err := d.CredentialStore.DeleteCredentialProfile(profileID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete credential profile")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "profile_id": profileID})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "rotate" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "credentials.profile.rotate", 120, time.Minute) {
			return
		}
		if d.SecretsManager == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}

		var req credentials.RotateProfileRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid credential rotation payload")
			return
		}
		req.Secret = strings.TrimSpace(req.Secret)
		req.Passphrase = strings.TrimSpace(req.Passphrase)
		req.Reason = strings.TrimSpace(req.Reason)
		if req.Secret == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "secret is required")
			return
		}
		if err := shared.ValidateMaxLen("secret", req.Secret, maxCredentialSecretLen); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := shared.ValidateMaxLen("passphrase", req.Passphrase, maxCredentialSecretLen); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		secretCiphertext, err := d.SecretsManager.EncryptString(req.Secret, profileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt credential secret")
			return
		}
		passphraseCiphertext := ""
		if req.Passphrase != "" {
			passphraseCiphertext, err = d.SecretsManager.EncryptString(req.Passphrase, profileID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt passphrase")
				return
			}
		}

		updated, err := d.CredentialStore.UpdateCredentialProfileSecret(profileID, secretCiphertext, passphraseCiphertext, cloneTimePtr(req.ExpiresAt))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to rotate credential profile secret")
			return
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"profile": updated,
			"rotation": map[string]any{
				"reason": req.Reason,
			},
		})
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown credential profile action")
}

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

// validateCreateProfileRequest validates the fields in a CreateProfileRequest.
func validateCreateProfileRequest(req credentials.CreateProfileRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Username = strings.TrimSpace(req.Username)
	req.Description = strings.TrimSpace(req.Description)
	req.Secret = strings.TrimSpace(req.Secret)
	req.Passphrase = strings.TrimSpace(req.Passphrase)

	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.Kind == "" {
		return errors.New("kind is required")
	}
	if req.Secret == "" {
		return errors.New("secret is required")
	}
	if req.Kind != credentials.KindSSHPassword &&
		req.Kind != credentials.KindSSHPrivateKey &&
		req.Kind != credentials.KindVNCPassword &&
		req.Kind != credentials.KindProxmoxAPIToken &&
		req.Kind != credentials.KindProxmoxPassword &&
		req.Kind != credentials.KindPBSAPIToken &&
		req.Kind != credentials.KindPortainerAPIKey &&
		req.Kind != credentials.KindTrueNASAPIKey &&
		req.Kind != credentials.KindHomeAssistantToken &&
		req.Kind != credentials.KindTelnetPassword &&
		req.Kind != credentials.KindRDPPassword &&
		req.Kind != credentials.KindFTPPassword &&
		req.Kind != credentials.KindSMBCredentials &&
		req.Kind != credentials.KindWebDAVCredentials {
		return fmt.Errorf(
			"kind must be one of the supported credential types (ssh_password, ssh_private_key, vnc_password, proxmox_api_token, proxmox_password, pbs_api_token, portainer_api_key, truenas_api_key, homeassistant_token, telnet_password, rdp_password, ftp_password, smb_credentials, webdav_credentials)",
		)
	}
	if err := shared.ValidateMaxLen("name", req.Name, maxCredentialNameLength); err != nil {
		return err
	}
	if err := shared.ValidateMaxLen("kind", req.Kind, maxCredentialKindLength); err != nil {
		return err
	}
	if err := shared.ValidateMaxLen("username", req.Username, maxActorIDLength); err != nil {
		return err
	}
	if err := shared.ValidateMaxLen("description", req.Description, maxCommandLength); err != nil {
		return err
	}
	if err := shared.ValidateMaxLen("secret", req.Secret, maxCredentialSecretLen); err != nil {
		return err
	}
	if err := shared.ValidateMaxLen("passphrase", req.Passphrase, maxCredentialSecretLen); err != nil {
		return err
	}
	return nil
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	out := value.UTC()
	return &out
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
