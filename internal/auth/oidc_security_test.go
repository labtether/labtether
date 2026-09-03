package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func allowOIDCTestLoopbackHTTP(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func testOIDCSettings(issuer string) OIDCSettings {
	return OIDCSettings{
		Enabled:                true,
		IssuerURL:              issuer,
		ClientID:               "test-client",
		ClientSecret:           "test-client-secret", // #nosec G101 -- test-only fixture
		AllowLoopbackEndpoints: true,
	}
}

func writeOIDCDiscovery(w http.ResponseWriter, issuer, tokenURL, jwksURL string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{
		"issuer": %q,
		"authorization_endpoint": %q,
		"token_endpoint": %q,
		"jwks_uri": %q,
		"id_token_signing_alg_values_supported": ["RS256"]
	}`, issuer, issuer+"/authorize", tokenURL, jwksURL)
}

func newOIDCDiscoveryServer(t *testing.T, endpointURLs func(string) (string, string)) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		tokenURL, jwksURL := endpointURLs(server.URL)
		writeOIDCDiscovery(w, server.URL, tokenURL, jwksURL)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNewOIDCProviderRejectsInsecureLoopbackBeforeDiscovery(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	_, err := NewOIDCProvider(context.Background(), testOIDCSettings(server.URL))
	if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) {
		t.Fatalf("expected invalid configuration error, got %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("unsafe issuer received %d discovery requests, want 0", got)
	}
}

func TestNewOIDCProviderRejectsAmbiguousIssuerURLs(t *testing.T) {
	for _, issuer := range []string{
		"https://user:password@identity.example.com",
		"https://identity.example.com?discovery=elsewhere",
		"https://identity.example.com#fragment",
		"ftp://identity.example.com",
	} {
		t.Run(issuer, func(t *testing.T) {
			_, err := NewOIDCProvider(context.Background(), testOIDCSettings(issuer))
			if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) {
				t.Fatalf("expected issuer URL rejection, got %v", err)
			}
		})
	}
}

func TestNewOIDCProviderRequiresExactExtraOrigins(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	issuer := newOIDCDiscoveryServer(t, func(issuer string) (string, string) {
		return issuer + "/token", issuer + "/keys"
	})
	for _, origin := range []string{
		issuer.URL + "/token",
		issuer.URL + "?target=token",
		strings.Replace(issuer.URL, "http://", "http://user@", 1),
	} {
		t.Run(origin, func(t *testing.T) {
			settings := testOIDCSettings(issuer.URL)
			settings.AllowedEndpointOrigins = []string{origin}
			_, err := NewOIDCProvider(context.Background(), settings)
			if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) {
				t.Fatalf("expected extra-origin rejection, got %v", err)
			}
		})
	}
}

func TestNewOIDCProviderKeepsPrivateAndLinkLocalDeniedByDefault(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL", "true")
	for _, issuer := range []string{
		"https://10.0.0.8/identity",
		"https://169.254.169.254/identity",
	} {
		t.Run(issuer, func(t *testing.T) {
			_, err := NewOIDCProvider(context.Background(), testOIDCSettings(issuer))
			if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) {
				t.Fatalf("expected OIDC-specific address policy rejection, got %v", err)
			}
		})
	}
}

func TestNewOIDCProviderRejectsUnapprovedTokenOrigin(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	issuer := newOIDCDiscoveryServer(t, func(issuer string) (string, string) {
		return target.URL + "/token", issuer + "/keys"
	})

	_, err := NewOIDCProvider(context.Background(), testOIDCSettings(issuer.URL))
	if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) || !strings.Contains(err.Error(), "token endpoint") {
		t.Fatalf("expected unapproved token endpoint rejection, got %v", err)
	}
}

func TestNewOIDCProviderRejectsUnapprovedSigningKeyOrigin(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	issuer := newOIDCDiscoveryServer(t, func(issuer string) (string, string) {
		return issuer + "/token", target.URL + "/keys"
	})

	_, err := NewOIDCProvider(context.Background(), testOIDCSettings(issuer.URL))
	if err == nil || !errors.Is(err, ErrOIDCInvalidConfiguration) || !strings.Contains(err.Error(), "signing key endpoint") {
		t.Fatalf("expected unapproved signing-key endpoint rejection, got %v", err)
	}
}

func TestNewOIDCProviderAllowsExplicitExtraEndpointOrigin(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer target.Close()
	issuer := newOIDCDiscoveryServer(t, func(string) (string, string) {
		return target.URL + "/token", target.URL + "/keys"
	})
	settings := testOIDCSettings(issuer.URL)
	settings.AllowedEndpointOrigins = []string{target.URL}

	provider, err := NewOIDCProvider(context.Background(), settings)
	if err != nil {
		t.Fatalf("create provider with explicit extra origin: %v", err)
	}
	if !provider.Enabled() {
		t.Fatal("provider should be enabled")
	}
}

func TestOIDCTokenRedirectCannotForwardClientSecretAcrossOrigins(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	var targetHits atomic.Int32
	var tokenHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOIDCDiscovery(w, issuer.URL, issuer.URL+"/token", issuer.URL+"/keys")
		case "/token":
			tokenHits.Add(1)
			http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()
	settings := testOIDCSettings(issuer.URL)
	settings.AllowedEndpointOrigins = []string{target.URL}
	provider, err := NewOIDCProvider(context.Background(), settings)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if _, err := provider.ExchangeCode(context.Background(), "code", "nonce", "https://app.example/callback"); err == nil {
		t.Fatal("expected cross-origin token redirect to fail")
	}
	if got := tokenHits.Load(); got == 0 {
		t.Fatal("token endpoint was not exercised")
	}
	if got := targetHits.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want 0", got)
	}
}

func TestOIDCSigningKeyRedirectCannotCrossOrigins(t *testing.T) {
	allowOIDCTestLoopbackHTTP(t)
	var targetHits atomic.Int32
	var signingKeyHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOIDCDiscovery(w, issuer.URL, issuer.URL+"/token", issuer.URL+"/keys")
		case "/keys":
			signingKeyHits.Add(1)
			http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()
	settings := testOIDCSettings(issuer.URL)
	settings.AllowedEndpointOrigins = []string{target.URL}
	provider, err := NewOIDCProvider(context.Background(), settings)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"iss":%q,"sub":"subject","aud":"test-client","exp":%d}`,
		issuer.URL,
		time.Now().Add(time.Minute).Unix(),
	)))
	if _, err := provider.verifier.Verify(context.Background(), header+"."+payload+".AA"); err == nil {
		t.Fatal("expected signing-key redirect to fail verification")
	}
	if got := signingKeyHits.Load(); got == 0 {
		t.Fatal("signing-key endpoint was not exercised")
	}
	if got := targetHits.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want 0", got)
	}
}
