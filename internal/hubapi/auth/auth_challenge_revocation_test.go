package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	coreauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

const revocationPassword = "challenge-before-reset-password"
const replacementPassword = "challenge-after-reset-password"
const revocationRecoveryCode = "12345678-abcdef01"

type challengeRevocationFixture struct {
	deps   *Deps
	store  *persistence.MemoryAuthStore
	user   coreauth.User
	secret string
}

func newChallengeRevocationFixture(t *testing.T) challengeRevocationFixture {
	t.Helper()
	store := persistence.NewMemoryAuthStore()
	hash, err := coreauth.HashPassword(revocationPassword)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUserWithRole("challenge-owner", hash, coreauth.RoleViewer, "local", "")
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		AuthStore: store, ChallengeStore: coreauth.NewChallengeStore(),
		TOTPEncryptionKey: make([]byte, 32), EnforceRateLimit: testutil.NoopRateLimit,
		UserIDFromContext: func(context.Context) string { return user.ID },
	}
	secret, _, err := coreauth.GenerateTOTPSecret(user.Username, "LabTether")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := coreauth.EncryptTOTPSecret(secret, deps.TOTPEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := coreauth.HashRecoveryCode(revocationRecoveryCode)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := json.Marshal([]string{recovery})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetUserTOTPSecret(user.ID, encrypted); err != nil {
		t.Fatal(err)
	}
	if err = store.ConfirmUserTOTP(user.ID, string(codes)); err != nil {
		t.Fatal(err)
	}
	return challengeRevocationFixture{deps: deps, store: store, user: user, secret: secret}
}

func TestPendingTwoFactorChallengesAreRevokedWithCredentialsAndSessions(t *testing.T) {
	for _, operation := range []string{"password change", "admin reset", "revoke sessions", "disable TOTP", "replace recovery codes", "delete account", "admin delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newChallengeRevocationFixture(t)
			challenge := passwordChallenge(t, f.deps, f.user.Username, revocationPassword)
			// A different user's challenge must remain usable after this mutation.
			other, err := f.store.CreateUserWithRole("unrelated-owner", "unused", coreauth.RoleViewer, "local", "")
			if err != nil {
				t.Fatal(err)
			}
			otherChallenge := f.deps.ChallengeStore.Create(other.ID)
			const currentToken = "exact-current-session"
			for _, token := range []string{currentToken, "other-session"} {
				if _, err := f.store.CreateAuthSession(f.user.ID, coreauth.HashToken(token), time.Now().Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			var rec *httptest.ResponseRecorder
			wantStatus := http.StatusOK
			switch operation {
			case "password change":
				rec = passwordChangeRequest(t, f.deps, currentToken, revocationPassword, replacementPassword)
			case "admin reset":
				rec = performJSONRequest(t, http.MethodPatch, "/auth/users/"+f.user.ID, map[string]string{"password": replacementPassword}, f.deps.HandleAuthUserActions)
			case "revoke sessions":
				rec = performJSONRequest(t, http.MethodDelete, "/auth/users/"+f.user.ID+"/sessions", nil, f.deps.HandleAuthUserActions)
			case "disable TOTP":
				rec = performJSONRequest(t, http.MethodDelete, "/auth/2fa", map[string]string{"code": currentTOTP(t, f.secret)}, f.deps.Handle2FADisable)
			case "replace recovery codes":
				rec = performJSONRequest(t, http.MethodPost, "/auth/2fa/recovery-codes", map[string]string{"code": currentTOTP(t, f.secret)}, f.deps.Handle2FARecoveryCodes)
			case "delete account":
				rec = performJSONRequest(t, http.MethodDelete, "/auth/account", nil, f.deps.HandleDeleteOwnAccount)
			case "admin delete":
				f.deps.UserIDFromContext = func(context.Context) string { return "different-admin" }
				rec = performJSONRequest(t, http.MethodDelete, "/auth/users/"+f.user.ID, nil, f.deps.HandleAuthUserActions)
				wantStatus = http.StatusNoContent
			}
			if rec.Code != wantStatus {
				t.Fatalf("mutation status=%d, body=%s", rec.Code, rec.Body.String())
			}
			rejected := performJSONRequest(t, http.MethodPost, "/auth/login/2fa", map[string]string{
				"challenge_token": challenge, "code": currentTOTP(t, f.secret),
			}, f.deps.HandleLogin2FA)
			if rejected.Code != http.StatusUnauthorized || hasSessionCookie(rejected) || !bytes.Contains(rejected.Body.Bytes(), []byte("invalid or expired challenge")) {
				t.Fatalf("old challenge status=%d, body=%s", rejected.Code, rejected.Body.String())
			}
			if userID, ok := f.deps.ChallengeStore.Validate(otherChallenge); !ok || userID != other.ID {
				t.Fatal("unrelated user's challenge was revoked")
			}
			sessions, err := f.store.ListSessionsByUserID(f.user.ID)
			wantSessions := 0
			if operation == "password change" {
				wantSessions = 1
			}
			if err != nil || len(sessions) != wantSessions {
				t.Fatalf("remaining sessions=%d, want=%d, err=%v", len(sessions), wantSessions, err)
			}
			if wantSessions == 1 && sessions[0].TokenHash != coreauth.HashToken(currentToken) {
				t.Fatal("password change retained a different session")
			}
		})
	}
}

