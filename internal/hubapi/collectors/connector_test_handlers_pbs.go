package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandlePBSConnectorTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL      string `json:"base_url"`
		TokenID      string `json:"token_id"`
		TokenSecret  string `json:"token_secret"`
		CredentialID string `json:"credential_id,omitempty"`
		SkipVerify   *bool  `json:"skip_verify,omitempty"`
		CAPEM        string `json:"ca_pem,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid test payload")
		return
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	tokenID := strings.TrimSpace(req.TokenID)
	tokenSecret := strings.TrimSpace(req.TokenSecret)
	credentialID := strings.TrimSpace(req.CredentialID)

	if tokenSecret == "" && credentialID != "" {
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
		tokenSecret = strings.TrimSpace(decrypted)
		if tokenID == "" {
			tokenID = strings.TrimSpace(profile.Username)
		}
		if baseURL == "" {
			baseURL = strings.TrimSpace(profile.Metadata["base_url"])
		}
	}

	if baseURL == "" || tokenID == "" || tokenSecret == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "base_url, token_id, and token_secret (or credential_id) are required")
		return
	}

	skipVerify := false
	if req.SkipVerify != nil {
		skipVerify = *req.SkipVerify
	}

	client, err := pbs.NewClient(pbs.Config{
		BaseURL:     baseURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		SkipVerify:  skipVerify,
		CAPEM:       strings.TrimSpace(req.CAPEM),
		Timeout:     15 * time.Second,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, SanitizeConnectorError(err, tokenSecret))
		return
	}

	ping, err := client.Ping(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorError(err, tokenSecret))
		return
	}
	if !ping.Pong {
		servicehttp.WriteError(w, http.StatusBadGateway, "pbs ping did not return pong=true")
		return
	}

	version, err := client.GetVersion(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorError(err, tokenSecret))
		return
	}
	datastores, err := client.ListDatastores(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, SanitizeConnectorError(err, tokenSecret))
		return
	}

	release := strings.TrimSpace(version.Release)
	if release == "" {
		release = strings.TrimSpace(version.Version)
	}

	message := "pbs API reachable"
	if release != "" {
		message += " (" + release + ")"
	}
	visibleDatastores := 0
	for _, datastore := range datastores {
		if strings.TrimSpace(datastore.Store) != "" {
			visibleDatastores++
		}
	}
	warning := ""
	if visibleDatastores == 0 {
		warning = "Connected, but the token can see 0 datastores."
		message += "; token can see 0 datastores"
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"message":            message,
		"warning":            warning,
		"release":            release,
		"pong":               ping.Pong,
		"visible_datastores": visibleDatastores,
	})
}
