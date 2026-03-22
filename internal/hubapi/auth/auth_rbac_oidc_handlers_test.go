package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

// --- Pure function tests ---

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alice", "alice"},
		{"  Bob  ", "bob"},
		{"user@example.com", "user"},
		{"a-b_c.d", "a-b_c.d"},
		{"hello world!", "helloworld"},
		{"---leading", "leading"},
		{"trailing...", "trailing"},
		{"", ""},
		{"  ", ""},
		{"@@@", ""},
		{"abcdefghijklmnopqrstuvwxyz1234567890", "abcdefghijklmnopqrstuvwxyz123456"}, // 32 char max
	}
	for _, tt := range tests {
		got := NormalizeUsername(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOIDCAssignableRole(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"owner", "admin"}, // owner downgrades to admin
		{"Owner", "admin"}, // case insensitive
		{"admin", "admin"}, // stays admin
		{"operator", "operator"},
		{"viewer", "viewer"},
		{"", "viewer"},        // empty normalizes to viewer
		{"unknown", "viewer"}, // unknown normalizes to viewer
	}
	for _, tt := range tests {
		got := OIDCAssignableRole(tt.input)
		if got != tt.want {
			t.Errorf("OIDCAssignableRole(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSelectOIDCUsername(t *testing.T) {
	tests := []struct {
		name     string
		identity auth.OIDCIdentity
		want     string
	}{
		{
			name:     "preferred username",
			identity: auth.OIDCIdentity{PreferredUsername: "alice", Email: "alice@co.com"},
			want:     "alice",
		},
		{
			name:     "falls back to email",
			identity: auth.OIDCIdentity{Email: "bob@example.com"},
			want:     "bob",
		},
		{
			name:     "falls back to name",
			identity: auth.OIDCIdentity{Name: "Charlie"},
			want:     "charlie",
		},
		{
			name:     "falls back to subject",
			identity: auth.OIDCIdentity{Subject: "sub123"},
			want:     "sub123",
		},
		{
			name:     "all empty falls back to default",
			identity: auth.OIDCIdentity{},
			want:     "oidc-user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectOIDCUsername(tt.identity)
			if got != tt.want {
				t.Errorf("SelectOIDCUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeNextPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/dashboard", "/dashboard"},
		{"//evil.com", "/"},
		{"http://evil.com", "/"},
		{"/path?q=1", "/path?q=1"},
		{"javascript:alert(1)", "/"},
		{"data:text/html,evil", "/"},
		{"/JAVASCRIPT:x", "/"},
		{"  /trimmed  ", "/trimmed"},
	}
	for _, tt := range tests {
		got := SanitizeNextPath(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeNextPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateAuthRedirectURI(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"https://example.com/callback", false},
		{"http://localhost:3000/callback", false},
		{"", true},
		{"ftp://example.com", true},
		{"not-a-url", true},
		{"https://example.com/cb#frag", true},
	}
	for _, tt := range tests {
		_, err := ValidateAuthRedirectURI(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateAuthRedirectURI(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
	}
}

func TestRandomURLToken(t *testing.T) {
	tok1, err := RandomURLToken(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tok2, err := RandomURLToken(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok1 == tok2 {
		t.Fatal("expected unique tokens")
	}
	if len(tok1) == 0 {
		t.Fatal("expected non-empty token")
	}
}

// --- Handler tests ---

func TestHandleAuthUsersListUsers(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")
	store.CreateUserWithRole("bob", hash, auth.RoleViewer, "local", "")

	req := httptest.NewRequest(http.MethodGet, "/auth/users", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	users := resp["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestHandleAuthUsersCreateUser(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(authCreateUserRequest{
		Username: "newuser",
		Password: "secureP@ss1",
		Role:     "operator",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthUsers(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthUsersCreateUserRejectsInvalidRole(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(authCreateUserRequest{
		Username: "newuser",
		Password: "secureP@ss1",
		Role:     "superadmin",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthUsers(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthUsersCreateUserRejectsOwnerRole(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	body, _ := json.Marshal(authCreateUserRequest{
		Username: "newuser",
		Password: "secureP@ss1",
		Role:     "owner",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthUsers(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for owner role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthUserActionsUpdateRole(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("alice", hash, auth.RoleViewer, "local", "")

	newRole := "operator"
	body, _ := json.Marshal(authUpdateUserRequest{Role: &newRole})
	req := httptest.NewRequest(http.MethodPatch, "/auth/users/"+u.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleAuthUserActions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]any)
	if user["role"] != "operator" {
		t.Fatalf("expected role=operator, got %v", user["role"])
	}
}

func TestHandleAuthUserActionsDeleteUserPreventsSelfDelete(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")
	deps.UserIDFromContext = func(_ context.Context) string { return u.ID }

	req := httptest.NewRequest(http.MethodDelete, "/auth/users/"+u.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthUserActions(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-delete, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthUserActionsDeleteUser(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	u, _ := store.CreateUserWithRole("alice", hash, auth.RoleAdmin, "local", "")
	// Caller is a different user.
	deps.UserIDFromContext = func(_ context.Context) string { return "other-user" }

	req := httptest.NewRequest(http.MethodDelete, "/auth/users/"+u.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthUserActions(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthUserActionsRejectsExtraSegments(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/users/id/extra/segments", nil)
	rec := httptest.NewRecorder()
	deps.HandleAuthUserActions(rec, req)
	// Should be handled (not 404 for prefix mismatch) but method not allowed for GET.
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// --- OIDC state tests ---

func TestStoreAndConsumeOIDCState(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	state := "test-state-token"
	entry := OIDCAuthState{
		Nonce:       "nonce",
		NextPath:    "/dashboard",
		RedirectURI: "https://example.com/callback",
		ExpiresAt:   time.Now().UTC().Add(5 * time.Minute),
	}
	if !deps.StoreOIDCState(state, entry) {
		t.Fatal("expected StoreOIDCState to succeed")
	}
	got, ok := deps.ConsumeOIDCState(state, "https://example.com/callback")
	if !ok {
		t.Fatal("expected ConsumeOIDCState to succeed")
	}
	if got.Nonce != "nonce" {
		t.Fatalf("expected nonce, got %q", got.Nonce)
	}
	// Second consume should fail (already consumed).
	_, ok = deps.ConsumeOIDCState(state, "https://example.com/callback")
	if ok {
		t.Fatal("expected second consume to fail")
	}
}

func TestConsumeOIDCStateRejectsExpired(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	state := "expired-state"
	entry := OIDCAuthState{
		Nonce:       "nonce",
		RedirectURI: "https://example.com/callback",
		ExpiresAt:   time.Now().UTC().Add(-1 * time.Minute), // already expired
	}
	deps.StoreOIDCState(state, entry)
	_, ok := deps.ConsumeOIDCState(state, "https://example.com/callback")
	if ok {
		t.Fatal("expected expired state to be rejected")
	}
}

func TestConsumeOIDCStateRejectsMismatchedRedirectURI(t *testing.T) {
	deps, _ := newTestAuthDeps(t)
	state := "uri-mismatch"
	entry := OIDCAuthState{
		Nonce:       "nonce",
		RedirectURI: "https://example.com/callback",
		ExpiresAt:   time.Now().UTC().Add(5 * time.Minute),
	}
	deps.StoreOIDCState(state, entry)
	_, ok := deps.ConsumeOIDCState(state, "https://evil.com/callback")
	if ok {
		t.Fatal("expected mismatched redirect URI to be rejected")
	}
}

func TestUniqueUsername(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	hash, _ := auth.HashPassword("testpassword123")
	store.CreateUserWithRole("alice", hash, auth.RoleViewer, "local", "")

	// First call: alice exists → should get alice-2.
	name, err := deps.UniqueUsername("alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "alice-2" {
		t.Fatalf("expected alice-2, got %q", name)
	}

	// Non-colliding name should be returned as-is.
	name, err = deps.UniqueUsername("bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "bob" {
		t.Fatalf("expected bob, got %q", name)
	}
}
