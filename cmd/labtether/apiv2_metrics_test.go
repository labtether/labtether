package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleV2MetricsOverview_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/metrics/overview", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2MetricsOverview(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2MetricsOverview_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/metrics/overview", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2MetricsOverview(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("session auth should have full access")
	}
}
