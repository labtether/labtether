package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleLogin2FA handles POST /auth/login/2fa.
func (d *Deps) HandleLogin2FA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.login.2fa", 10, time.Minute) {
		return
	}

	var req struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}

	req.ChallengeToken = strings.TrimSpace(req.ChallengeToken)
	req.Code = strings.TrimSpace(req.Code)
	if req.ChallengeToken == "" || req.Code == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "challenge_token and code are required")
		return
	}

	// Consume the challenge token atomically. If the TOTP code turns out to be
	// wrong, the token is already spent and the user must re-login. This
	// prevents TOCTOU races where two concurrent requests both pass Validate
	// before either calls Consume.
	userID, ok := d.ChallengeStore.Consume(req.ChallengeToken)
	if !ok {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid or expired challenge")
		return
	}

	user, userOk, err := d.AuthStore.GetUserByID(userID)
	if err != nil || !userOk {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid challenge")
		return
	}

	secret, err := auth.DecryptTOTPSecret(user.TOTPSecret, d.TOTPEncryptionKey)
	if err != nil {
		log.Printf("2fa: failed to decrypt TOTP secret for user %s: %v", userID, err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "2fa verification failed")
		return
	}

	if auth.ValidateTOTPCode(secret, req.Code) {
		d.CompleteLogin(w, user)
		return
	}

	if d.TryRecoveryCode(user, req.Code) {
		d.CompleteLogin(w, user)
		return
	}

	servicehttp.WriteError(w, http.StatusUnauthorized, "invalid code")
}

// CompleteLogin creates a session and sets the session cookie.
func (d *Deps) CompleteLogin(w http.ResponseWriter, user auth.User) {
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
	auth.SetSessionCookie(w, raw, auth.SessionDuration, d.TLSEnabled)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user":       auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
		"session_id": session.ID,
		"expires_at": session.ExpiresAt,
	})
}

// TryRecoveryCode attempts to consume a recovery code for 2FA.
func (d *Deps) TryRecoveryCode(user auth.User, code string) bool {
	consumed, err := d.AuthStore.ConsumeRecoveryCode(user.ID, code)
	if err != nil {
		log.Printf("2fa: recovery code consumption error for user %s: %v", user.ID, err)
		return false
	}
	return consumed
}

// Handle2FASetup handles POST /auth/2fa/setup.
func (d *Deps) Handle2FASetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil || !ok {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user.TOTPSecret != "" && user.TOTPVerifiedAt != nil {
		servicehttp.WriteError(w, http.StatusConflict, "2FA is already enabled; disable it first")
		return
	}

	secret, uri, err := auth.GenerateTOTPSecret(user.Username, "LabTether")
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate TOTP secret")
		return
	}

	encrypted, err := auth.EncryptTOTPSecret(secret, d.TOTPEncryptionKey)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt TOTP secret")
		return
	}

	if err := d.AuthStore.SetUserTOTPSecret(userID, encrypted); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save TOTP secret")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"secret": secret,
		"uri":    uri,
	})
}

// Handle2FAVerify handles POST /auth/2fa/verify.
func (d *Deps) Handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.2fa.verify", 10, time.Minute) {
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil || !ok {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user.TOTPSecret == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "call /auth/2fa/setup first")
		return
	}
	if user.TOTPVerifiedAt != nil {
		servicehttp.WriteError(w, http.StatusConflict, "2FA is already verified")
		return
	}

	secret, err := auth.DecryptTOTPSecret(user.TOTPSecret, d.TOTPEncryptionKey)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to decrypt secret")
		return
	}

	if !auth.ValidateTOTPCode(secret, req.Code) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	rawCodes := auth.GenerateRecoveryCodes(8)
	hashedCodes := make([]string, len(rawCodes))
	for i, code := range rawCodes {
		hash, err := auth.HashRecoveryCode(code)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate recovery codes")
			return
		}
		hashedCodes[i] = hash
	}
	codesJSON, _ := json.Marshal(hashedCodes)

	if err := d.AuthStore.ConfirmUserTOTP(userID, string(codesJSON)); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to confirm 2FA")
		return
	}
	if err := d.AuthStore.DeleteSessionsByUserID(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke active sessions")
		return
	}
	auth.ClearSessionCookie(w, d.TLSEnabled)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "enabled",
		"recovery_codes": rawCodes,
	})
}

// Handle2FADisable handles DELETE /auth/2fa.
func (d *Deps) Handle2FADisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.2fa.disable", 10, time.Minute) {
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "code is required to disable 2FA")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil || !ok {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user.TOTPSecret == "" || user.TOTPVerifiedAt == nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "2FA is not enabled")
		return
	}

	secret, err := auth.DecryptTOTPSecret(user.TOTPSecret, d.TOTPEncryptionKey)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to verify code")
		return
	}

	if !auth.ValidateTOTPCode(secret, req.Code) && !d.TryRecoveryCode(user, req.Code) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	if err := d.AuthStore.ClearUserTOTP(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to disable 2FA")
		return
	}
	if err := d.AuthStore.DeleteSessionsByUserID(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke active sessions")
		return
	}
	auth.ClearSessionCookie(w, d.TLSEnabled)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

// Handle2FARecoveryCodes handles POST /auth/2fa/recovery-codes.
func (d *Deps) Handle2FARecoveryCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "auth.2fa.recovery_codes", 10, time.Minute) {
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	userID := d.UserIDFromContext(r.Context())
	user, ok, err := d.AuthStore.GetUserByID(userID)
	if err != nil || !ok {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user.TOTPSecret == "" || user.TOTPVerifiedAt == nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "2FA is not enabled")
		return
	}

	secret, err := auth.DecryptTOTPSecret(user.TOTPSecret, d.TOTPEncryptionKey)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to verify code")
		return
	}

	if !auth.ValidateTOTPCode(secret, strings.TrimSpace(req.Code)) && !d.TryRecoveryCode(user, strings.TrimSpace(req.Code)) {
		servicehttp.WriteError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	rawCodes := auth.GenerateRecoveryCodes(8)
	hashedCodes := make([]string, len(rawCodes))
	for i, code := range rawCodes {
		hash, hashErr := auth.HashRecoveryCode(code)
		if hashErr != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to generate recovery codes")
			return
		}
		hashedCodes[i] = hash
	}
	codesJSON, _ := json.Marshal(hashedCodes)

	if err := d.AuthStore.UpdateUserRecoveryCodes(userID, string(codesJSON)); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update recovery codes")
		return
	}
	if err := d.AuthStore.DeleteSessionsByUserID(userID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke active sessions")
		return
	}
	auth.ClearSessionCookie(w, d.TLSEnabled)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"recovery_codes": rawCodes,
	})
}
