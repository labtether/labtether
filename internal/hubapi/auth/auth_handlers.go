package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	loginPerClientLimit = 30
	loginGlobalLimit    = 120
)

func (d *Deps) loginClientIP(r *http.Request) string {
	if d.ClientIP != nil {
		if ip := strings.TrimSpace(d.ClientIP(r)); ip != "" {
			return ip
		}
	}
	return shared.RequestClientKey(r)
}

func (d *Deps) requestIsSecure(r *http.Request) bool {
	if d == nil || d.RequestIsSecure == nil {
		return true
	}
	return d.RequestIsSecure(r)
}

// HandleAuthLogin handles POST /auth/login.
func (d *Deps) HandleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	// Per-client first: a single client that hits its own ceiling is rejected
	// before it consumes the shared global budget, so it cannot lock everyone
	// else out of logging in.
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.login.client:"+d.loginClientIP(r), loginPerClientLimit, time.Minute) {
		return
	}
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, "auth.login.global", loginGlobalLimit, time.Minute) {
		return
	}

	var req LoginRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid login payload")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := ValidateLoginRequest(req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	accountBucket := loginAccountBucket(req.Username)
	if !d.EnforceRateLimit(w, r, accountBucket, 10, time.Minute) {
		return
	}
	if d.EnforceRateLimitGlobal != nil && !d.EnforceRateLimitGlobal(w, accountBucket+":global", 20, time.Minute) {
		return
	}

	user, ok, err := d.AuthStore.GetUserByUsername(req.Username)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if ok {
		unlock := d.authChallenges().LockUser(user.ID)
		defer unlock()
		// The username lookup may have raced a password reset or revocation.
		// Read the credential again inside the same boundary as session creation.
		user, ok, err = d.AuthStore.GetUserByID(user.ID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
			return
		}
	}
	localUser := ok && strings.EqualFold(strings.TrimSpace(user.AuthProvider), "local")
	passwordOK := false
	if localUser {
		passwordOK = auth.CheckPassword(req.Password, user.PasswordHash)
	} else {
		passwordOK = auth.CheckDummyPassword(req.Password)
	}
	if !localUser || !passwordOK {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check if user has 2FA enabled
	if user.TOTPSecret != "" && user.TOTPVerifiedAt != nil {
		challengeToken := d.authChallenges().Create(user.ID)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"requires_2fa":    true,
			"challenge_token": challengeToken,
		})
		return
	}

	d.CompleteLogin(w, r, user)
}

func loginAccountBucket(username string) string {
	normalized := strings.ToLower(strings.TrimSpace(username))
	digest := sha256.Sum256([]byte(normalized))
	return "auth.login.account:" + hex.EncodeToString(digest[:8])
}

// HandleAuthLogout handles POST /auth/logout.
func (d *Deps) HandleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := auth.ExtractSessionToken(r)
	if token != "" && d.AuthStore != nil {
		hashed := auth.HashToken(token)
		session, ok, err := d.AuthStore.ValidateSession(hashed)
		if err == nil && ok {
			if delErr := d.AuthStore.DeleteSession(session.ID); delErr != nil {
				log.Printf("auth: logout: failed to delete session %s: %v", session.ID, delErr) // #nosec G706 -- Session IDs are store-generated identifiers.
			}
			d.revokeStreams(shared.StreamSessionKey(session.ID))
		}
	}

	auth.ClearSessionCookie(w)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

// HandleDeleteOwnAccount handles DELETE /auth/account.
// Allows authenticated users to delete their own account (App Store guideline 5.1.1(v)).
// Owner accounts cannot self-delete.
func (d *Deps) HandleDeleteOwnAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	challenges := d.authChallenges()
	unlock := challenges.LockUser(userID)
	defer unlock()
	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up account")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "account not found")
		return
	}
	if auth.NormalizeRole(user.Role) == auth.RoleOwner {
		servicehttp.WriteError(w, http.StatusForbidden, "owner account cannot be self-deleted; transfer ownership first")
		return
	}

	challenges.RevokeForUser(userID)
	if delErr := d.AuthStore.DeleteSessionsByUserID(userID); delErr != nil {
		log.Printf("auth: delete-account: failed to revoke sessions for user %s: %v", userID, delErr) // #nosec G706 -- User IDs are store-generated identifiers.
	}

	if err := d.AuthStore.DeleteUser(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}
	d.revokeUserStreams(r.Context(), userID)

	auth.ClearSessionCookie(w)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleAuthMe handles GET /auth/me.
func (d *Deps) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if d.AuthStore != nil {
		user, ok, err := d.AuthStore.GetUserByID(userID)
		if err == nil && ok {
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
				"user":         auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
				"totp_enabled": user.TOTPSecret != "" && user.TOTPVerifiedAt != nil,
			})
			return
		}
	}

	// API keys are not user rows. Report the key's own role so clients do not
	// show controls the key cannot use.
	if keyID, isKey := strings.CutPrefix(userID, "apikey:"); isKey {
		name := "API key"
		if d.APIKeyStore != nil {
			if key, found, err := d.APIKeyStore.GetAPIKey(r.Context(), keyID); err == nil && found && strings.TrimSpace(key.Name) != "" {
				name = key.Name
			}
		}
		role := auth.RoleViewer
		if d.UserRoleFromContext != nil {
			role = auth.NormalizeRole(d.UserRoleFromContext(r.Context()))
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"user": map[string]string{"id": userID, "username": name, "role": role},
		})
		return
	}

	// Fallback for bearer token auth (userID="owner") or missing store
	username := DefaultBootstrapAdminUsername
	if configured, err := ConfiguredBootstrapAdminUsername(); err == nil {
		username = configured
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user": map[string]string{
			"id":       userID,
			"username": username,
			"role":     auth.RoleOwner,
		},
	})
}
