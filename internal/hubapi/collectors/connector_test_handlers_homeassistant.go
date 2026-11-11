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
		// AllowLocalHTTP: "Allow http on my network" for this connection.
		AllowLocalHTTP bool `json:"allow_local_http,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	token := strings.TrimSpace(req.Token)
	credentialID := strings.TrimSpace(req.CredentialID)
	credentialProfile, credentialOK := d.loadAuthorizedCredentialProfile(w, r, credentialID)
	if !credentialOK {
		return
	}

	if token == "" && credentialID != "" {
		if d.SecretsManager == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
			return
		}
		decrypted, err := d.SecretsManager.DecryptString(credentialProfile.SecretCiphertext, credentialProfile.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "failed to decrypt credential secret")
			return
		}
		token = strings.TrimSpace(decrypted)
		if baseURL == "" {
			baseURL = strings.TrimSpace(credentialProfile.Metadata["base_url"])
		}
	}

	if baseURL == "" || token == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "base_url and token (or credential_id) are required")
		return
	}
	if err := validateHomeAssistantBaseURL(baseURL, req.AllowLocalHTTP); err != nil {
		writeConnectorTestFailure(w, "Home Assistant", err.Error())
		return
	}

	skipVerify := false
	if req.SkipVerify != nil {
		skipVerify = *req.SkipVerify
	}

	connector := homeassistant.NewWithConfig(homeassistant.Config{
		BaseURL:        baseURL,
		Token:          token,
		SkipVerify:     skipVerify,
		Timeout:        15 * time.Second,
		AllowLocalHTTP: req.AllowLocalHTTP,
	})
	health, _ := connector.TestConnection(r.Context())
	if health.Status != "ok" {
		writeConnectorTestFailure(w, "Home Assistant", health.Message, token)
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": health.Message,
	})
}

// validateHomeAssistantBaseURL checks the address as the connection will use
// it: plain http on the local network when its owner allowed that, otherwise
// the Hub's outbound policy (https, or http with
// LABTETHER_ALLOW_INSECURE_TRANSPORT).
func validateHomeAssistantBaseURL(baseURL string, allowLocalHTTP bool) error {
	if allowLocalHTTP && homeassistant.UsesPlainHTTP(baseURL) && !securityruntime.InsecureTransportAllowed() {
		_, err := securityruntime.ValidateLocalHTTPURL(baseURL)
		return err
	}
	_, err := securityruntime.ValidateOutboundURL(baseURL)
	return err
}
