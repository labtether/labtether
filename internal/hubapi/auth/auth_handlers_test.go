package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// --- test Deps factory ---

func newTestAuthDeps(t *testing.T) (*Deps, *memAuthStore) {
	t.Helper()
	store := newMemAuthStore()
	return &Deps{
		AuthStore:                 store,
		OIDCRef:                   &OIDCProviderRef{},
		ChallengeStore:            auth.NewChallengeStore(),
		TOTPEncryptionKey:         make([]byte, 32),
		OIDCStates:                make(map[string]OIDCAuthState),
		EnforceRateLimit:          testutil.NoopRateLimit,
		ValidateOwnerTokenRequest: func(_ *http.Request) bool { return true },
		UserIDFromContext:         testutil.TestUserID,
		WrapAuth:                  testutil.NoopAuth,
		WrapAdmin:                 testutil.NoopAuth,
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
	t.Setenv("LABTETHER_SETUP_TOKEN", "test-local-setup-token")
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(BootstrapSetupRequest{Username: "admin", Password: "password"})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(BootstrapSetupTokenHeader(), "test-local-setup-token")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthBootstrapSetupSucceeds(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "test-local-setup-token")
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(BootstrapSetupRequest{Username: "admin", Password: "secureP@ssw0rd!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(BootstrapSetupTokenHeader(), "test-local-setup-token")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthBootstrapSetupFailsClosedWithoutConfiguredToken(t *testing.T) {
	t.Setenv("LABTETHER_SETUP_TOKEN", "")
	t.Setenv("LABTETHER_SETUP_TOKEN_FILE", filepath.Join(t.TempDir(), "missing-token"))
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(BootstrapSetupRequest{Username: "admin", Password: "secureP@ssw0rd!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(BootstrapSetupTokenHeader(), "attacker-controlled")
	rec := httptest.NewRecorder()
	deps.HandleAuthBootstrapSetup(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when setup token is not configured, got %d: %s", rec.Code, rec.Body.String())
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
