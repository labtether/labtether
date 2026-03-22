package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// BootstrapSetupRequest holds the payload for initial admin setup.
type BootstrapSetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- Bootstrap request carries runtime credential material.
}

// HandleAuthBootstrapStatus handles GET /auth/bootstrap/status.
func (d *Deps) HandleAuthBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}

	setupRequired, err := AuthBootstrapSetupRequired(d.AuthStore)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to determine bootstrap status")
		return
	}

	suggestedUsername := DefaultBootstrapAdminUsername
	if configured, usernameErr := ConfiguredBootstrapAdminUsername(); usernameErr == nil {
		suggestedUsername = configured
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"setup_required":     setupRequired,
		"suggested_username": suggestedUsername,
	})
}

// HandleAuthBootstrapSetup handles POST /auth/bootstrap.
func (d *Deps) HandleAuthBootstrapSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	if !d.ValidateOwnerTokenRequest(r) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "bootstrap setup requires service authorization")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.bootstrap.setup", 10, time.Minute) {
		return
	}

	var req BootstrapSetupRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid setup payload")
		return
	}

	req.Username = NormalizeUsername(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if err := ValidateLoginRequest(LoginRequest{Username: req.Username, Password: req.Password}); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, weak := WeakPasswords[req.Password]; weak {
		servicehttp.WriteError(w, http.StatusBadRequest, "choose a stronger password")
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
	user, created, err := d.AuthStore.BootstrapFirstUser(req.Username, hash)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create bootstrap user")
		return
	}
	if !created {
		servicehttp.WriteError(w, http.StatusConflict, "setup already completed")
		return
	}

	d.CompleteLogin(w, user)
}
