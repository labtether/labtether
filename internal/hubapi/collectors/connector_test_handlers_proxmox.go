package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleProxmoxConnectorTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL      string `json:"base_url"`
		AuthMethod   string `json:"auth_method,omitempty"`
		TokenID      string `json:"token_id"`
		TokenSecret  string `json:"token_secret"`
		Username     string `json:"username,omitempty"`
		Password     string `json:"password,omitempty"` // #nosec G117 -- Connector test request carries runtime auth material.
		CredentialID string `json:"credential_id,omitempty"`
		SkipVerify   *bool  `json:"skip_verify,omitempty"`
		CAPEM        string `json:"ca_pem,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	authMethod := strings.TrimSpace(req.AuthMethod)
	tokenID := strings.TrimSpace(req.TokenID)
	tokenSecret := strings.TrimSpace(req.TokenSecret)
	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	credentialID := strings.TrimSpace(req.CredentialID)

	// Resolve from credential profile if inline secrets are not provided.
	if tokenSecret == "" && password == "" && credentialID != "" {
		if d.CredentialStore == nil || d.SecretsManager == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
			return
		}
		profile, ok, err := d.CredentialStore.GetCredentialProfile(credentialID)
		if err != nil || !ok {
			servicehttp.WriteError(w, http.StatusBadRequest, "credential_id not found")
			return
		}
		decrypted, err := d.SecretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "failed to decrypt credential secret")
			return
		}
		if authMethod == "password" {
			password = decrypted
			if username == "" {
				username = strings.TrimSpace(profile.Username)
			}
		} else {
			tokenSecret = strings.TrimSpace(decrypted)
			if tokenID == "" {
				tokenID = strings.TrimSpace(profile.Username)
			}
		}
		if baseURL == "" {
			baseURL = strings.TrimSpace(profile.Metadata["base_url"])
		}
	}

	skipVerify := false
	if req.SkipVerify != nil {
		skipVerify = *req.SkipVerify
	}

	var cfg proxmox.Config
	if authMethod == "password" {
		if baseURL == "" || username == "" || password == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "base_url, username, and password are required for password auth")
			return
		}
		cfg = proxmox.Config{
			BaseURL:    baseURL,
			AuthMode:   proxmox.AuthModePassword,
			Username:   username,
			Password:   password,
			SkipVerify: skipVerify,
			CAPEM:      strings.TrimSpace(req.CAPEM),
			Timeout:    15 * time.Second,
		}
	} else {
		if baseURL == "" || tokenID == "" || tokenSecret == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "base_url, token_id, and token_secret (or credential_id) are required")
			return
		}
		cfg = proxmox.Config{
			BaseURL:     baseURL,
			TokenID:     tokenID,
			TokenSecret: tokenSecret,
			SkipVerify:  skipVerify,
			CAPEM:       strings.TrimSpace(req.CAPEM),
			Timeout:     15 * time.Second,
		}
	}

	client, err := proxmox.NewClient(cfg)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, SanitizeConnectorError(err, tokenSecret, password))
		return
	}

	release, err := client.GetVersion(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorError(err, tokenSecret, password))
		return
	}

	message := "proxmox API reachable"
	if release != "" {
		message = message + " (" + release + ")"
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": message,
		"release": release,
	})
}
