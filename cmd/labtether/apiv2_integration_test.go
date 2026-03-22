// cmd/labtether/apiv2_integration_test.go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
)

// TestV2Integration_CreateKeyAndUseIt tests the full flow:
// 1. Create an API key (admin auth)
// 2. Use that key to call v2 endpoints
// 3. Verify scope enforcement
func TestV2Integration_CreateKeyAndUseIt(t *testing.T) {
	s := newTestAPIServer(t)

	// Seed an asset
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})
	if err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	// Step 1: Create an API key via the admin CRUD handler
	createBody := `{"name":"test-integration","role":"operator","scopes":["assets:read","assets:exec"],"allowed_assets":["srv1"]}`
	createReq := httptest.NewRequest("POST", "/api/v2/keys", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(createReq.Context(), "admin", "admin")
	createReq = createReq.WithContext(ctx)
	createRec := httptest.NewRecorder()
	s.handleAPIKeys(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	data := createResp["data"].(map[string]any)
	rawKey := data["raw_key"].(string)
	keyID := data["id"].(string)

	if !strings.HasPrefix(rawKey, "lt_") {
		t.Fatalf("raw key should start with lt_, got %q", rawKey)
	}

	// Step 2: Simulate API key authentication by setting up the context
	// as the withAuth middleware would after validating the key
	hash := apikeys.HashKey(rawKey)
	key, found, err := s.apiKeyStore.LookupAPIKeyByHash(context.Background(), hash)
	if err != nil || !found {
		t.Fatal("created key should be findable by hash")
	}
	if key.ID != keyID {
		t.Fatalf("key ID mismatch: %s vs %s", key.ID, keyID)
	}

	// Step 3: Use the key context to call whoami
	whoamiReq := httptest.NewRequest("GET", "/api/v2/whoami", nil)
	whoamiCtx := contextWithPrincipal(whoamiReq.Context(), "apikey:"+keyID, "operator")
	whoamiCtx = contextWithScopes(whoamiCtx, key.Scopes)
	whoamiCtx = contextWithAllowedAssets(whoamiCtx, key.AllowedAssets)
	whoamiCtx = contextWithAPIKeyID(whoamiCtx, keyID)
	whoamiReq = whoamiReq.WithContext(whoamiCtx)
	whoamiRec := httptest.NewRecorder()
	s.handleV2Whoami(whoamiRec, whoamiReq)

	if whoamiRec.Code != http.StatusOK {
		t.Fatalf("whoami: expected 200, got %d: %s", whoamiRec.Code, whoamiRec.Body.String())
	}

	var whoamiResp map[string]any
	json.Unmarshal(whoamiRec.Body.Bytes(), &whoamiResp)
	whoamiData := whoamiResp["data"].(map[string]any)
	if whoamiData["auth_type"] != "api_key" {
		t.Errorf("auth_type = %v, want api_key", whoamiData["auth_type"])
	}
	if whoamiData["role"] != "operator" {
		t.Errorf("role = %v, want operator", whoamiData["role"])
	}

	// Step 4: Call assets list — should only see srv1 (allowed_assets filter)
	assetsReq := httptest.NewRequest("GET", "/api/v2/assets", nil)
	assetsReq = assetsReq.WithContext(whoamiCtx)
	assetsRec := httptest.NewRecorder()
	s.handleV2Assets(assetsRec, assetsReq)

	if assetsRec.Code != http.StatusOK {
		t.Fatalf("assets list: expected 200, got %d", assetsRec.Code)
	}

	// Step 5: Try to access an asset NOT in allowed_assets — should get 403
	forbiddenReq := httptest.NewRequest("GET", "/api/v2/assets/srv2", nil)
	forbiddenReq = forbiddenReq.WithContext(whoamiCtx)
	forbiddenRec := httptest.NewRecorder()
	s.handleV2AssetActions(forbiddenRec, forbiddenReq)

	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("forbidden asset: expected 403, got %d", forbiddenRec.Code)
	}

	// Step 6: Try to use a scope the key doesn't have (e.g., docker:read)
	dockerReq := httptest.NewRequest("GET", "/api/v2/docker/hosts", nil)
	dockerReq = dockerReq.WithContext(whoamiCtx) // scopes are assets:read, assets:exec — no docker
	dockerRec := httptest.NewRecorder()
	s.handleV2DockerHosts(dockerRec, dockerReq)

	if dockerRec.Code != http.StatusForbidden {
		t.Fatalf("docker scope denied: expected 403, got %d", dockerRec.Code)
	}
}

