package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/pquerna/otp/totp"
)

func TestTwoFactorLifecycleAndSingleUseRecovery(t *testing.T) {
	store := persistence.NewMemoryAuthStore()
	password := "viewer-secure-password-123!"
	passwordHash, err := coreauth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := store.CreateUserWithRole("viewer-2fa", passwordHash, coreauth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	deps := &Deps{
		AuthStore:         store,
		OIDCRef:           &OIDCProviderRef{},
		ChallengeStore:    coreauth.NewChallengeStore(),
		TOTPEncryptionKey: make([]byte, 32),
		EnforceRateLimit:  testutil.NoopRateLimit,
		UserIDFromContext: func(context.Context) string { return user.ID },
	}

	if _, err := store.CreateAuthSession(user.ID, "pre-enrollment-session", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create pre-enrollment session: %v", err)
	}

	setup := performJSONRequest(t, http.MethodPost, "/auth/2fa/setup", nil, deps.Handle2FASetup)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setup.Code, setup.Body.String())
	}
	setupPayload := decodeJSONMap(t, setup)
	secret, _ := setupPayload["secret"].(string)
	if secret == "" {
		t.Fatal("setup secret missing")
	}
	persisted, ok, err := store.GetUserByID(user.ID)
	if err != nil || !ok {
		t.Fatalf("load setup user: ok=%t err=%v", ok, err)
	}
	if persisted.TOTPSecret == "" || persisted.TOTPSecret == secret {
		t.Fatal("TOTP secret was not persisted as ciphertext")
	}

	code, err := totp.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate TOTP: %v", err)
	}
	wrongCode := code[:len(code)-1] + string('0'+rune((int(code[len(code)-1]-'0')+1)%10))
	rejected := performJSONRequest(t, http.MethodPost, "/auth/2fa/verify", map[string]any{"code": wrongCode}, deps.Handle2FAVerify)
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("invalid enrollment code status = %d", rejected.Code)
	}

	verified := performJSONRequest(t, http.MethodPost, "/auth/2fa/verify", map[string]any{"code": code}, deps.Handle2FAVerify)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", verified.Code, verified.Body.String())
	}
	verifiedPayload := decodeJSONMap(t, verified)
	recoveryCodes := stringSliceField(t, verifiedPayload, "recovery_codes", 8)
	sessions, err := store.ListSessionsByUserID(user.ID)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("verification did not revoke prior sessions: count=%d err=%v", len(sessions), err)
	}

	challenge := passwordChallenge(t, deps, user.Username, password)
	completed := performJSONRequest(t, http.MethodPost, "/auth/login/2fa", map[string]any{
		"challenge_token": challenge,
		"code":            currentTOTP(t, secret),
	}, deps.HandleLogin2FA)
	if completed.Code != http.StatusOK || !hasSessionCookie(completed) {
		t.Fatalf("TOTP challenge status = %d, cookie=%t", completed.Code, hasSessionCookie(completed))
	}
	replay := performJSONRequest(t, http.MethodPost, "/auth/login/2fa", map[string]any{
		"challenge_token": challenge,
		"code":            currentTOTP(t, secret),
	}, deps.HandleLogin2FA)
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("challenge replay status = %d", replay.Code)
	}

	recoveryLogin := completeChallenge(t, deps, user.Username, password, recoveryCodes[0])
	if recoveryLogin.Code != http.StatusOK || !hasSessionCookie(recoveryLogin) {
		t.Fatalf("recovery login status = %d", recoveryLogin.Code)
	}
	reusedRecovery := completeChallenge(t, deps, user.Username, password, recoveryCodes[0])
	if reusedRecovery.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code status = %d", reusedRecovery.Code)
	}

	regenerated := performJSONRequest(t, http.MethodPost, "/auth/2fa/recovery-codes", map[string]any{
		"code": currentTOTP(t, secret),
	}, deps.Handle2FARecoveryCodes)
	if regenerated.Code != http.StatusOK {
		t.Fatalf("recovery regeneration status = %d, body = %s", regenerated.Code, regenerated.Body.String())
	}
	replacementCodes := stringSliceField(t, decodeJSONMap(t, regenerated), "recovery_codes", 8)
	if replacementCodes[0] == recoveryCodes[0] {
		t.Fatal("recovery regeneration returned the old code set")
	}
	if oldAfterRegeneration := completeChallenge(t, deps, user.Username, password, recoveryCodes[1]); oldAfterRegeneration.Code != http.StatusUnauthorized {
		t.Fatalf("old recovery code after regeneration status = %d", oldAfterRegeneration.Code)
	}
	if replacement := completeChallenge(t, deps, user.Username, password, replacementCodes[0]); replacement.Code != http.StatusOK {
		t.Fatalf("replacement recovery code status = %d", replacement.Code)
	}

	disabled := performJSONRequest(t, http.MethodDelete, "/auth/2fa", map[string]any{
		"code": currentTOTP(t, secret),
	}, deps.Handle2FADisable)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disabled.Code, disabled.Body.String())
	}
	persisted, ok, err = store.GetUserByID(user.ID)
	if err != nil || !ok || persisted.TOTPSecret != "" || persisted.TOTPVerifiedAt != nil || persisted.TOTPRecoveryCodes != "" {
		t.Fatalf("2FA state remained after disable: ok=%t err=%v", ok, err)
	}
	plainLogin := performJSONRequest(t, http.MethodPost, "/auth/login", LoginRequest{
		Username: user.Username,
		Password: password,
	}, deps.HandleAuthLogin)
	if plainLogin.Code != http.StatusOK || !hasSessionCookie(plainLogin) {
		t.Fatalf("post-disable password login status = %d, cookie=%t", plainLogin.Code, hasSessionCookie(plainLogin))
	}
	if payload := decodeJSONMap(t, plainLogin); payload["requires_2fa"] == true {
		t.Fatal("post-disable password login still required 2FA")
	}
}

