package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScopesFromContext(t *testing.T) {
	ctx := contextWithScopes(context.Background(), []string{"assets:read", "files:*"})
	scopes := scopesFromContext(ctx)
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(scopes))
	}
}

func TestAllowedAssetsFromContext(t *testing.T) {
	ctx := contextWithAllowedAssets(context.Background(), []string{"server1"})
	assets := allowedAssetsFromContext(ctx)
	if len(assets) != 1 || assets[0] != "server1" {
		t.Errorf("expected [server1], got %v", assets)
	}

	ctx2 := contextWithAllowedAssets(context.Background(), nil)
	if allowedAssetsFromContext(ctx2) != nil {
		t.Error("nil allowed_assets should return nil")
	}
}

func TestAPIKeyIDFromContext(t *testing.T) {
	ctx := contextWithAPIKeyID(context.Background(), "key_abc123")
	if apiKeyIDFromContext(ctx) != "key_abc123" {
		t.Error("should return stored key ID")
	}
}

func TestWithAuth_NoAuth_Unauthorized(t *testing.T) {
	s := &apiServer{}
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestWithAuth_APIKey_RejectPlainHTTP(t *testing.T) {
	s := &apiServer{tlsState: TLSState{Enabled: false}}
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "http://hub.local/test", nil)
	req.Header.Set("Authorization", "Bearer lt_test_fakesecret12345")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("API key auth on plain HTTP should not succeed")
	}
}

func TestWithAuth_APIKey_InvalidKey_Returns401(t *testing.T) {
	s := newTestAPIServer(t)
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer lt_fake_notarealkey123456")
	// Simulate TLS so TLS check passes
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid API key, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuth_APIKey_StoreNil_Returns503(t *testing.T) {
	s := newTestAPIServer(t)
	s.apiKeyStore = nil // simulate unconfigured store
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer lt_fake_notarealkey123456")
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil store, got %d: %s", rec.Code, rec.Body.String())
	}
}