type stalePasswordReadStore struct {
	persistence.AuthStore
	read    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *stalePasswordReadStore) GetUserByUsername(name string) (coreauth.User, bool, error) {
	user, ok, err := s.AuthStore.GetUserByUsername(name)
	s.once.Do(func() { close(s.read); <-s.release })
	return user, ok, err
}

func TestPasswordLoginCannotIssueChallengeFromReadBeforeReset(t *testing.T) {
	f := newChallengeRevocationFixture(t)
	blocked := &stalePasswordReadStore{AuthStore: f.store, read: make(chan struct{}), release: make(chan struct{})}
	f.deps.AuthStore = blocked
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer release()
	login := startAuthRequest(t, f.deps.HandleAuthLogin, http.MethodPost, "/auth/login", LoginRequest{Username: f.user.Username, Password: revocationPassword})
	waitAuthSignal(t, blocked.read)
	reset := waitAuthResponse(t, startAuthRequest(t, f.deps.HandleAuthUserActions, http.MethodPatch, "/auth/users/"+f.user.ID, map[string]string{"password": replacementPassword}))
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status=%d, body=%s", reset.Code, reset.Body.String())
	}
	release()
	result := waitAuthResponse(t, login)
	if result.Code != http.StatusUnauthorized || hasSessionCookie(result) {
		t.Fatalf("old password login after reset status=%d, body=%s", result.Code, result.Body.String())
	}
	if fresh := completeChallenge(t, f.deps, f.user.Username, replacementPassword, currentTOTP(t, f.secret)); fresh.Code != http.StatusOK || !hasSessionCookie(fresh) {
		t.Fatalf("fresh password login status=%d", fresh.Code)
	}
}

type pendingAuthSessionStore struct {
	persistence.AuthStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *pendingAuthSessionStore) CreateAuthSession(userID, hash string, expires time.Time) (coreauth.Session, error) {
	s.once.Do(func() { close(s.entered); <-s.release })
	return s.AuthStore.CreateAuthSession(userID, hash, expires)
}

func TestSessionRevocationWaitsForConsumedTwoFactorLogin(t *testing.T) {
	for _, factor := range []string{"TOTP", "recovery"} {
		t.Run(factor, func(t *testing.T) {
			f := newChallengeRevocationFixture(t)
			challenge := passwordChallenge(t, f.deps, f.user.Username, revocationPassword)
			blocked := &pendingAuthSessionStore{AuthStore: f.store, entered: make(chan struct{}), release: make(chan struct{})}
			f.deps.AuthStore = blocked
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
			defer release()
			code := currentTOTP(t, f.secret)
			if factor == "recovery" {
				code = revocationRecoveryCode
			}
			login := startAuthRequest(t, f.deps.HandleLogin2FA, http.MethodPost, "/auth/login/2fa", map[string]string{"challenge_token": challenge, "code": code})
			waitAuthSignal(t, blocked.entered)
			revoke := startAuthRequest(t, f.deps.HandleAuthUserActions, http.MethodDelete, "/auth/users/"+f.user.ID+"/sessions", nil)
			var early *httptest.ResponseRecorder
			select {
			case early = <-revoke:
			case <-time.After(100 * time.Millisecond):
			}
			release()
			loginResult := waitAuthResponse(t, login)
			revokeResult := early
			if revokeResult == nil {
				revokeResult = waitAuthResponse(t, revoke)
			}
			if early != nil {
				t.Fatal("session revocation returned while consumed challenge could still insert a session")
			}
			if loginResult.Code != http.StatusOK || revokeResult.Code != http.StatusOK {
				t.Fatalf("login=%d, revoke=%d", loginResult.Code, revokeResult.Code)
			}
			sessions, err := f.store.ListSessionsByUserID(f.user.ID)
			if err != nil || len(sessions) != 0 {
				t.Fatalf("late session survived revocation: count=%d, err=%v", len(sessions), err)
			}
			for _, cookie := range loginResult.Result().Cookies() {
				if cookie.Name == coreauth.SessionCookieName {
					if _, valid, err := f.store.ValidateSession(coreauth.HashToken(cookie.Value)); err != nil || valid {
						t.Fatalf("revoked login cookie valid=%t, err=%v", valid, err)
					}
				}
			}
		})
	}
}

func startAuthRequest(t *testing.T, handler http.HandlerFunc, method, path string, payload any) <-chan *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	result := make(chan *httptest.ResponseRecorder, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("auth fixture left a handler running")
		}
	})
	go func() {
		defer close(finished)
		rec := httptest.NewRecorder()
		handler(rec, req)
		result <- rec
	}()
	return result
}

func waitAuthSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("auth fixture did not reach its held store operation")
	}
}

func waitAuthResponse(t *testing.T, result <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-result:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("auth handler did not finish after its store operation was released")
		return nil
	}
}
