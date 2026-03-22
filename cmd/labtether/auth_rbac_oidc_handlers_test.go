package main

import (
	"net/http"
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

func TestSanitizeNextPath(t *testing.T) {
	if got := sanitizeNextPath("/console?tab=home"); got != "/console?tab=home" {
		t.Fatalf("expected valid path to pass through, got %q", got)
	}
	for _, input := range []string{"", "https://evil.example", "//evil", "/javascript:alert(1)"} {
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
