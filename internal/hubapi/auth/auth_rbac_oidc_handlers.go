package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
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

type authCreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	Role     string `json:"role"`
}

type authUpdateUserRequest struct {
	Role     *string `json:"role,omitempty"`
	Password *string `json:"password,omitempty"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
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

// ResolveOIDCUser finds or creates a user from an OIDC identity.
func (d *Deps) ResolveOIDCUser(identity auth.OIDCIdentity) (auth.User, bool, error) {
	const provider = "oidc"
	issuer := strings.TrimSpace(identity.Issuer)
	subject := strings.TrimSpace(identity.Subject)
	if issuer == "" || subject == "" {
		return auth.User{}, false, errors.New("verified oidc issuer and subject are required")
	}
	desiredRole := OIDCAssignableRole(identity.Role)

	user, ok, err := d.AuthStore.GetUserByOIDCIdentity(provider, issuer, subject)
	if err != nil {
		return auth.User{}, false, err
	}
	if ok {
		if desiredRole != auth.NormalizeRole(user.Role) && auth.NormalizeRole(user.Role) != auth.RoleOwner {
			if updateErr := d.AuthStore.UpdateUserRole(user.ID, desiredRole); updateErr == nil {
				user.Role = desiredRole
			}
		}
		return user, false, nil
	}

	// Users provisioned before issuer-scoped identities have a subject but no
	// issuer. Adopt that exact row once, atomically. The first adoption preserves
	// its current role instead of applying a claim-driven role change as an
	// incidental side effect of the schema migration. Later logins follow the
	// existing role synchronization policy above.
	legacyUser, legacyOK, err := d.AuthStore.GetUserByOIDCSubject(provider, subject)
	if err != nil {
		return auth.User{}, false, err
	}
	if legacyOK {
		boundUser, bound, bindErr := d.AuthStore.BindLegacyOIDCIdentity(
			legacyUser.ID,
			provider,
			subject,
			issuer,
		)
		if bindErr == nil && bound {
			return boundUser, false, nil
		}

		// A concurrent login for the same verified issuer may have completed
		// the one-time binding. Accept it only when it is the same user; a
		// competing issuer or a different user fails closed.
		currentUser, currentOK, currentErr := d.AuthStore.GetUserByOIDCIdentity(provider, issuer, subject)
		if currentErr != nil {
			return auth.User{}, false, currentErr
		}
		if currentOK && currentUser.ID == legacyUser.ID {
			return currentUser, false, nil
		}
		if bindErr != nil {
			return auth.User{}, false, bindErr
		}
		return auth.User{}, false, errors.New("legacy oidc identity was bound by a competing issuer")
	}

	_, oidcAutoProvision := d.OIDCRef.Get()
	if !oidcAutoProvision {
		return auth.User{}, false, fmt.Errorf("oidc user %q is not provisioned", subject)
	}
	if setupRequired, setupErr := AuthBootstrapSetupRequired(d.AuthStore); setupErr != nil {
		return auth.User{}, false, setupErr
	} else if setupRequired {
		return auth.User{}, false, ErrOIDCSetupRequired
	}

	username := SelectOIDCUsername(identity)
	username, err = d.UniqueUsername(username)
	if err != nil {
		return auth.User{}, false, err
	}
	passwordHash, err := auth.HashPassword(GenerateSyntheticOIDCPassword())
	if err != nil {
		return auth.User{}, false, err
	}
	createdUser, err := d.AuthStore.CreateUserWithOIDCIdentity(
		username,
		passwordHash,
		desiredRole,
		provider,
		issuer,
		subject,
	)
	if err != nil {
		return auth.User{}, false, err
	}
	return createdUser, true, nil
}

// OIDCAssignableRole downgrades owner to admin for OIDC-provisioned users.
func OIDCAssignableRole(role string) string {
	normalized := auth.NormalizeRole(role)
	if normalized == auth.RoleOwner {
		return auth.RoleAdmin
	}
	return normalized
}

// GenerateSyntheticOIDCPassword generates a random password for OIDC-provisioned users.
func GenerateSyntheticOIDCPassword() string {
	token, err := RandomURLToken(48)
	if err != nil {
		return "oidc-fallback-password-not-used"
	}
	return token
}

// SelectOIDCUsername picks the best username from an OIDC identity.
func SelectOIDCUsername(identity auth.OIDCIdentity) string {
	for _, candidate := range []string{identity.PreferredUsername, identity.Email, identity.Name, identity.Subject} {
		normalized := NormalizeUsername(candidate)
		if normalized != "" {
			return normalized
		}
	}
	return "oidc-user"
}

// NormalizeUsername sanitizes and normalizes a username string.
func NormalizeUsername(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if at := strings.Index(raw, "@"); at > 0 {
		raw = raw[:at]
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return ""
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// UniqueUsername generates a unique username by appending numeric suffixes.
func (d *Deps) UniqueUsername(base string) (string, error) {
	candidate := NormalizeUsername(base)
	if candidate == "" {
		candidate = "oidc-user"
	}
	for i := 0; i < 100; i++ {
		name := candidate
		if i > 0 {
			name = fmt.Sprintf("%s-%d", candidate, i+1)
		}
		if len(name) > 64 {
			name = name[:64]
		}
		_, exists, err := d.AuthStore.GetUserByUsername(name)
		if err != nil {
			return "", err
		}
		if !exists {
			return name, nil
		}
	}
	return "", fmt.Errorf("unable to allocate username for oidc identity")
}

// HandleAuthUsers handles GET/POST /auth/users.
func (d *Deps) HandleAuthUsers(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != authUsersRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		users, err := d.AuthStore.ListUsers(200)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		type userEntry struct {
			ID           string `json:"id"`
			Username     string `json:"username"`
			Role         string `json:"role"`
			AuthProvider string `json:"auth_provider"`
		}
		items := make([]userEntry, 0, len(users))
		for _, u := range users {
			items = append(items, userEntry{ID: u.ID, Username: u.Username, Role: auth.NormalizeRole(u.Role), AuthProvider: u.AuthProvider})
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"users": items})
	case http.MethodPost:
		var req authCreateUserRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid user payload")
			return
		}
		req.Username = NormalizeUsername(req.Username)
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if role == "" {
			role = auth.RoleViewer
		}
		if !auth.IsValidRole(role) {
			servicehttp.WriteError(w, http.StatusBadRequest, "role must be owner, admin, operator, or viewer")
			return
		}
		req.Role = role
		if req.Username == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "username is required")
			return
		}
		if err := ValidateLoginRequest(LoginRequest{Username: req.Username, Password: req.Password}); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Role == auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "owner role is reserved")
			return
		}
		if _, exists, err := d.AuthStore.GetUserByUsername(req.Username); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate username")
			return
		} else if exists {
			servicehttp.WriteError(w, http.StatusConflict, "username already exists")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		created, err := d.AuthStore.CreateUserWithRole(req.Username, hash, req.Role, "local", "")
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
			"user": auth.UserInfo{ID: created.ID, Username: created.Username, Role: auth.NormalizeRole(created.Role)},
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAuthUserActions handles PATCH /auth/users/{id}.
func (d *Deps) HandleAuthUserActions(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, authUsersRouteSlash) {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, authUsersRouteSlash)
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "user id is required")
		return
	}

	if strings.HasSuffix(id, "/sessions") {
		userID := strings.TrimSuffix(id, "/sessions")
		d.handleUserSessions(w, r, userID)
		return
	}

	switch r.Method {
	case http.MethodPatch:
	case http.MethodDelete:
		d.handleDeleteUser(w, r, id)
		return
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req authUpdateUserRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid user update payload")
		return
	}
	if req.Role == nil && req.Password == nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "at least one field must be provided")
		return
	}

	user, ok, err := d.AuthStore.GetUserByID(id)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Role != nil {
		role := strings.ToLower(strings.TrimSpace(*req.Role))
		if !auth.IsValidRole(role) {
			servicehttp.WriteError(w, http.StatusBadRequest, "role must be owner, admin, operator, or viewer")
			return
		}
		if auth.NormalizeRole(user.Role) == auth.RoleOwner && role != auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "cannot change owner role")
			return
		}
		if role == auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "owner role is reserved")
			return
		}
		if err := d.AuthStore.UpdateUserRole(user.ID, role); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update user role")
			return
		}
		user.Role = role
	}
	if req.Password != nil {
		if auth.NormalizeRole(user.Role) == auth.RoleOwner && d.UserIDFromContext(r.Context()) != user.ID {
			servicehttp.WriteError(w, http.StatusForbidden, "cannot change owner password")
			return
		}
		password := *req.Password
		if err := ValidateLoginRequest(LoginRequest{Username: user.Username, Password: password}); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		if err := d.AuthStore.DeleteSessionsByUserID(user.ID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke active sessions")
			return
		}
		if err := d.AuthStore.UpdateUserPasswordHash(user.ID, hash); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update password")
			return
		}
		if user.ID == d.UserIDFromContext(r.Context()) {
			auth.ClearSessionCookie(w)
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user": auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
	})
}

func (d *Deps) handleDeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	callerID := d.UserIDFromContext(r.Context())
	if callerID == id {
		servicehttp.WriteError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	user, ok, err := d.AuthStore.GetUserByID(id)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Role == "owner" {
		servicehttp.WriteError(w, http.StatusForbidden, "cannot delete the owner account")
		return
	}

	if delErr := d.AuthStore.DeleteSessionsByUserID(id); delErr != nil {
		log.Printf("auth: delete-user: failed to revoke sessions for user %s: %v", id, delErr) // #nosec G706 -- User IDs are store-generated identifiers and the error is local runtime state.
	}

	if err := d.AuthStore.DeleteUser(id); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (d *Deps) handleUserSessions(w http.ResponseWriter, r *http.Request, userID string) {
	if userID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "missing user id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := d.AuthStore.ListSessionsByUserID(userID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
			return
		}
		if sessions == nil {
			sessions = []auth.Session{}
		}
		type sessionInfo struct {
			ID        string    `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		out := make([]sessionInfo, len(sessions))
		for i, s := range sessions {
			out[i] = sessionInfo{ID: s.ID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"sessions": out, "count": len(out)})

	case http.MethodDelete:
		if err := d.AuthStore.DeleteSessionsByUserID(userID); err != nil {
			log.Printf("auth: revoke-sessions: failed to delete sessions for user %s: %v", userID, err) // #nosec G706 -- User IDs are store-generated identifiers and the error is local runtime state.
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke sessions")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"revoked": true})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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

// ValidateMobileOIDCRedirectURI enforces the native app's single registered
// callback. No caller-provided custom scheme, host, path, query, or fragment is
// accepted, so the public start endpoint cannot become an open redirect.
func ValidateMobileOIDCRedirectURI(raw string) (string, error) {
	if strings.TrimSpace(raw) != MobileOIDCRedirectURI {
		return "", fmt.Errorf("redirect_uri must exactly match %s", MobileOIDCRedirectURI)
	}
	return MobileOIDCRedirectURI, nil
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