func TestTwoFactorMutationsRetainVerifiedCurrentSession(t *testing.T) {
	store := persistence.NewMemoryAuthStore()
	user, err := store.CreateUserWithRole("viewer-2fa-session", "unused", coreauth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	deps := &Deps{
		AuthStore:         store,
		ChallengeStore:    coreauth.NewChallengeStore(),
		TOTPEncryptionKey: make([]byte, 32),
		EnforceRateLimit:  testutil.NoopRateLimit,
		UserIDFromContext: func(context.Context) string { return user.ID },
	}
	setup := performJSONRequest(t, http.MethodPost, "/auth/2fa/setup", nil, deps.Handle2FASetup)
	secret, _ := decodeJSONMap(t, setup)["secret"].(string)
	if setup.Code != http.StatusOK || secret == "" {
		t.Fatalf("setup status = %d", setup.Code)
	}

	const currentToken = "verified-current-session"
	currentHash := coreauth.HashToken(currentToken)
	if _, err := store.CreateAuthSession(user.ID, currentHash, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	createOtherSession := func(label string) {
		t.Helper()
		if _, createErr := store.CreateAuthSession(user.ID, coreauth.HashToken(label), time.Now().UTC().Add(time.Hour)); createErr != nil {
			t.Fatalf("create other session: %v", createErr)
		}
	}
	assertOnlyCurrent := func() {
		t.Helper()
		sessions, listErr := store.ListSessionsByUserID(user.ID)
		if listErr != nil || len(sessions) != 1 || sessions[0].TokenHash != currentHash {
			t.Fatalf("sessions = %#v, err=%v; want only current", sessions, listErr)
		}
	}

	createOtherSession("other-before-verify")
	verified := performJSONRequestWithCookie(t, http.MethodPost, "/auth/2fa/verify", map[string]any{
		"code": currentTOTP(t, secret),
	}, currentToken, deps.Handle2FAVerify)
	if verified.Code != http.StatusOK || clearsSessionCookie(verified) {
		t.Fatalf("verify status = %d, cleared current=%t", verified.Code, clearsSessionCookie(verified))
	}
	assertOnlyCurrent()

	createOtherSession("other-before-regeneration")
	regenerated := performJSONRequestWithCookie(t, http.MethodPost, "/auth/2fa/recovery-codes", map[string]any{
		"code": currentTOTP(t, secret),
	}, currentToken, deps.Handle2FARecoveryCodes)
	if regenerated.Code != http.StatusOK || clearsSessionCookie(regenerated) {
		t.Fatalf("regeneration status = %d, cleared current=%t", regenerated.Code, clearsSessionCookie(regenerated))
	}
	assertOnlyCurrent()

	createOtherSession("other-before-disable")
	disabled := performJSONRequestWithCookie(t, http.MethodDelete, "/auth/2fa", map[string]any{
		"code": currentTOTP(t, secret),
	}, currentToken, deps.Handle2FADisable)
	if disabled.Code != http.StatusOK || clearsSessionCookie(disabled) {
		t.Fatalf("disable status = %d, cleared current=%t", disabled.Code, clearsSessionCookie(disabled))
	}
	assertOnlyCurrent()
}

func TestTwoFactorVerificationDoesNotCommitWhenSessionRevocationFails(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	user, err := store.CreateUserWithRole("viewer-2fa-failure", "unused", coreauth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	deps.UserIDFromContext = func(context.Context) string { return user.ID }
	setup := performJSONRequest(t, http.MethodPost, "/auth/2fa/setup", nil, deps.Handle2FASetup)
	secret, _ := decodeJSONMap(t, setup)["secret"].(string)
	if setup.Code != http.StatusOK || secret == "" {
		t.Fatalf("setup status = %d", setup.Code)
	}

	const currentToken = "current-2fa-failure-session"
	if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken(currentToken), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	if _, err := store.CreateAuthSession(user.ID, coreauth.HashToken("other-2fa-failure-session"), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	deps.AuthStore = failingSessionDeleteAuthStore{memAuthStore: store}

	verified := performJSONRequestWithCookie(t, http.MethodPost, "/auth/2fa/verify", map[string]any{
		"code": currentTOTP(t, secret),
	}, currentToken, deps.Handle2FAVerify)
	if verified.Code != http.StatusInternalServerError {
		t.Fatalf("verify status = %d, want 500", verified.Code)
	}
	persisted, ok, err := store.GetUserByID(user.ID)
	if err != nil || !ok || persisted.TOTPVerifiedAt != nil || persisted.TOTPRecoveryCodes != "" {
		t.Fatalf("2FA committed despite revocation failure: ok=%t err=%v", ok, err)
	}
}

func performJSONRequest(
	t *testing.T,
	method string,
	path string,
	payload any,
	handler http.HandlerFunc,
) *httptest.ResponseRecorder {
	return performJSONRequestWithCookie(t, method, path, payload, "", handler)
}

func performJSONRequestWithCookie(
	t *testing.T,
	method string,
	path string,
	payload any,
	rawSessionToken string,
	handler http.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rawSessionToken != "" {
		req.AddCookie(&http.Cookie{Name: coreauth.SessionCookieName, Value: rawSessionToken})
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func stringSliceField(t *testing.T, payload map[string]any, field string, want int) []string {
	t.Helper()
	raw, ok := payload[field].([]any)
	if !ok || len(raw) != want {
		t.Fatalf("%s length = %d, want %d", field, len(raw), want)
	}
	out := make([]string, len(raw))
	for i, value := range raw {
		out[i], ok = value.(string)
		if !ok || out[i] == "" {
			t.Fatalf("%s[%d] is invalid", field, i)
		}
	}
	return out
}

func currentTOTP(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate TOTP: %v", err)
	}
	return code
}

func passwordChallenge(t *testing.T, deps *Deps, username, password string) string {
	t.Helper()
	rec := performJSONRequest(t, http.MethodPost, "/auth/login", LoginRequest{Username: username, Password: password}, deps.HandleAuthLogin)
	if rec.Code != http.StatusOK {
		t.Fatalf("password challenge status = %d, body = %s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	if payload["requires_2fa"] != true {
		t.Fatal("password login did not require 2FA")
	}
	challenge, _ := payload["challenge_token"].(string)
	if challenge == "" {
		t.Fatal("challenge token missing")
	}
	return challenge
}

func completeChallenge(t *testing.T, deps *Deps, username, password, code string) *httptest.ResponseRecorder {
	t.Helper()
	return performJSONRequest(t, http.MethodPost, "/auth/login/2fa", map[string]any{
		"challenge_token": passwordChallenge(t, deps, username, password),
		"code":            code,
	}, deps.HandleLogin2FA)
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == coreauth.SessionCookieName && cookie.Value != "" {
			return true
		}
	}
	return false
}

func clearsSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == coreauth.SessionCookieName && cookie.MaxAge < 0 {
			return true
		}
	}
	return false
}
