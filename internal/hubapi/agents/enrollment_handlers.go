package agents

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/platforms"
	"github.com/labtether/labtether/internal/servicehttp"
)

// hubSchemes returns the HTTP and WebSocket scheme strings based on whether TLS is enabled.
func (d *Deps) HubSchemes() (httpScheme, wsScheme string) {
	if d.TLSEnabled {
		return "https", "wss"
	}
	return "http", "ws"
}

func (d *Deps) ResolvePublicHubHost(r *http.Request) string {
	if sanitized, ok := shared.SanitizeHostPort(r.Host); ok {
		return sanitized
	}

	return net.JoinHostPort("localhost", strconv.Itoa(shared.EnvOrDefaultInt("API_PORT", 8080)))
}

// handleEnroll processes POST /api/v1/enroll — unauthenticated (validates enrollment token).
func (d *Deps) HandleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	// Rate limit: 30 requests per minute per IP
	if !d.EnforceRateLimit(w, r, "enroll", 30, time.Minute) {
		return
	}

	var req enrollment.EnrollRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid enrollment request")
		return
	}

	if strings.TrimSpace(req.EnrollmentToken) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "enrollment_token is required")
		return
	}
	if strings.TrimSpace(req.Hostname) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "hostname is required")
		return
	}

	// Validate enrollment token without consuming it yet — we check for
	// conflicts first so that duplicate hostnames don't burn one-time tokens.
	tokenHash := auth.HashToken(strings.TrimSpace(req.EnrollmentToken))
	_, valid, err := d.EnrollmentStore.ValidateEnrollmentToken(tokenHash)
	if err != nil {
		log.Printf("enroll: error validating enrollment token: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "enrollment error")
		return
	}
	if !valid {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid or expired enrollment token")
		return
	}

	// Generate asset ID from hostname (normalized to safe characters).
	assetID := NormalizeHostnameForAssetID(req.Hostname)
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "hostname produced an empty asset ID after normalization")
		return
	}
	resolvedPlatform := platforms.Resolve(req.Platform, "", "", "", "")
	if d.AssetStore != nil {
		if _, exists, err := d.AssetStore.GetAsset(assetID); err != nil {
			log.Printf("enroll: failed to validate existing asset %s: %v", assetID, err)
			servicehttp.WriteError(w, http.StatusInternalServerError, "enrollment error")
			return
		} else if exists {
			servicehttp.WriteError(w, http.StatusConflict, "hostname is already enrolled; create a new asset identity before re-enrolling")
			return
		}
	}

	// Now atomically consume the token after all precondition checks pass.
	etok, valid, err := d.EnrollmentStore.ConsumeEnrollmentToken(tokenHash)
	if err != nil {
		log.Printf("enroll: error consuming enrollment token: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "enrollment error")
		return
	}
	if !valid {
		servicehttp.WriteError(w, http.StatusUnauthorized, "enrollment token was consumed by a concurrent request")
		return
	}

	// Generate per-agent token
	rawToken, hashedToken, err := auth.GenerateSessionToken()
	if err != nil {
		log.Printf("enroll: error generating agent token: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "enrollment error")
		return
	}
	expiresAt := NewAgentTokenExpiry(time.Now().UTC())

	// Store agent token
	if _, err := d.EnrollmentStore.CreateAgentToken(assetID, hashedToken, etok.ID, expiresAt); err != nil {
		log.Printf("enroll: error creating agent token: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "enrollment error")
		return
	}

	// Upsert asset via heartbeat
	hbReq := assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "node",
		Name:     req.Hostname,
		Source:   "agent",
		GroupID:  strings.TrimSpace(req.GroupID),
		Status:   "online",
		Platform: resolvedPlatform,
	}
	if _, err := d.ProcessHeartbeatRequest(hbReq); err != nil {
		log.Printf("enroll: heartbeat upsert failed for %s: %v", assetID, err)
	}

	// Reuse the same public connection resolver as discovery/install flows so
	// auto-enrolled agents stay on the best reachable origin (for example the
	// active Tailscale HTTPS endpoint) instead of collapsing back to localhost.
	connection := d.ResolveHubConnectionSelection(r)
	hubAPIURL := connection.HubURL
	hubWSURL := connection.WSURL

	servicehttp.WriteJSON(w, http.StatusOK, enrollment.EnrollResponse{
		AgentToken: rawToken,
		AssetID:    assetID,
		HubWSURL:   hubWSURL,
		HubAPIURL:  hubAPIURL,
		CACertPEM:  string(d.CACertPEM),
	})
}

// handleDiscover returns hub connection info — unauthenticated.
func (d *Deps) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	connection := d.ResolveHubConnectionSelection(r)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"hub":            "labtether",
		"api_url":        connection.HubURL,
		"ws_url":         connection.WSURL,
		"enroll_url":     strings.TrimRight(connection.HubURL, "/") + "/api/v1/enroll",
		"hub_url":        connection.HubURL,
		"hub_ws_url":     connection.WSURL,
		"hub_candidates": connection.Candidates,
	})
}