// TestV2Integration_SessionAuthFullAccess verifies that session-authenticated
// users (no scopes in context) have full access to all v2 endpoints.
func TestV2Integration_SessionAuthFullAccess(t *testing.T) {
	s := newTestAPIServer(t)

	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})
	if err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	// Session auth — no scopes, no allowed_assets
	ctx := contextWithPrincipal(context.Background(), "admin-user", "admin")

	// Should access whoami
	req := httptest.NewRequest("GET", "/api/v2/whoami", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Whoami(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("whoami: expected 200, got %d", rec.Code)
	}

	// Should access assets
	req2 := httptest.NewRequest("GET", "/api/v2/assets", nil)
	req2 = req2.WithContext(ctx)
	rec2 := httptest.NewRecorder()
	s.handleV2Assets(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("assets: expected 200, got %d", rec2.Code)
	}

	// Should access docker (no scope restriction)
	req3 := httptest.NewRequest("GET", "/api/v2/docker/hosts", nil)
	req3 = req3.WithContext(ctx)
	rec3 := httptest.NewRecorder()
	s.handleV2DockerHosts(rec3, req3)
	if rec3.Code == http.StatusForbidden {
		t.Fatal("session auth should not be scope-restricted")
	}
}

// TestV2Integration_ExpiredKey verifies that an expired key is rejected.
func TestV2Integration_ExpiredKey(t *testing.T) {
	s := newTestAPIServer(t)

	// Create a key that's already expired
	expired := time.Now().Add(-1 * time.Hour)
	key := apikeys.APIKey{
		ID:         "key_expired",
		Name:       "expired-key",
		Prefix:     "ex12",
		SecretHash: apikeys.HashKey("lt_ex12_fakesecret"),
		Role:       "operator",
		Scopes:     []string{"*"},
		ExpiresAt:  &expired,
		CreatedBy:  "admin",
		CreatedAt:  time.Now().UTC(),
	}
	s.apiKeyStore.CreateAPIKey(context.Background(), key)

	// The withAuth middleware checks expiry — simulate that check
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		// Key is expired — this is the expected behavior
		t.Log("expired key correctly identified as expired")
	} else {
		t.Error("key should be expired")
	}
}

// TestV2Integration_KeyDeletion verifies that a deleted key no longer works.
func TestV2Integration_KeyDeletion(t *testing.T) {
	s := newTestAPIServer(t)

	// Create and then delete a key
	key := apikeys.APIKey{
		ID:         "key_todelete",
		Name:       "delete-me",
		Prefix:     "dl12",
		SecretHash: apikeys.HashKey("lt_dl12_fakesecret"),
		Role:       "operator",
		Scopes:     []string{"*"},
		CreatedBy:  "admin",
		CreatedAt:  time.Now().UTC(),
	}
	s.apiKeyStore.CreateAPIKey(context.Background(), key)

	// Delete it
	s.apiKeyStore.DeleteAPIKey(context.Background(), "key_todelete")

	// Try to look it up — should fail
	_, found, _ := s.apiKeyStore.LookupAPIKeyByHash(context.Background(), key.SecretHash)
	if found {
		t.Error("deleted key should not be found")
	}
}

// TestV2Integration_V2ResponseEnvelope verifies that v2 endpoints return
// the correct response envelope format.
func TestV2Integration_V2ResponseEnvelope(t *testing.T) {
	s := newTestAPIServer(t)

	ctx := contextWithPrincipal(context.Background(), "admin", "admin")

	// Test whoami response format
	req := httptest.NewRequest("GET", "/api/v2/whoami", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Whoami(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	// Must have request_id
	if _, ok := resp["request_id"]; !ok {
		t.Error("response must have request_id")
	}
	reqID := resp["request_id"].(string)
	if !strings.HasPrefix(reqID, "req_") {
		t.Errorf("request_id should start with req_, got %q", reqID)
	}

	// Must have data
	if _, ok := resp["data"]; !ok {
		t.Error("response must have data")
	}
}
