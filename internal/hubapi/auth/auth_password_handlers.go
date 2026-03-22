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

// HandleChangePassword handles POST /auth/me/password.
// Allows authenticated users to change their own password.
// Requires current_password + new_password in the request body.
func (d *Deps) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.password.change", 5, time.Minute) {
		return
	}

	userID := d.UserIDFromContext(r.Context())
	if userID == "" {
		servicehttp.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.CurrentPassword = strings.TrimSpace(req.CurrentPassword)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	if req.CurrentPassword == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "current_password is required")
		return
	}
	if len(req.NewPassword) < MinPasswordLength {
		servicehttp.WriteError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > 256 {
		servicehttp.WriteError(w, http.StatusBadRequest, "new password exceeds max length 256")
		return
	}

	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if strings.TrimSpace(strings.ToLower(user.AuthProvider)) != "local" {
		servicehttp.WriteError(w, http.StatusBadRequest, "password change is only available for local accounts")
		return
	}
	if !auth.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := d.AuthStore.UpdateUserPasswordHash(userID, newHash); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Revoke all sessions except the current one so other devices are signed out.
	if token := auth.ExtractSessionToken(r); token != "" {
		currentHash := auth.HashToken(token)
		sessions, listErr := d.AuthStore.ListSessionsByUserID(userID)
		if listErr == nil {
			for _, s := range sessions {
				if s.TokenHash != currentHash {
					if delErr := d.AuthStore.DeleteSession(s.ID); delErr != nil {
						log.Printf("auth: password-change: failed to revoke session %s for user %s: %v", s.ID, userID, delErr) // #nosec G706 -- Session and user IDs are store-generated identifiers.
					}
				}
			}
		} else {
			log.Printf("auth: password-change: failed to list sessions for user %s: %v", userID, listErr) // #nosec G706 -- User IDs are store-generated identifiers.
		}
	} else {
		// No current token identifiable — revoke all sessions.
		if delErr := d.AuthStore.DeleteSessionsByUserID(userID); delErr != nil {
			log.Printf("auth: password-change: failed to revoke all sessions for user %s: %v", userID, delErr) // #nosec G706 -- User IDs are store-generated identifiers.
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}
