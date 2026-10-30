package main

import (
	"net/http/httptest"
	"testing"
)

func TestValidateOIDCRedirectURIDoesNotTrustArbitraryOriginHeader(t *testing.T) {
	sut := &apiServer{}
	req := httptest.NewRequest("POST", "https://hub.example/auth/oidc/start", nil)
	req.Host = "hub.example"
	req.Header.Set("Origin", "https://attacker.example")

	if _, err := sut.validateOIDCRedirectURI(req, "https://attacker.example/api/auth/oidc/callback"); err == nil {
		t.Fatal("expected arbitrary Origin header not to authorize an attacker callback")
	}
}

func TestValidateOIDCRedirectURIAllowsExactRequestOrigin(t *testing.T) {
	sut := &apiServer{}
	req := httptest.NewRequest("POST", "https://hub.example/auth/oidc/start", nil)
	req.Host = "hub.example"
	req.Header.Set("Origin", "https://hub.example")

	got, err := sut.validateOIDCRedirectURI(req, "https://hub.example/api/auth/oidc/callback")
	if err != nil {
		t.Fatalf("validate exact callback: %v", err)
	}
	if got != "https://hub.example/api/auth/oidc/callback" {
		t.Fatalf("redirect = %q", got)
	}
}

func TestValidateOIDCRedirectURIDeniesDifferentPort(t *testing.T) {
	sut := &apiServer{}
	req := httptest.NewRequest("POST", "https://hub.example:8443/auth/oidc/start", nil)
	req.Host = "hub.example:8443"
	req.Header.Set("Origin", "https://hub.example:8443")

	if _, err := sut.validateOIDCRedirectURI(req, "https://hub.example:9443/api/auth/oidc/callback"); err == nil {
		t.Fatal("expected a different service on the same hostname to be denied")
	}
}

func TestValidateOIDCRedirectURIDeniesArbitraryLoopbackPort(t *testing.T) {
	sut := &apiServer{}
	req := httptest.NewRequest("POST", "http://127.0.0.1:8080/auth/oidc/start", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://127.0.0.1:8080")

	if _, err := sut.validateOIDCRedirectURI(req, "http://localhost:49152/api/auth/oidc/callback"); err == nil {
		t.Fatal("expected an arbitrary loopback callback port to be denied")
	}
}

func TestValidateOIDCRedirectURIAllowsConfiguredExternalOrigin(t *testing.T) {
	sut := &apiServer{externalURL: "https://console.example:9443"}
	req := httptest.NewRequest("POST", "https://labtether:8443/auth/oidc/start", nil)
	req.Host = "labtether:8443"

	got, err := sut.validateOIDCRedirectURI(req, "https://console.example:9443/api/auth/oidc/callback")
	if err != nil {
		t.Fatalf("validate configured external callback: %v", err)
	}
	if got != "https://console.example:9443/api/auth/oidc/callback" {
		t.Fatalf("redirect = %q", got)
	}
}

// On a default LAN install the console is http://<LAN address>:3000 and calls
// the Hub at https://labtether:8443, so single sign-on needs the console's
// browser address configured. Request headers must not widen the list.
func TestValidateOIDCRedirectURIAllowsConfiguredConsoleOriginOnly(t *testing.T) {
	sut := &apiServer{consoleURL: "http://192.0.2.10:3000/"}
	req := httptest.NewRequest("POST", "https://labtether:8443/auth/oidc/start", nil)
	req.Host = "labtether:8443"

	got, err := sut.validateOIDCRedirectURI(req, "http://192.0.2.10:3000/api/auth/oidc/callback")
	if err != nil || got != "http://192.0.2.10:3000/api/auth/oidc/callback" {
		t.Fatalf("configured console callback = %q, %v", got, err)
	}
	if _, err := sut.validateOIDCRedirectURI(req, "http://192.0.2.10:3001/api/auth/oidc/callback"); err == nil {
		t.Fatal("another port on the console host was allowed")
	}
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	if _, err := sut.validateOIDCRedirectURI(req, "https://attacker.example/api/auth/oidc/callback"); err == nil {
		t.Fatal("a forwarded host from an untrusted peer was allowed")
	}
}
