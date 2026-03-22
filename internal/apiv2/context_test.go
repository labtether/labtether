package apiv2

import (
	"context"
	"testing"
)

func TestPrincipalActorID_EmptyContext(t *testing.T) {
	got := PrincipalActorID(context.Background())
	if got != "system" {
		t.Errorf("PrincipalActorID on empty context = %q, want %q", got, "system")
	}
}

func TestPrincipalActorID_WithUserID(t *testing.T) {
	ctx := ContextWithUserID(context.Background(), "user-42")
	got := PrincipalActorID(ctx)
	if got != "user-42" {
		t.Errorf("PrincipalActorID = %q, want %q", got, "user-42")
	}
}

func TestPrincipalActorID_WhitespaceOnlyUserID(t *testing.T) {
	ctx := ContextWithUserID(context.Background(), "   ")
	got := PrincipalActorID(ctx)
	if got != "system" {
		t.Errorf("PrincipalActorID with whitespace-only user ID = %q, want %q", got, "system")
	}
}

func TestScopesFromContext_EmptyContext(t *testing.T) {
	got := ScopesFromContext(context.Background())
	if got != nil {
		t.Errorf("ScopesFromContext on empty context = %v, want nil", got)
	}
}

func TestScopesFromContext_RoundTrip(t *testing.T) {
	scopes := []string{"assets:read", "docker:write"}
	ctx := ContextWithScopes(context.Background(), scopes)
	got := ScopesFromContext(ctx)
	if len(got) != len(scopes) {
		t.Fatalf("ScopesFromContext len = %d, want %d", len(got), len(scopes))
	}
	for i, s := range scopes {
		if got[i] != s {
			t.Errorf("ScopesFromContext[%d] = %q, want %q", i, got[i], s)
		}
	}
}

func TestScopesFromContext_NilInput(t *testing.T) {
	// ContextWithScopes with a nil ctx must not panic.
	ctx := ContextWithScopes(nil, []string{"assets:read"})
	got := ScopesFromContext(ctx)
	if len(got) != 1 || got[0] != "assets:read" {
		t.Errorf("ScopesFromContext after nil ctx = %v, want [assets:read]", got)
	}
}

func TestAllowedAssetsFromContext_EmptyContext(t *testing.T) {
	got := AllowedAssetsFromContext(context.Background())
	if got != nil {
		t.Errorf("AllowedAssetsFromContext on empty context = %v, want nil", got)
	}
}

func TestAllowedAssetsFromContext_RoundTrip(t *testing.T) {
	assets := []string{"server1", "server2"}
	ctx := ContextWithAllowedAssets(context.Background(), assets)
	got := AllowedAssetsFromContext(ctx)
	if len(got) != len(assets) {
		t.Fatalf("AllowedAssetsFromContext len = %d, want %d", len(got), len(assets))
	}
	for i, a := range assets {
		if got[i] != a {
			t.Errorf("AllowedAssetsFromContext[%d] = %q, want %q", i, got[i], a)
		}
	}
}

func TestContextWithUserID_RoundTrip(t *testing.T) {
	ctx := ContextWithUserID(context.Background(), "alice")
	got := UserIDFromContext(ctx)
	if got != "alice" {
		t.Errorf("UserIDFromContext = %q, want %q", got, "alice")
	}
}

func TestContextWithUserID_EmptyContext(t *testing.T) {
	got := UserIDFromContext(context.Background())
	if got != "" {
		t.Errorf("UserIDFromContext on empty context = %q, want empty string", got)
	}
}

func TestContextWithUserRole_RoundTrip(t *testing.T) {
	ctx := ContextWithUserRole(context.Background(), "admin")
	got := UserRoleFromContext(ctx)
	if got != "admin" {
		t.Errorf("UserRoleFromContext = %q, want %q", got, "admin")
	}
}

func TestContextWithPrincipal_RoundTrip(t *testing.T) {
	ctx := ContextWithPrincipal(context.Background(), "bob", "operator")
	if got := UserIDFromContext(ctx); got != "bob" {
		t.Errorf("UserIDFromContext after ContextWithPrincipal = %q, want %q", got, "bob")
	}
	if got := UserRoleFromContext(ctx); got != "operator" {
		t.Errorf("UserRoleFromContext after ContextWithPrincipal = %q, want %q", got, "operator")
	}
}

func TestAPIKeyIDFromContext_RoundTrip(t *testing.T) {
	ctx := ContextWithAPIKeyID(context.Background(), "key-abc")
	got := APIKeyIDFromContext(ctx)
	if got != "key-abc" {
		t.Errorf("APIKeyIDFromContext = %q, want %q", got, "key-abc")
	}
}

func TestAPIKeyIDFromContext_EmptyContext(t *testing.T) {
	got := APIKeyIDFromContext(context.Background())
	if got != "" {
		t.Errorf("APIKeyIDFromContext on empty context = %q, want empty string", got)
	}
}

func TestContextKeys_NoCollision(t *testing.T) {
	// Verify each key stores and retrieves independently without cross-contamination.
	ctx := context.Background()
	ctx = ContextWithUserID(ctx, "user-1")
	ctx = ContextWithUserRole(ctx, "admin")
	ctx = ContextWithScopes(ctx, []string{"s1"})
	ctx = ContextWithAllowedAssets(ctx, []string{"a1"})
	ctx = ContextWithAPIKeyID(ctx, "k1")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"user_id", UserIDFromContext(ctx), "user-1"},
		{"user_role", UserRoleFromContext(ctx), "admin"},
		{"api_key_id", APIKeyIDFromContext(ctx), "k1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}

	if scopes := ScopesFromContext(ctx); len(scopes) != 1 || scopes[0] != "s1" {
		t.Errorf("ScopesFromContext = %v, want [s1]", scopes)
	}
	if assets := AllowedAssetsFromContext(ctx); len(assets) != 1 || assets[0] != "a1" {
		t.Errorf("AllowedAssetsFromContext = %v, want [a1]", assets)
	}
}
