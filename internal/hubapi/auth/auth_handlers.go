package auth

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

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
	if !d.EnforceRateLimit(w, r, "auth.login", 10, time.Minute) {
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

	user, ok, err := d.AuthStore.GetUserByUsername(req.Username)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if !ok || strings.TrimSpace(strings.ToLower(user.AuthProvider)) != "local" || !auth.CheckPassword(req.Password, user.PasswordHash) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check if user has 2FA enabled
	if user.TOTPSecret != "" && user.TOTPVerifiedAt != nil {
		challengeToken := d.ChallengeStore.Create(user.ID)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"requires_2fa":    true,
			"challenge_token": challengeToken,
		})
		return
	}

	d.CompleteLogin(w, user)
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
		}
	}

	auth.ClearSessionCookie(w, d.TLSEnabled)
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

	if delErr := d.AuthStore.DeleteSessionsByUserID(userID); delErr != nil {
		log.Printf("auth: delete-account: failed to revoke sessions for user %s: %v", userID, delErr) // #nosec G706 -- User IDs are store-generated identifiers.
	}

	if err := d.AuthStore.DeleteUser(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}

	auth.ClearSessionCookie(w, d.TLSEnabled)
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
