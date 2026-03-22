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

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// --- in-memory AuthStore for tests ---

type memAuthStore struct {
	mu       sync.Mutex
	users    []auth.User
	sessions []auth.Session
	nextID   int
}

func newMemAuthStore() *memAuthStore {
	return &memAuthStore{users: []auth.User{}, sessions: []auth.Session{}}
}

func (s *memAuthStore) genID() string {
	s.nextID++
	return "id-" + string(rune('0'+s.nextID))
}

func (s *memAuthStore) GetUserByID(id string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.ID == id {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) GetUserByUsername(username string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == username {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.AuthProvider == provider && u.OIDCSubject == subject {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) ListUsers(limit int) ([]auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.users) {
		limit = len(s.users)
	}
	return append([]auth.User{}, s.users[:limit]...), nil
}

func (s *memAuthStore) BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return s.users[0], false, nil
	}
	u := auth.User{
		ID: s.genID(), Username: username, PasswordHash: passwordHash,
		Role: auth.RoleOwner, AuthProvider: "local",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.users = append(s.users, u)
	return u, true, nil
}

func (s *memAuthStore) CreateUser(username, passwordHash string) (auth.User, error) {
	return s.CreateUserWithRole(username, passwordHash, auth.RoleViewer, "local", "")
}

func (s *memAuthStore) CreateUserWithRole(username, passwordHash, role, provider, oidcSubject string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := auth.User{
		ID: s.genID(), Username: username, PasswordHash: passwordHash,
		Role: role, AuthProvider: provider, OIDCSubject: oidcSubject,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.users = append(s.users, u)
	return u, nil
}

func (s *memAuthStore) UpdateUserPasswordHash(id, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].PasswordHash = hash
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) UpdateUserRole(id, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].Role = role
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i] = s.users[len(s.users)-1]
			s.users = s.users[:len(s.users)-1]
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ListSessionsByUserID(userID string) ([]auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []auth.Session
	for _, sess := range s.sessions {
		if sess.UserID == userID {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *memAuthStore) SetUserTOTPSecret(id, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPSecret = secret
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ConfirmUserTOTP(id, codes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPVerifiedAt = &now
			s.users[i].TOTPRecoveryCodes = codes
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ClearUserTOTP(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPSecret = ""
			s.users[i].TOTPVerifiedAt = nil
			s.users[i].TOTPRecoveryCodes = ""
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) UpdateUserRecoveryCodes(id, codes string) error { return nil }
func (s *memAuthStore) ConsumeRecoveryCode(userID, code string) (bool, error) {
	return false, nil
}

func (s *memAuthStore) CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := auth.Session{
		ID: s.genID(), UserID: userID, TokenHash: tokenHash,
		ExpiresAt: expiresAt, CreatedAt: time.Now().UTC(),
	}
	s.sessions = append(s.sessions, sess)
	return sess, nil
}

func (s *memAuthStore) ValidateSession(tokenHash string) (auth.Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, sess := range s.sessions {
		if sess.TokenHash == tokenHash && sess.ExpiresAt.After(now) {
			return sess, true, nil
		}
	}
	return auth.Session{}, false, nil
}

func (s *memAuthStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		if s.sessions[i].ID == id {
			s.sessions[i] = s.sessions[len(s.sessions)-1]
			s.sessions = s.sessions[:len(s.sessions)-1]
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) DeleteSessionsByUserID(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []auth.Session
	for _, sess := range s.sessions {
		if sess.UserID != userID {
			kept = append(kept, sess)
		}
	}
	s.sessions = kept
	return nil
}

func (s *memAuthStore) DeleteExpiredSessions() (int64, error) { return 0, nil }

// --- test Deps factory ---

func newTestAuthDeps(t *testing.T) (*Deps, *memAuthStore) {
	t.Helper()
	store := newMemAuthStore()
	return &Deps{
		AuthStore:                 store,
		OIDCRef:                   &OIDCProviderRef{},
		TLSEnabled:               false,
		ChallengeStore:           auth.NewChallengeStore(),
		TOTPEncryptionKey:        make([]byte, 32),
		OIDCStates:               make(map[string]OIDCAuthState),
		EnforceRateLimit:         testutil.NoopRateLimit,
		ValidateOwnerTokenRequest: func(_ *http.Request) bool { return true },
		UserIDFromContext:         testutil.TestUserID,
		WrapAuth:                 testutil.NoopAuth,
		WrapAdmin:                testutil.NoopAuth,
	}, store
}

// --- Tests ---

func TestHandleAuthLoginRejectsGET(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthLogin(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleAuthLoginRejectsBadCredentials(t *testing.T) {
	deps, store := newTestAuthDeps(t)

	// Create a user with a known password.
	hash, _ := auth.HashPassword("testpassword123")
	store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")

	body, _ := json.Marshal(LoginRequest{Username: "alice", Password: "wrongpassword"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthLogin(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAuthLoginSucceeds(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")

	body, _ := json.Marshal(LoginRequest{Username: "alice", Password: "testpassword123"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["session_id"] == nil {
		t.Fatal("expected session_id in response")
	}
}

func TestHandleAuthLogoutClearsCookie(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthLogout(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected session cookie to be cleared")
	}
}

func TestHandleAuthMeReturnsUser(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")
	deps.UserIDFromContext = func(_ context.Context) string { return u.ID }

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]any)
	if user["username"] != "alice" {
		t.Fatalf("expected username alice, got %v", user["username"])
	}
}

func TestHandleAuthBootstrapStatusReturnsSetupRequired(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/bootstrap/status", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["setup_required"] != true {
		t.Fatalf("expected setup_required=true, got %v", resp["setup_required"])
	}
}

func TestHandleAuthBootstrapStatusReturnsFalseAfterSetup(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	store.CreateUserWithRole("admin", hash, auth.RoleOwner, "local", "")

	req := httptest.NewRequest(http.MethodGet, "/auth/bootstrap/status", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["setup_required"] != false {
		t.Fatalf("expected setup_required=false, got %v", resp["setup_required"])
	}
}

func TestHandleAuthBootstrapSetupRejectsWeakPassword(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(BootstrapSetupRequest{Username: "admin", Password: "password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthBootstrapSetupSucceeds(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(BootstrapSetupRequest{Username: "admin", Password: "secureP@ssw0rd!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteOwnAccountRejectsGET(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/account", nil)
	rec := httptest.NewRecorder()
	deps.HandleDeleteOwnAccount(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleDeleteOwnAccountSucceeds(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("alice", hash, auth.RoleViewer, "local", "")
	deps.UserIDFromContext = func(_ context.Context) string { return u.ID }

	req := httptest.NewRequest(http.MethodDelete, "/auth/account", nil)
	rec := httptest.NewRecorder()
	deps.HandleDeleteOwnAccount(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp["ok"])
	}
	// Verify user is actually deleted.
	_, exists, _ := store.GetUserByID(u.ID)
	if exists {
		t.Fatal("expected user to be deleted")
	}
}

func TestHandleDeleteOwnAccountForbidsOwner(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("owner-user", hash, auth.RoleOwner, "local", "")
	deps.UserIDFromContext = func(_ context.Context) string { return u.ID }

	req := httptest.NewRequest(http.MethodDelete, "/auth/account", nil)
	rec := httptest.NewRecorder()
	deps.HandleDeleteOwnAccount(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	// Verify user is NOT deleted.
	_, exists, _ := store.GetUserByID(u.ID)
	if !exists {
		t.Fatal("owner account should not have been deleted")
	}
}

func TestHandleDeleteOwnAccountClearsSessions(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("bob", hash, auth.RoleOperator, "local", "")
	store.CreateAuthSession(u.ID, "hash1", time.Now().Add(time.Hour))
	store.CreateAuthSession(u.ID, "hash2", time.Now().Add(time.Hour))
	deps.UserIDFromContext = func(_ context.Context) string { return u.ID }

	req := httptest.NewRequest(http.MethodDelete, "/auth/account", nil)
	rec := httptest.NewRecorder()
	deps.HandleDeleteOwnAccount(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	sessions, _ := store.ListSessionsByUserID(u.ID)
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after deletion, got %d", len(sessions))
	}
}

func TestHandleAuthProvidersReturnsLocal(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/providers", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthProviders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if local["enabled"] != true {
		t.Fatal("expected local auth to be enabled")
	}
}
