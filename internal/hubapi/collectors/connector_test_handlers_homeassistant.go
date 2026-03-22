package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleHomeAssistantConnectorTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL      string `json:"base_url"`
		Token        string `json:"token"`
		CredentialID string `json:"credential_id,omitempty"`
		SkipVerify   *bool  `json:"skip_verify,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	token := strings.TrimSpace(req.Token)
	credentialID := strings.TrimSpace(req.CredentialID)

	if token == "" && credentialID != "" {
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
		token = strings.TrimSpace(decrypted)
		if baseURL == "" {
			baseURL = strings.TrimSpace(profile.Metadata["base_url"])
		}
	}

	if baseURL == "" || token == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "base_url and token (or credential_id) are required")
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

	connector := homeassistant.NewWithConfig(homeassistant.Config{
		BaseURL:    baseURL,
		Token:      token,
		SkipVerify: skipVerify,
		Timeout:    15 * time.Second,
	})
	health, _ := connector.TestConnection(r.Context())
	if health.Status != "ok" {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorErrorMessage(strings.TrimSpace(health.Message), token))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": health.Message,
	})
}
