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
