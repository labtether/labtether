package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authUsersRoute      = "/auth/users"
	authUsersRouteSlash = "/auth/users/"
	oidcStateTTL        = 10 * time.Minute
	oidcMobileStateTTL  = 5 * time.Minute
	// MobileOIDCRedirectURI is the single native callback registered by the
	// LabTether iOS/iPadOS application. PKCE protects authorization codes even
	// if another installed app claims the custom scheme.
	MobileOIDCRedirectURI = "com.labtether.mobile:/oauth2redirect" // #nosec G101 -- Public application callback URI, not credential material.
)

// ErrOIDCSetupRequired is returned when OIDC sign-in is attempted before initial setup.
var ErrOIDCSetupRequired = errors.New("initial setup required before oidc sign-in")

type authOIDCStartRequest struct {
	RedirectURI string `json:"redirect_uri"`
	Next        string `json:"next"`
}

type authOIDCCallbackRequest struct {
	Code        string `json:"code"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

// HandleAuthProviders handles GET /auth/providers.
func (d *Deps) HandleAuthProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.URL.Path != "/auth/providers" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	oidcProvider, oidcAutoProvision := d.OIDCRef.Get()
	payload := map[string]any{
		"local": map[string]any{"enabled": true},
		"oidc": map[string]any{
			"enabled":      oidcProvider != nil,
			"display_name": "Single Sign-On",
		},
	}
	if oidcProvider != nil {
		payload["oidc"] = map[string]any{
			"enabled":                true,
			"display_name":           oidcProvider.DisplayName(),
			"issuer":                 oidcProvider.IssuerURL(),
			"auto_provision":         oidcAutoProvision,
			"mobile_supported":       true,
			"mobile_redirect_uri":    MobileOIDCRedirectURI,
			"pkce_methods_supported": []string{auth.PKCECodeChallengeMethodS256},
		}
	}
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

func (d *Deps) validateOIDCRedirectURI(r *http.Request, raw string) (string, error) {
	if d != nil && d.ValidateOIDCRedirectURI != nil {
		return d.ValidateOIDCRedirectURI(r, raw)
	}
	return ValidateAuthRedirectURI(raw)
}

// HandleAuthOIDCStart handles POST /auth/oidc/start.
func (d *Deps) HandleAuthOIDCStart(w http.ResponseWriter, r *http.Request) {
	setOIDCNoStoreHeaders(w)
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	oidcProvider, _ := d.OIDCRef.Get()
	if oidcProvider == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "oidc is not enabled")
		return
	}
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.oidc.web.start.global", 120, time.Minute) {
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.oidc.start", 20, time.Minute) {
		return
	}

	var req authOIDCStartRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid oidc start payload")
		return
	}
	redirectURI, err := d.validateOIDCRedirectURI(r, req.RedirectURI)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := RandomURLToken(32)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate oidc state")
		return
	}
	nonce, err := RandomURLToken(32)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate oidc nonce")
		return
	}

	expiresAt := time.Now().UTC().Add(oidcStateTTL)
	if !d.StoreOIDCState(state, OIDCAuthState{
		Nonce:       nonce,
		NextPath:    SanitizeNextPath(req.Next),
		RedirectURI: redirectURI,
		Flow:        OIDCAuthFlowWeb,
		ExpiresAt:   expiresAt,
	}) {
		servicehttp.WriteError(w, http.StatusTooManyRequests, "too many pending oidc sign-in attempts")
		return
	}
	authURL, err := oidcProvider.BuildAuthURL(state, nonce, redirectURI)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to build oidc auth url")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"auth_url":   authURL,
		"expires_at": expiresAt,
	})
}

func (d *Deps) appendOIDCAudit(clientType, decision, reason string, user auth.User, session auth.Session, created bool) {
	if d == nil || d.AppendAuditEventBestEffort == nil {
		return
	}
	details := map[string]any{
		"client_type": clientType,
		"provider":    "oidc",
	}
	if user.ID != "" {
		details["role"] = auth.NormalizeRole(user.Role)
		details["created"] = created
	}
	d.AppendAuditEventBestEffort(audit.Event{
		Type:      "auth.oidc.login",
		ActorID:   user.ID,
		Target:    "oidc",
		SessionID: session.ID,
		Decision:  decision,
		Reason:    reason,
		Details:   details,
		Timestamp: time.Now().UTC(),
	}, clientType+" oidc login "+decision)
}

func setOIDCNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// HandleAuthOIDCCallback handles POST /auth/oidc/callback.
func (d *Deps) HandleAuthOIDCCallback(w http.ResponseWriter, r *http.Request) {
	setOIDCNoStoreHeaders(w)
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	oidcProvider, _ := d.OIDCRef.Get()
	if oidcProvider == nil {
		servicehttp.WriteError(w, http.StatusNotFound, "oidc is not enabled")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.oidc.web.callback.global", 120, time.Minute) {
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.oidc.callback", 20, time.Minute) {
		return
	}

	var req authOIDCCallbackRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid oidc callback payload")
		return
	}
	redirectURI, err := d.validateOIDCRedirectURI(r, req.RedirectURI)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, ok := d.ConsumeOIDCState(strings.TrimSpace(req.State), redirectURI)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "oidc state is invalid or expired")
		return
	}

	identity, err := oidcProvider.ExchangeCode(r.Context(), req.Code, state.Nonce, redirectURI)
	if err != nil {
		d.appendOIDCAudit("web", "deny", "provider_exchange_failed", auth.User{}, auth.Session{}, false)
		servicehttp.WriteError(w, http.StatusUnauthorized, "oidc authentication failed")
		return
	}

	user, created, err := d.ResolveOIDCUser(identity)
	if err != nil {
		if errors.Is(err, ErrOIDCSetupRequired) {
			servicehttp.WriteError(w, http.StatusConflict, "complete initial setup before using single sign-on")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to provision oidc user")
		return
	}

	raw, hashed, err := auth.GenerateSessionToken()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expiresAt := time.Now().UTC().Add(auth.SessionDuration)
	session, err := d.AuthStore.CreateAuthSession(user.ID, hashed, expiresAt)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	auth.SetSessionCookie(w, raw, auth.SessionDuration)
	d.appendOIDCAudit("web", "allow", "", user, session, created)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user":       auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
		"created":    created,
		"next":       state.NextPath,
		"session_id": session.ID,
		"expires_at": session.ExpiresAt,
	})
}

// SanitizeNextPath sanitizes a redirect-next path.
func SanitizeNextPath(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || strings.Contains(next, "\\") || strings.ContainsAny(next, "\x00\r\n") {
		return "/"
	}
	parsed, err := url.ParseRequestURI(next)
	if err != nil {
		return "/"
	}
	if parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}
	if !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") {
		return "/"
	}
	lower := strings.ToLower(parsed.Path)
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
		return "/"
	}
	normalized := parsed.Path
	if normalized == "" {
		normalized = "/"
	}
	if parsed.RawQuery != "" {
		normalized += "?" + parsed.RawQuery
	}
	return normalized
}

// ValidateAuthRedirectURI validates an OAuth2 redirect URI.
func ValidateAuthRedirectURI(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("redirect_uri is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("redirect_uri is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("redirect_uri must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("redirect_uri host is required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("redirect_uri userinfo is not allowed")
	}
	if parsed.RawQuery != "" {
		return "", fmt.Errorf("redirect_uri query is not allowed")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("redirect_uri fragment is not allowed")
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path != "/api/auth/oidc/callback" && path != "/auth/oidc/callback" {
		return "", fmt.Errorf("redirect_uri must target the oidc callback endpoint")
	}
	return parsed.String(), nil
}

// RandomURLToken generates a random URL-safe base64 token.
func RandomURLToken(length int) (string, error) {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
