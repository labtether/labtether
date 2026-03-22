package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleV2Webhooks_Create(t *testing.T) {
	s := newTestAPIServer(t)
	handler := s.handleV2Webhooks

	body := `{"name":"my-webhook","url":"https://example.com/hook","events":["asset.online","asset.offline"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object in response")
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected non-empty id in response")
	}
	if data["name"] != "my-webhook" {
		t.Errorf("expected name 'my-webhook', got %v", data["name"])
	}
	if data["url"] != "https://example.com/hook" {
		t.Errorf("expected url 'https://example.com/hook', got %v", data["url"])
	}
}

func TestHandleV2Webhooks_Create_MissingName(t *testing.T) {
	s := newTestAPIServer(t)
	handler := s.handleV2Webhooks

	body := `{"url":"https://example.com/hook","events":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Webhooks_Create_MissingURL(t *testing.T) {
	s := newTestAPIServer(t)
	handler := s.handleV2Webhooks

	body := `{"name":"my-webhook","events":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2Webhooks_List(t *testing.T) {
	s := newTestAPIServer(t)
	handler := s.handleV2Webhooks

	req := httptest.NewRequest(http.MethodGet, "/api/v2/webhooks", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["data"] == nil {
		t.Error("expected data field in list response")
	}
}

func TestHandleV2Webhooks_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/webhooks", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no webhooks scope
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Webhooks(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2Webhooks_CreateInvalidURLScheme(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"name":"bad-scheme","url":"ftp://evil.internal/hook","events":["alert.fired"]}`
	req := httptest.NewRequest("POST", "/api/v2/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Webhooks(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for ftp:// URL, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2WebhookActions_Delete(t *testing.T) {
	s := newTestAPIServer(t)

	// Create one first.
	createBody := `{"name":"to-delete","url":"https://example.com/hook","events":[]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/webhooks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(createReq.Context(), "admin", "admin")
	createReq = createReq.WithContext(ctx)
	createRec := httptest.NewRecorder()
	s.handleV2Webhooks(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("setup: unmarshal: %v", err)
	}
	id := createResp["data"].(map[string]any)["id"].(string)

	// Now delete it.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v2/webhooks/"+id, nil)
	delReq.URL.Path = "/api/v2/webhooks/" + id
	delReq = delReq.WithContext(contextWithPrincipal(delReq.Context(), "admin", "admin"))
	delRec := httptest.NewRecorder()
	s.handleV2WebhookActions(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", delRec.Code, delRec.Body.String())
	}

	var delResp map[string]any
	if err := json.Unmarshal(delRec.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("delete unmarshal: %v", err)
	}
	delData, ok := delResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in delete response, got %v", delResp["data"])
	}
	if delData["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %v", delData["status"])
	}
}
