package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func TestHandleAPIKeys_Create(t *testing.T) {
	s := newTestAPIServer(t)
	s.apiKeyStore = persistence.NewMemoryAPIKeyStore()
	handler := s.handleAPIKeys

	body := `{"name":"test-key","role":"operator","scopes":["assets:read","assets:exec"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
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
	data := resp["data"].(map[string]any)
	if data["raw_key"] == nil {
		t.Error("response should include raw_key on creation")
	}
	rawKey := data["raw_key"].(string)
	if !strings.HasPrefix(rawKey, "lt_") {
		t.Errorf("raw_key should start with lt_, got %q", rawKey)
	}
}

func TestHandleAPIKeys_List(t *testing.T) {
	s := newTestAPIServer(t)
	s.apiKeyStore = persistence.NewMemoryAPIKeyStore()
	handler := s.handleAPIKeys

	req := httptest.NewRequest("GET", "/api/v2/keys", nil)
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleAPIKeys_InvalidRole(t *testing.T) {
	s := newTestAPIServer(t)
	s.apiKeyStore = persistence.NewMemoryAPIKeyStore()
	handler := s.handleAPIKeys

	body := `{"name":"test","role":"superadmin","scopes":["assets:read"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAPIKeys_InvalidScopes(t *testing.T) {
	s := newTestAPIServer(t)
	s.apiKeyStore = persistence.NewMemoryAPIKeyStore()
	handler := s.handleAPIKeys

	body := `{"name":"test","role":"operator","scopes":["bogus_scope"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid scopes, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAPIKeyActions_GetByID(t *testing.T) {
	s := newTestAPIServer(t)

	// Create a key first
	body := `{"name":"get-test","role":"operator","scopes":["assets:read"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleAPIKeys(rec, req)

	var createResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	data := createResp["data"].(map[string]any)
	keyID := data["id"].(string)

	// GET by ID
	getReq := httptest.NewRequest("GET", "/api/v2/keys/"+keyID, nil)
	getReq = getReq.WithContext(contextWithPrincipal(getReq.Context(), "admin", "admin"))
	getRec := httptest.NewRecorder()
	s.handleAPIKeyActions(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var getResp map[string]any
	json.Unmarshal(getRec.Body.Bytes(), &getResp)
	getData := getResp["data"].(map[string]any)
	if getData["name"] != "get-test" {
		t.Errorf("name = %v, want get-test", getData["name"])
	}
	// raw_key should NOT be in GET response
	if getData["raw_key"] != nil {
		t.Error("GET response should not include raw_key")
	}
}

func TestHandleAPIKeyActions_GetNotFound(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v2/keys/nonexistent", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleAPIKeyActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleAPIKeyActions_Delete(t *testing.T) {
	s := newTestAPIServer(t)

	// Create a key
	body := `{"name":"delete-test","role":"viewer","scopes":["assets:read"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleAPIKeys(rec, req)

	var createResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	keyID := createResp["data"].(map[string]any)["id"].(string)

	// DELETE
	delReq := httptest.NewRequest("DELETE", "/api/v2/keys/"+keyID, nil)
	delReq = delReq.WithContext(contextWithPrincipal(delReq.Context(), "admin", "admin"))
	delRec := httptest.NewRecorder()
	s.handleAPIKeyActions(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d", delRec.Code)
	}

	// GET after delete should 404
	getReq := httptest.NewRequest("GET", "/api/v2/keys/"+keyID, nil)
	getReq = getReq.WithContext(contextWithPrincipal(getReq.Context(), "admin", "admin"))
	getRec := httptest.NewRecorder()
	s.handleAPIKeyActions(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE: expected 404, got %d", getRec.Code)
	}
}

func TestHandleAPIKeyActions_Patch(t *testing.T) {
	s := newTestAPIServer(t)

	// Create
	body := `{"name":"patch-test","role":"operator","scopes":["assets:read"]}`
	req := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleAPIKeys(rec, req)

	var createResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &createResp)
	keyID := createResp["data"].(map[string]any)["id"].(string)

	// PATCH — update scopes
	patchBody := `{"scopes":["assets:read","docker:read"]}`
	patchReq := httptest.NewRequest("PATCH", "/api/v2/keys/"+keyID, strings.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = patchReq.WithContext(contextWithPrincipal(patchReq.Context(), "admin", "admin"))
	patchRec := httptest.NewRecorder()
	s.handleAPIKeyActions(patchRec, patchReq)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH: expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
}
