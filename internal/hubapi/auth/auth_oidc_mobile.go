package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
)

type authOIDCMobileStartRequest struct {
	RedirectURI         string `json:"redirect_uri"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

type authOIDCMobileCallbackRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
}

// HandleAuthOIDCMobileStart handles POST /auth/oidc/mobile/start. The native
// app owns the verifier and sends only its S256 challenge at this stage.
func (d *Deps) HandleAuthOIDCMobileStart(w http.ResponseWriter, r *http.Request) {
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
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.oidc.mobile.start.global", 120, time.Minute) {
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.oidc.mobile.start", 20, time.Minute) {
		return
	}

	var req authOIDCMobileStartRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid oidc mobile start payload")
		return
	}
	redirectURI, err := ValidateMobileOIDCRedirectURI(req.RedirectURI)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.CodeChallengeMethod != auth.PKCECodeChallengeMethodS256 {
		servicehttp.WriteError(w, http.StatusBadRequest, "code_challenge_method must be S256")
		return
	}
	codeChallenge := strings.TrimSpace(req.CodeChallenge)
	if err := auth.ValidatePKCECodeChallenge(codeChallenge); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid pkce code challenge")
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
	authURL, err := oidcProvider.BuildAuthURLWithPKCE(state, nonce, redirectURI, codeChallenge)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to build oidc auth url")
		return
	}

	expiresAt := time.Now().UTC().Add(oidcMobileStateTTL)
	if !d.StoreOIDCState(state, OIDCAuthState{
		Nonce:         nonce,
		RedirectURI:   redirectURI,
		Flow:          OIDCAuthFlowMobile,
		CodeChallenge: codeChallenge,
		ExpiresAt:     expiresAt,
	}) {
		servicehttp.WriteError(w, http.StatusTooManyRequests, "too many pending oidc sign-in attempts")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"auth_url":   authURL,
		"state":      state,
		"expires_at": expiresAt,
	})
}

// HandleAuthOIDCMobileCallback handles POST /auth/oidc/mobile/callback. It
// consumes state before contacting the provider and requires proof of the
// verifier bound at start, then creates the same cookie session as web OIDC.
func (d *Deps) HandleAuthOIDCMobileCallback(w http.ResponseWriter, r *http.Request) {
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
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.oidc.mobile.callback.global", 120, time.Minute) {
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.oidc.mobile.callback", 20, time.Minute) {
		return
	}

	var req authOIDCMobileCallbackRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid oidc mobile callback payload")
		return
	}
	redirectURI, err := ValidateMobileOIDCRedirectURI(req.RedirectURI)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	codeChallenge, err := auth.PKCECodeChallengeS256(req.CodeVerifier)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "oidc state or pkce verifier is invalid or expired")
		return
	}
	state, ok := d.ConsumeMobileOIDCState(strings.TrimSpace(req.State), redirectURI)
	if !ok || subtle.ConstantTimeCompare([]byte(state.CodeChallenge), []byte(codeChallenge)) != 1 {
		servicehttp.WriteError(w, http.StatusBadRequest, "oidc state or pkce verifier is invalid or expired")
		return
	}

	identity, err := oidcProvider.ExchangeCodeWithPKCE(r.Context(), req.Code, state.Nonce, redirectURI, req.CodeVerifier)
	if err != nil {
		d.appendOIDCAudit("native_mobile", "deny", "provider_exchange_failed", auth.User{}, auth.Session{}, false)
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
	d.appendOIDCAudit("native_mobile", "allow", "", user, session, created)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user":       auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
		"created":    created,
		"session_id": session.ID,
		"expires_at": session.ExpiresAt,
	})
}

// ValidateMobileOIDCRedirectURI enforces the native app's single registered
// callback. No caller-provided custom scheme, host, path, query, or fragment is
// accepted, so the public start endpoint cannot become an open redirect.
func ValidateMobileOIDCRedirectURI(raw string) (string, error) {
	if strings.TrimSpace(raw) != MobileOIDCRedirectURI {
		return "", fmt.Errorf("redirect_uri must exactly match %s", MobileOIDCRedirectURI)
	}
	return MobileOIDCRedirectURI, nil
}
