package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleTrueNASConnectorTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL      string `json:"base_url"`
		APIKey       string `json:"api_key"` // #nosec G117 -- Connector test request carries runtime auth material.
		CredentialID string `json:"credential_id,omitempty"`
		SkipVerify   *bool  `json:"skip_verify,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	credentialID := strings.TrimSpace(req.CredentialID)

	// Resolve API key from credential when not provided inline.
	if apiKey == "" && credentialID != "" {
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
		apiKey = strings.TrimSpace(decrypted)
		if baseURL == "" {
			baseURL = strings.TrimSpace(profile.Metadata["base_url"])
		}
	}
	if baseURL == "" || apiKey == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "base_url and api_key (or credential_id) are required")
		return
	}
	if _, err := securityruntime.ValidateOutboundURL(baseURL); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	skipVerify := false
	if req.SkipVerify != nil {
		skipVerify = *req.SkipVerify
	}

	connector := truenas.NewWithConfig(truenas.Config{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		SkipVerify: skipVerify,
		Timeout:    15 * time.Second,
	})

	health, _ := connector.TestConnection(r.Context())
	if health.Status != "ok" {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorErrorMessage(strings.TrimSpace(health.Message), apiKey))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": health.Message,
	})
}