// handleEnrollmentTokens handles GET/POST /settings/enrollment (admin-auth).
func (d *Deps) HandleEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		tokens, err := d.EnrollmentStore.ListEnrollmentTokens(50)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list enrollment tokens")
			return
		}
		if tokens == nil {
			tokens = []enrollment.EnrollmentToken{}
		}

		connection := d.ResolveHubConnectionSelection(r)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"tokens":         tokens,
			"hub_url":        connection.HubURL,
			"ws_url":         connection.WSURL,
			"hub_candidates": connection.Candidates,
		})

	case http.MethodPost:
		var req enrollment.CreateTokenRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request")
			return
		}

		if req.TTLHours <= 0 {
			req.TTLHours = 24
		}
		if req.TTLHours > 720 { // max 30 days
			req.TTLHours = 720
		}
		if req.MaxUses <= 0 {
			req.MaxUses = 1
		}

		expiresAt := time.Now().UTC().Add(time.Duration(req.TTLHours) * time.Hour)

		// Generate the raw token the admin will copy
		rawToken, hashedToken, err := auth.GenerateSessionToken()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		tok, err := d.EnrollmentStore.CreateEnrollmentToken(hashedToken, req.Label, expiresAt, req.MaxUses)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create enrollment token")
			return
		}

		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
			"token":     tok,
			"raw_token": rawToken,
		})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleEnrollmentTokenActions handles DELETE /settings/enrollment/{id} (admin-auth).
func (d *Deps) HandleEnrollmentTokenActions(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/settings/enrollment/")
	id = strings.TrimSpace(id)
	if id == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "token id required")
		return
	}

	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := d.EnrollmentStore.RevokeEnrollmentToken(id); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke enrollment token")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// handleAgentTokens handles GET /settings/agent-tokens (admin-auth).
func (d *Deps) HandleAgentTokens(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tokens, err := d.EnrollmentStore.ListAgentTokens(100)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list agent tokens")
		return
	}
	if tokens == nil {
		tokens = []enrollment.AgentToken{}
	}

	type agentTokenSummary struct {
		ID                string     `json:"id"`
		AssetID           string     `json:"asset_id"`
		Status            string     `json:"status"`
		EnrolledVia       string     `json:"enrolled_via,omitempty"`
		ExpiresAt         time.Time  `json:"expires_at"`
		LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
		CreatedAt         time.Time  `json:"created_at"`
		RevokedAt         *time.Time `json:"revoked_at,omitempty"`
		DeviceFingerprint string     `json:"device_fingerprint,omitempty"`
	}

	summaries := make([]agentTokenSummary, 0, len(tokens))
	for _, token := range tokens {
		summary := agentTokenSummary{
			ID:          token.ID,
			AssetID:     token.AssetID,
			Status:      token.Status,
			EnrolledVia: token.EnrolledVia,
			ExpiresAt:   token.ExpiresAt,
			LastUsedAt:  token.LastUsedAt,
			CreatedAt:   token.CreatedAt,
			RevokedAt:   token.RevokedAt,
		}
		if d.AssetStore != nil {
			if asset, ok, err := d.AssetStore.GetAsset(token.AssetID); err == nil && ok {
				summary.DeviceFingerprint = strings.TrimSpace(asset.Metadata["agent_device_fingerprint"])
			}
		}
		summaries = append(summaries, summary)
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"tokens": summaries})
}

// handleTokenCleanup handles DELETE /settings/tokens/cleanup (admin-auth).
// Deletes dead enrollment tokens (revoked/expired/exhausted) and agent tokens (revoked + never used).
func (d *Deps) HandleTokenCleanup(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	enrollDeleted, agentDeleted, err := d.EnrollmentStore.DeleteDeadTokens()
	if err != nil {
		log.Printf("token cleanup: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to clean up tokens")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"enrollment_deleted": enrollDeleted,
		"agent_deleted":      agentDeleted,
	})
}

// handleAgentTokenActions handles DELETE /settings/agent-tokens/{id} (admin-auth).
func (d *Deps) HandleAgentTokenActions(w http.ResponseWriter, r *http.Request) {
	if d.EnrollmentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "enrollment not available")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/settings/agent-tokens/")
	id = strings.TrimSpace(id)
	if id == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "token id required")
		return
	}

	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := d.EnrollmentStore.RevokeAgentToken(id); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke agent token")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
