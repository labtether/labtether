package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Unified Search ---

func TestHandleV2Search_ReturnsMatchingAssets(t *testing.T) {
	s := newTestAPIServer(t)

	// Seed an asset.
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "host-search-01",
		Name:     "searchable-linux-host",
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/search?q=searchable", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataMap, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map in response, got: %v", resp)
	}
	assetMatches, ok := dataMap["assets"].([]any)
	if !ok || len(assetMatches) == 0 {
		t.Errorf("expected at least one asset match, got data: %v", dataMap)
	}
}

func TestHandleV2Search_RequiresQuery(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/search", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Search_EmptyQueryReturnsError(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/search?q=", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Bulk Operations ---

func TestHandleV2BulkServiceAction_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"action":"restart","service":"nginx","targets":["host-01"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Set a limited scope that does NOT include bulk:*
	ctx := contextWithPrincipal(req.Context(), "user1", "user")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 scope denial, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_MissingFields(t *testing.T) {
	s := newTestAPIServer(t)

	// No targets
	body := `{"action":"restart","service":"nginx","targets":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/bulk/service-action", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── POST /api/v2/settings/prometheus/test ────────────────────────────────

func TestHandleV2PrometheusTest_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"url":"http://localhost:9090/api/v1/write"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/settings/prometheus/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"settings:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2PrometheusTest(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2PrometheusTest_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/settings/prometheus/test", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2PrometheusTest(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2PrometheusTest_EmptyURL(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"url":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/settings/prometheus/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2PrometheusTest(rec, req)
	// Returns 200 with success=false when URL is empty.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", resp)
	}
	if data["success"] != false {
		t.Errorf("expected success=false, got %v", data["success"])
	}
}

// ─── POST /api/v2/hub/tls/renew ───────────────────────────────────────────

func TestHandleV2HubTLSRenew_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/tls/renew", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"hub:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2HubTLSRenew(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2HubTLSRenew_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/hub/tls/renew", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2HubTLSRenew(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2HubTLSRenew_TLSDisabled_Returns422(t *testing.T) {
	s := newTestAPIServer(t)
	// Default test server has tlsSource == "" (disabled).
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/tls/renew", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2HubTLSRenew(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when TLS disabled, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2HubTLSRenew_UploadedSource_Returns422(t *testing.T) {
	s := newTestAPIServer(t)
	s.tlsState.Source = tlsSourceUIUploaded
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/tls/renew", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2HubTLSRenew(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2HubTLSRenew_BuiltIn_NilReloader(t *testing.T) {
	s := newTestAPIServer(t)
	s.tlsState.Source = tlsSourceBuiltIn
	s.tlsState.CertReloader = nil
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/tls/renew", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2HubTLSRenew(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when reloader is nil, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2HubTLSDeleteRequiresAdminScope(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/hub/tls", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	ctx = contextWithScopes(ctx, []string{"hub:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.handleV2HubTLS(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
