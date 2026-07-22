package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
)

func TestMethodAllowedForRole(t *testing.T) {
	tests := []struct {
		name   string
		role   string
		method string
		want   bool
	}{
		{name: "viewer get", role: auth.RoleViewer, method: http.MethodGet, want: true},
		{name: "viewer post", role: auth.RoleViewer, method: http.MethodPost, want: false},
		{name: "operator post", role: auth.RoleOperator, method: http.MethodPost, want: true},
		{name: "admin patch", role: auth.RoleAdmin, method: http.MethodPatch, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := methodAllowedForRole(tt.role, tt.method); got != tt.want {
				t.Fatalf("methodAllowedForRole(%q, %q) = %v, want %v", tt.role, tt.method, got, tt.want)
			}
		})
	}
}

func TestSelfServiceAuthAllowsViewerMutationButRejectsNonSessionCredentials(t *testing.T) {
	sut := newTestAPIServer(t)
	viewer, err := sut.authStore.CreateUserWithRole("viewer-self-service", "unused", auth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	rawToken := "viewer-self-service-session"
	if _, err := sut.authStore.CreateAuthSession(viewer.ID, auth.HashToken(rawToken), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create viewer session: %v", err)
	}

	called := false
	handler := sut.withSelfServiceAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := userIDFromContext(r.Context()); got != viewer.ID {
			t.Fatalf("principal = %q, want %q", got, viewer.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/2fa/setup", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: rawToken})
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("viewer self-service mutation status = %d, called = %t", rec.Code, called)
	}

	called = false
	req = httptest.NewRequest(http.MethodPost, "/auth/2fa/setup", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("non-session credential status = %d, called = %t", rec.Code, called)
	}

	called = false
	generic := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	req = httptest.NewRequest(http.MethodPost, "/assets/manual", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: rawToken})
	rec = httptest.NewRecorder()
	generic(rec, req)
	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("viewer fleet mutation status = %d, called = %t", rec.Code, called)
	}
}

func TestSanitizeNextPath(t *testing.T) {
	if got := sanitizeNextPath("/console?tab=home"); got != "/console?tab=home" {
		t.Fatalf("expected valid path to pass through, got %q", got)
	}
	for _, input := range []string{"", "https://evil.example", "//evil", "/javascript:alert(1)", "/\\evil.example", "/%5cevil.example"} {
		if got := sanitizeNextPath(input); got != "/" {
			t.Fatalf("expected %q to normalize to '/', got %q", input, got)
		}
	}
}

func TestNormalizeUsername(t *testing.T) {
	if got := normalizeUsername(" Jane.Doe@example.com "); got != "jane.doe" {
		t.Fatalf("unexpected normalized username: %q", got)
	}
	if got := normalizeUsername("!!!"); got != "" {
		t.Fatalf("expected invalid username to normalize to empty, got %q", got)
	}
}

func TestOIDCStateStoreAndConsume(t *testing.T) {
	sut := &apiServer{}
	if !sut.storeOIDCState("state-1", oidcAuthState{
		Nonce:       "nonce-1",
		NextPath:    "/",
		RedirectURI: "https://console.local/api/auth/oidc/callback",
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
	}) {
		t.Fatalf("expected oidc state to be stored")
	}
	entry, ok := sut.consumeOIDCState("state-1", "https://console.local/api/auth/oidc/callback")
	if !ok {
		t.Fatalf("expected state to be consumed")
	}
	if entry.Nonce != "nonce-1" {
		t.Fatalf("expected nonce-1, got %q", entry.Nonce)
	}
	if _, ok := sut.consumeOIDCState("state-1", "https://console.local/api/auth/oidc/callback"); ok {
		t.Fatalf("state should be single-use")
	}
}

func TestResolveOIDCUserBlocksAutoProvisionBeforeInitialSetup(t *testing.T) {
	store := &fakeBootstrapAuthStore{fakeAdminBootstrapStore: &fakeAdminBootstrapStore{}}
	sut := &apiServer{
		authStore: store,
		oidcRef:   authpkg.NewOIDCProviderRef(nil, true),
	}

	_, _, err := sut.resolveOIDCUser(auth.OIDCIdentity{
		Issuer:  "https://issuer.example",
		Subject: "oidc-subject-1",
		Role:    auth.RoleViewer,
	})
	if err != errOIDCSetupRequired {
		t.Fatalf("expected errOIDCSetupRequired, got %v", err)
	}
	if store.createCount != 0 {
		t.Fatalf("expected no auto-provisioned user before bootstrap setup, got %d", store.createCount)
	}
}

func TestOIDCAssignableRoleDowngradesOwnerToAdmin(t *testing.T) {
	if got := oidcAssignableRole(auth.RoleOwner); got != auth.RoleAdmin {
		t.Fatalf("expected owner oidc role to downgrade to admin, got %q", got)
	}
}
