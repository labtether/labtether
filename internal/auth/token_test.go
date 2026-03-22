package auth

import (
	"net/http/httptest"
	"testing"
)

func TestTokenValidatorAcceptsAnyConfiguredToken(t *testing.T) {
	validator := NewTokenValidator("owner-token", "api-token", "owner-token")
	if !validator.Configured() {
		t.Fatalf("expected validator to be configured")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer api-token")
	if !validator.ValidateRequest(req) {
		t.Fatalf("expected api token to validate")
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Labtether-Token", "owner-token")
	if !validator.ValidateRequest(req2) {
		t.Fatalf("expected owner token header to validate")
	}
}
