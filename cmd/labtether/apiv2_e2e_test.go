package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
)

// TestE2E_FullAPIKeyLifecycle exercises the complete gateway through the real
// HTTP handler chain: route dispatch → withAuth middleware → scope enforcement
// → handler → response. No context injection, no mocking — real HTTP requests
// with real auth headers.
func TestE2E_FullAPIKeyLifecycle(t *testing.T) {
	const ownerToken = "e2e-test-owner-token-secret"

	// Build a test server with all routes wired.
	s := newTestAPIServer(t)
	s.authValidator = auth.NewTokenValidator(ownerToken)
	s.tlsState.Enabled = true // pretend TLS is enabled so API keys are accepted

	handlers := s.buildHTTPHandlers(nil, nil, nil)
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	srv := httptest.NewTLSServer(mux) // TLS server so r.TLS is non-nil
	defer srv.Close()

	client := srv.Client()

	// Seed assets for testing.
	s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})
	s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv2", Name: "Server 2", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})

	// Helper: make an authenticated request.
	doReq := func(t *testing.T, method, path, token string, body string) *http.Response {
		t.Helper()
		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, reqBody)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}

	// Helper: parse response body as JSON.
	parseResp := func(t *testing.T, resp *http.Response) map[string]any {
		t.Helper()
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("parse response: %v (body: %s)", err, string(data))
		}
		return result
	}

	// ─── Step 1: Create an API key via owner auth ───────────────────────

	t.Run("create_api_key", func(t *testing.T) {
		resp := doReq(t, "POST", "/api/v2/keys",
			ownerToken,
			`{"name":"e2e-test-key","role":"operator","scopes":["assets:read","assets:exec"],"allowed_assets":["srv1"]}`,
		)
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("create key: expected 201, got %d: %s", resp.StatusCode, string(body))
		}

		result := parseResp(t, resp)
		data := result["data"].(map[string]any)

		rawKey := data["raw_key"].(string)
		if !strings.HasPrefix(rawKey, "lt_") {
			t.Fatalf("raw_key should start with lt_, got %q", rawKey)
		}

		// Store the key for subsequent tests.
		t.Setenv("E2E_RAW_KEY", rawKey)
		t.Setenv("E2E_KEY_ID", data["id"].(string))
	})

	// Get the key from the first sub-test. Since t.Setenv doesn't work
	// across subtests, we'll create the key inline and pass it around.
	createResp := doReq(t, "POST", "/api/v2/keys",
		ownerToken,
		`{"name":"e2e-key","role":"operator","scopes":["assets:read","assets:exec"],"allowed_assets":["srv1"]}`,
	)
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create key: expected 201, got %d: %s", createResp.StatusCode, string(body))
	}
	createData := parseResp(t, createResp)
	rawKey := createData["data"].(map[string]any)["raw_key"].(string)
	keyID := createData["data"].(map[string]any)["id"].(string)

	// ─── Step 2: Whoami with API key ────────────────────────────────────

	t.Run("whoami_with_api_key", func(t *testing.T) {
		resp := doReq(t, "GET", "/api/v2/whoami", rawKey, "")
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("whoami: expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		result := parseResp(t, resp)

		// Must have request_id
		if result["request_id"] == nil {
			t.Error("response should have request_id")
		}

		data := result["data"].(map[string]any)
		if data["auth_type"] != "api_key" {
			t.Errorf("auth_type = %v, want api_key", data["auth_type"])
		}
		if data["role"] != "operator" {
			t.Errorf("role = %v, want operator", data["role"])
		}
		if data["key_name"] != "e2e-key" {
			t.Errorf("key_name = %v, want e2e-key", data["key_name"])
		}
	})

	// ─── Step 3: List assets (filtered by allowlist) ────────────────────

	t.Run("list_assets_filtered", func(t *testing.T) {
		resp := doReq(t, "GET", "/api/v2/assets", rawKey, "")
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("list assets: expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		result := parseResp(t, resp)
		data := result["data"].([]any)

		// Should only see srv1 (allowed), not srv2
		if len(data) != 1 {
			t.Fatalf("expected 1 asset (srv1 only), got %d", len(data))
		}
		asset := data[0].(map[string]any)
		if asset["id"] != "srv1" {
			t.Errorf("expected srv1, got %v", asset["id"])
		}
	})

	// ─── Step 4: Exec on allowed asset ──────────────────────────────────

	t.Run("exec_allowed_asset", func(t *testing.T) {
		resp := doReq(t, "POST", "/api/v2/assets/srv1/exec", rawKey, `{"command":"uptime"}`)
		// Should get 409 (no agent connected), NOT 403
		if resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("exec on allowed asset: expected 409 (no agent), got %d: %s", resp.StatusCode, string(body))
		}

		result := parseResp(t, resp)
		if result["error"] != "asset_offline" {
			t.Errorf("error = %v, want asset_offline", result["error"])
		}
	})

	// ─── Step 5: Exec on denied asset ───────────────────────────────────

	t.Run("exec_denied_asset", func(t *testing.T) {
		resp := doReq(t, "POST", "/api/v2/assets/srv2/exec", rawKey, `{"command":"uptime"}`)
		if resp.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("exec on denied asset: expected 403, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// ─── Step 6: Docker scope denied ────────────────────────────────────

	t.Run("docker_scope_denied", func(t *testing.T) {
		resp := doReq(t, "GET", "/api/v2/docker/hosts", rawKey, "")
		if resp.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("docker without scope: expected 403, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// ─── Step 7: Invalid key rejected ───────────────────────────────────

	t.Run("invalid_key_rejected", func(t *testing.T) {
		resp := doReq(t, "GET", "/api/v2/whoami", "lt_fake_notarealkey1234567890", "")
		if resp.StatusCode != http.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("invalid key: expected 401, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// ─── Step 8: Delete key, then verify rejection ──────────────────────

	t.Run("deleted_key_rejected", func(t *testing.T) {
		// Delete via owner auth
		delResp := doReq(t, "DELETE", "/api/v2/keys/"+keyID, ownerToken, "")
		if delResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(delResp.Body)
			t.Fatalf("delete key: expected 200, got %d: %s", delResp.StatusCode, string(body))
		}
		delResp.Body.Close()

		// Try to use the deleted key
		resp := doReq(t, "GET", "/api/v2/whoami", rawKey, "")
		if resp.StatusCode != http.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("deleted key: expected 401, got %d: %s", resp.StatusCode, string(body))
		}
		resp.Body.Close()
	})

	// ─── Step 9: Expired key rejected ───────────────────────────────────

	t.Run("expired_key_rejected", func(t *testing.T) {
		// Create a key with an expiry in the past
		expired := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		body := `{"name":"expired-key","role":"operator","scopes":["assets:read"],"expires_at":"` + expired + `"}`
		createResp := doReq(t, "POST", "/api/v2/keys", ownerToken, body)
		if createResp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(createResp.Body)
			t.Fatalf("create expired key: expected 201, got %d: %s", createResp.StatusCode, string(b))
		}
		expData := parseResp(t, createResp)
		expKey := expData["data"].(map[string]any)["raw_key"].(string)

		// Try to use the expired key
		resp := doReq(t, "GET", "/api/v2/whoami", expKey, "")
		if resp.StatusCode != http.StatusUnauthorized {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expired key: expected 401, got %d: %s", resp.StatusCode, string(b))
		}
		resp.Body.Close()
	})

	// ─── Step 10: v2 response envelope verified ─────────────────────────

	t.Run("response_envelope_format", func(t *testing.T) {
		// Create a fresh key for this test
		createResp := doReq(t, "POST", "/api/v2/keys", ownerToken,
			`{"name":"envelope-test","role":"viewer","scopes":["assets:read"]}`)
		envData := parseResp(t, createResp)
		envKey := envData["data"].(map[string]any)["raw_key"].(string)

		// Call whoami and verify envelope
		resp := doReq(t, "GET", "/api/v2/whoami", envKey, "")
		result := parseResp(t, resp)

		if result["request_id"] == nil {
			t.Error("missing request_id in response")
		}
		reqID, ok := result["request_id"].(string)
		if !ok || !strings.HasPrefix(reqID, "req_") {
			t.Errorf("request_id should start with req_, got %v", result["request_id"])
		}
		if result["data"] == nil {
			t.Error("missing data in response")
		}
	})
}

// TestE2E_OwnerAuthFullAccess verifies that owner-token auth has full
// access to all v2 endpoints without scope restrictions.
func TestE2E_OwnerAuthFullAccess(t *testing.T) {
	const ownerToken = "e2e-owner-token"

	s := newTestAPIServer(t)
	s.authValidator = auth.NewTokenValidator(ownerToken)
	s.tlsState.Enabled = true

	handlers := s.buildHTTPHandlers(nil, nil, nil)
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	client := srv.Client()

	doGet := func(path string) int {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// Owner should access all endpoints without scope restrictions
	endpoints := []string{
		"/api/v2/whoami",
		"/api/v2/assets",
		"/api/v2/docker/hosts",
		"/api/v2/groups",
		"/api/v2/alerts",
		"/api/v2/connectors",
		"/api/v2/metrics/overview",
	}

	for _, ep := range endpoints {
		code := doGet(ep)
		if code == http.StatusForbidden || code == http.StatusUnauthorized {
			t.Errorf("owner should have access to %s, got %d", ep, code)
		}
	}
}

// TestE2E_NonTLSRejectsAPIKeys verifies that API keys are rejected
// when TLS is not enabled and the request arrives over plain HTTP.
func TestE2E_NonTLSRejectsAPIKeys(t *testing.T) {
	const ownerToken = "e2e-owner-tls"

	s := newTestAPIServer(t)
	s.authValidator = auth.NewTokenValidator(ownerToken)
	s.tlsState.Enabled = false // TLS disabled

	handlers := s.buildHTTPHandlers(nil, nil, nil)
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	srv := httptest.NewServer(mux) // plain HTTP, not TLS
	defer srv.Close()
	client := srv.Client()

	// First create a key using owner auth (owner works over HTTP)
	createBody := `{"name":"tls-test","role":"operator","scopes":["assets:read"]}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/v2/keys", strings.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key: expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	defer resp.Body.Close()
	var createData map[string]any
	json.NewDecoder(resp.Body).Decode(&createData)
	rawKey := createData["data"].(map[string]any)["raw_key"].(string)

	// Now try to use the API key over plain HTTP — should be rejected
	req2, _ := http.NewRequest("GET", srv.URL+"/api/v2/whoami", nil)
	req2.Header.Set("Authorization", "Bearer "+rawKey)
	resp2, err2 := client.Do(req2)
	if err2 != nil {
		t.Fatalf("API key on plain HTTP: request failed: %v", err2)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("API key on plain HTTP: expected 403, got %d: %s", resp2.StatusCode, string(body))
	}
}
