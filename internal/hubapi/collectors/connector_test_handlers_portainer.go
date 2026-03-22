package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandlePortainerConnectorTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL      string `json:"base_url"`
		AuthMethod   string `json:"auth_method,omitempty"`
		TokenID      string `json:"token_id"`
		TokenSecret  string `json:"token_secret"`
		Username     string `json:"username,omitempty"`
		Password     string `json:"password,omitempty"` // #nosec G117 -- Connector test request carries runtime auth material.
		CredentialID string `json:"credential_id,omitempty"`
		SkipVerify   *bool  `json:"skip_verify,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	authMethod := strings.ToLower(strings.TrimSpace(req.AuthMethod))
	if authMethod == "" {
		authMethod = "api_key"
	}
	tokenSecret := strings.TrimSpace(req.TokenSecret)
	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	credentialID := strings.TrimSpace(req.CredentialID)

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
			password = strings.TrimSpace(decrypted)
			if username == "" {
				username = strings.TrimSpace(profile.Username)
			}
		} else {
			tokenSecret = strings.TrimSpace(decrypted)
		}
		if baseURL == "" {
			baseURL = strings.TrimSpace(profile.Metadata["base_url"])
		}
	}

	skipVerify := false
	if req.SkipVerify != nil {
		skipVerify = *req.SkipVerify
	}

	var cfg portainer.Config
	switch authMethod {
	case "password":
		if baseURL == "" || username == "" || password == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "base_url, username, and password are required for password auth")
			return
		}
		cfg = portainer.Config{
			BaseURL:    baseURL,
			Username:   username,
			Password:   password,
			SkipVerify: skipVerify,
			Timeout:    15 * time.Second,
		}
	case "api_key":
		if baseURL == "" || tokenSecret == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "base_url and token_secret (or credential_id) are required")
			return
		}
		cfg = portainer.Config{
			BaseURL:    baseURL,
			APIKey:     tokenSecret,
			SkipVerify: skipVerify,
			Timeout:    15 * time.Second,
		}
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "unsupported auth_method: must be api_key or password")
		return
	}

	client := portainer.NewClient(cfg)
	connector := portainer.NewWithClient(client)
	health, _ := connector.TestConnection(r.Context())
	if health.Status != "ok" {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorErrorMessage(health.Message, tokenSecret, password))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": health.Message,
	})
}

func (d *Deps) HandleDockerConnectorTest(w http.ResponseWriter, r *http.Request) {
	if d.ConnectorRegistry == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "connector registry unavailable")
		return
	}

	connector, ok := d.ConnectorRegistry.Get("docker")
	if !ok || connector == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "docker connector not found")
		return
	}

	health, err := connector.TestConnection(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorError(err))
		return
	}
	if health.Status != "ok" {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorErrorMessage(health.Message))
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": health.Message,
	})
}
