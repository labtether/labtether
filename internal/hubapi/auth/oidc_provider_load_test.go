package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

// Single sign-on set in the console was off after every Hub restart: the Hub
// built its provider only from LABTETHER_OIDC_* at start.
func TestActivateOIDCAtStartUsesSettingsSavedInConsole(t *testing.T) {
	for _, key := range []string{"LABTETHER_OIDC_ENABLED", "LABTETHER_OIDC_ISSUER_URL", "LABTETHER_OIDC_CLIENT_ID", "LABTETHER_OIDC_CLIENT_SECRET", "LABTETHER_OIDC_AUTO_PROVISION"} {
		t.Setenv(key, "")
	}
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_OIDC_ALLOW_LOOPBACK", "true")
	var issuer string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
			"jwks_uri": issuer + "/keys", "response_types_supported": []string{"code"},
			"subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	defer idp.Close()
	issuer = idp.URL

	manager := testutil.TestSecretsManager(t)
	ciphertext, err := manager.EncryptString("client-secret", persistence.OIDCClientSecretAAD)
	if err != nil {
		t.Fatal(err)
	}
	enabled, autoProvision := true, false
	raw, err := json.Marshal(oidcDBSettings{
		Enabled: &enabled, IssuerURL: issuer, ClientID: "labtether", ClientSecretCiphertext: ciphertext, AutoProvision: &autoProvision,
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		SettingsStore:  &oidcSettingsTestStore{raw: raw, found: true},
		SecretsManager: manager,
		OIDCRef:        NewOIDCProviderRef(nil, false),
	}

	deps.ActivateOIDCAtStart(context.Background())

	provider, gotAutoProvision := deps.OIDCRef.Get()
	if provider == nil || provider.IssuerURL() != issuer || gotAutoProvision {
		t.Fatalf("after start: provider=%v auto_provision=%t; want the saved settings active", provider != nil, gotAutoProvision)
	}
}
