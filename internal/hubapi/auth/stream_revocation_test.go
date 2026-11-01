package auth

import (
	"testing"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// An IdP-side demotion applied at SSO sign-in must close the user's open
// root terminals, like a demotion made in the console.
func TestSSORoleDemotionClosesStreams(t *testing.T) {
	deps, store := newTestAuthDeps(t)
	user, err := store.CreateUserWithOIDCIdentity("sso-admin", "", auth.RoleAdmin, "oidc", "https://idp.example", "subject-1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	var revoked []string
	deps.RevokeStreams = func(keys ...string) { revoked = append(revoked, keys...) }

	if _, _, err := deps.ResolveOIDCUser(auth.OIDCIdentity{Issuer: "https://idp.example", Subject: "subject-1", Role: auth.RoleViewer}); err != nil {
		t.Fatalf("resolve user: %v", err)
	}
	if len(revoked) == 0 || revoked[0] != shared.StreamActorKey(user.ID) {
		t.Fatalf("revoked keys = %v, want the user's streams", revoked)
	}
}
