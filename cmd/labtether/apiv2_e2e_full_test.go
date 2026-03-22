package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
)

// TestE2E_FullFunctionality exercises every v2 endpoint category through the
// real HTTP handler chain. One test server is created, assets are seeded once,
// and sub-tests cover each endpoint family end-to-end.
func TestE2E_FullFunctionality(t *testing.T) {
	const ownerToken = "e2e-full-owner-token"

	// ── Build the test server ──────────────────────────────────────────
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

	// ── Seed 2 assets ─────────────────────────────────────────────────
	s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "Server 1", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})
	s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv2", Name: "Server 2", Source: "agent", Type: "host",
		Status: "online", Platform: "linux",
	})

	// ── Shared helpers ─────────────────────────────────────────────────

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

	parseJSON := func(t *testing.T, resp *http.Response) map[string]any {
		t.Helper()
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse JSON: %v (body: %s)", err, string(raw))
		}
		return m
	}

	mustStatus := func(t *testing.T, resp *http.Response, want int) {
		t.Helper()
		if resp.StatusCode != want {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("expected %d, got %d: %s", want, resp.StatusCode, string(raw))
		}
	}

	notAuthError := func(t *testing.T, resp *http.Response) {
		t.Helper()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("expected non-auth status, got %d: %s", resp.StatusCode, string(raw))
		}
		resp.Body.Close()
	}

	// Create a wildcard key once for delegated-endpoint smoke tests.
	createWildcardKey := func(t *testing.T, name string) string {
		t.Helper()
		body := `{"name":"` + name + `","role":"operator","scopes":["*"]}`
		resp := doReq(t, "POST", "/api/v2/keys", ownerToken, body)
		mustStatus(t, resp, http.StatusCreated)
		m := parseJSON(t, resp)
		return m["data"].(map[string]any)["raw_key"].(string)
	}

	// ── 1. Key Management CRUD ─────────────────────────────────────────
	t.Run("key_management_crud", func(t *testing.T) {
		// Create
		createResp := doReq(t, "POST", "/api/v2/keys", ownerToken,
			`{"name":"crud-key","role":"operator","scopes":["assets:read"]}`)
		mustStatus(t, createResp, http.StatusCreated)
		createData := parseJSON(t, createResp)
		keyData := createData["data"].(map[string]any)
		keyID := keyData["id"].(string)

		if !strings.HasPrefix(keyData["raw_key"].(string), "lt_") {
			t.Fatalf("raw_key should start with lt_, got %q", keyData["raw_key"])
		}

		// List — verify created key appears
		listResp := doReq(t, "GET", "/api/v2/keys", ownerToken, "")
		mustStatus(t, listResp, http.StatusOK)
		listData := parseJSON(t, listResp)
		items := listData["data"].([]any)
		found := false
		for _, item := range items {
			if item.(map[string]any)["id"] == keyID {
				found = true
			}
		}
		if !found {
			t.Fatalf("created key %s not found in list", keyID)
		}

		// Get by ID
		getResp := doReq(t, "GET", "/api/v2/keys/"+keyID, ownerToken, "")
		mustStatus(t, getResp, http.StatusOK)
		getData := parseJSON(t, getResp)
		if getData["data"].(map[string]any)["id"] != keyID {
			t.Fatalf("get key: expected id %s, got %v", keyID, getData["data"].(map[string]any)["id"])
		}

		// Patch (update scopes)
		patchResp := doReq(t, "PATCH", "/api/v2/keys/"+keyID, ownerToken,
			`{"scopes":["assets:read","assets:exec"]}`)
		mustStatus(t, patchResp, http.StatusOK)
		patchData := parseJSON(t, patchResp)
		if patchData["data"].(map[string]any)["status"] != "updated" {
			t.Fatalf("patch key: expected status=updated, got %v", patchData["data"])
		}

		// Delete
		delResp := doReq(t, "DELETE", "/api/v2/keys/"+keyID, ownerToken, "")
		mustStatus(t, delResp, http.StatusOK)
		delData := parseJSON(t, delResp)
		if delData["data"].(map[string]any)["status"] != "deleted" {
			t.Fatalf("delete key: expected status=deleted, got %v", delData["data"])
		}

		// Verify deletion: get should return 404
		getAfterDel := doReq(t, "GET", "/api/v2/keys/"+keyID, ownerToken, "")
		if getAfterDel.StatusCode != http.StatusNotFound {
			raw, _ := io.ReadAll(getAfterDel.Body)
			getAfterDel.Body.Close()
			t.Fatalf("expected 404 after deletion, got %d: %s", getAfterDel.StatusCode, string(raw))
		}
		getAfterDel.Body.Close()
	})

	// ── 2. Assets ──────────────────────────────────────────────────────
	t.Run("assets", func(t *testing.T) {
		// List — both seeded assets
		listResp := doReq(t, "GET", "/api/v2/assets", ownerToken, "")
		mustStatus(t, listResp, http.StatusOK)
		listData := parseJSON(t, listResp)
		items := listData["data"].([]any)
		if len(items) < 2 {
			t.Fatalf("expected at least 2 assets, got %d", len(items))
		}

		// Get asset by ID
		getResp := doReq(t, "GET", "/api/v2/assets/srv1", ownerToken, "")
		mustStatus(t, getResp, http.StatusOK)
		getData := parseJSON(t, getResp)
		if getData["data"].(map[string]any)["asset"].(map[string]any)["id"] != "srv1" {
			t.Fatalf("get asset: expected id=srv1, got %v", getData["data"])
		}

		// Get nonexistent asset — 404
		notFoundResp := doReq(t, "GET", "/api/v2/assets/does-not-exist-9999", ownerToken, "")
		if notFoundResp.StatusCode != http.StatusNotFound {
			raw, _ := io.ReadAll(notFoundResp.Body)
			notFoundResp.Body.Close()
			t.Fatalf("expected 404 for nonexistent asset, got %d: %s", notFoundResp.StatusCode, string(raw))
		}
		notFoundResp.Body.Close()

		// Create asset — 201
		createResp := doReq(t, "POST", "/api/v2/assets", ownerToken,
			`{"name":"e2e-created-asset","platform":"linux"}`)
		mustStatus(t, createResp, http.StatusCreated)
		createData := parseJSON(t, createResp)
		newID := createData["data"].(map[string]any)["id"].(string)
		if newID == "" {
			t.Fatal("created asset should have a non-empty id")
		}

		// Delete the created asset
		delResp := doReq(t, "DELETE", "/api/v2/assets/"+newID, ownerToken, "")
		mustStatus(t, delResp, http.StatusOK)
		delData := parseJSON(t, delResp)
		if delData["data"].(map[string]any)["status"] != "deleted" {
			t.Fatalf("delete asset: expected status=deleted, got %v", delData["data"])
		}
	})

	// ── 3. Exec ────────────────────────────────────────────────────────
	t.Run("exec", func(t *testing.T) {
		// Single target — no agent connected → 409
		singleResp := doReq(t, "POST", "/api/v2/assets/srv1/exec", ownerToken,
			`{"command":"uptime"}`)
		if singleResp.StatusCode != http.StatusConflict {
			raw, _ := io.ReadAll(singleResp.Body)
			singleResp.Body.Close()
			t.Fatalf("exec single: expected 409 (no agent), got %d: %s", singleResp.StatusCode, string(raw))
		}
		singleData := parseJSON(t, singleResp)
		if singleData["error"] != "asset_offline" {
			t.Fatalf("exec single: expected error=asset_offline, got %v", singleData["error"])
		}

		// Multi-target fan-out — agents offline but route should respond 200 with results
		multiResp := doReq(t, "POST", "/api/v2/exec", ownerToken,
			`{"targets":["srv1","srv2"],"command":"hostname"}`)
		mustStatus(t, multiResp, http.StatusOK)
		multiData := parseJSON(t, multiResp)
		results, ok := multiData["data"].(map[string]any)["results"]
		if !ok || results == nil {
			t.Fatalf("exec multi: expected results in response, got %v", multiData["data"])
		}

		// Missing command — 400
		badResp := doReq(t, "POST", "/api/v2/assets/srv1/exec", ownerToken, `{}`)
		if badResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(badResp.Body)
			badResp.Body.Close()
			t.Fatalf("exec missing cmd: expected 400, got %d: %s", badResp.StatusCode, string(raw))
		}
		badResp.Body.Close()
	})

	// ── 4. Scoped Access ───────────────────────────────────────────────
	t.Run("scoped_access", func(t *testing.T) {
		// Create a key with ONLY assets:read scope (and allow all assets via empty list = wildcard)
		scopeResp := doReq(t, "POST", "/api/v2/keys", ownerToken,
			`{"name":"assets-read-only","role":"operator","scopes":["assets:read"]}`)
		mustStatus(t, scopeResp, http.StatusCreated)
		scopeData := parseJSON(t, scopeResp)
		narrowKey := scopeData["data"].(map[string]any)["raw_key"].(string)

		// GET /api/v2/assets → 200 (has assets:read)
		assetsResp := doReq(t, "GET", "/api/v2/assets", narrowKey, "")
		mustStatus(t, assetsResp, http.StatusOK)
		assetsResp.Body.Close()

		// POST /api/v2/assets/srv1/exec → 403 (no exec scope)
		execResp := doReq(t, "POST", "/api/v2/assets/srv1/exec", narrowKey, `{"command":"uptime"}`)
		if execResp.StatusCode != http.StatusForbidden {
			raw, _ := io.ReadAll(execResp.Body)
			execResp.Body.Close()
			t.Fatalf("scoped exec: expected 403, got %d: %s", execResp.StatusCode, string(raw))
		}
		execResp.Body.Close()

		// GET /api/v2/docker/hosts → 403 (no docker scope)
		dockerResp := doReq(t, "GET", "/api/v2/docker/hosts", narrowKey, "")
		if dockerResp.StatusCode != http.StatusForbidden {
			raw, _ := io.ReadAll(dockerResp.Body)
			dockerResp.Body.Close()
			t.Fatalf("scoped docker: expected 403, got %d: %s", dockerResp.StatusCode, string(raw))
		}
		dockerResp.Body.Close()

		// GET /api/v2/groups → 403 (no groups scope)
		groupsResp := doReq(t, "GET", "/api/v2/groups", narrowKey, "")
		if groupsResp.StatusCode != http.StatusForbidden {
			raw, _ := io.ReadAll(groupsResp.Body)
			groupsResp.Body.Close()
			t.Fatalf("scoped groups: expected 403, got %d: %s", groupsResp.StatusCode, string(raw))
		}
		groupsResp.Body.Close()

		// GET /api/v2/alerts → 403 (no alerts scope)
		alertsResp := doReq(t, "GET", "/api/v2/alerts", narrowKey, "")
		if alertsResp.StatusCode != http.StatusForbidden {
			raw, _ := io.ReadAll(alertsResp.Body)
			alertsResp.Body.Close()
			t.Fatalf("scoped alerts: expected 403, got %d: %s", alertsResp.StatusCode, string(raw))
		}
		alertsResp.Body.Close()
	})

	// ── 5. Webhooks ────────────────────────────────────────────────────
	t.Run("webhooks", func(t *testing.T) {
		// Create — 201
		createResp := doReq(t, "POST", "/api/v2/webhooks", ownerToken,
			`{"name":"e2e-webhook","url":"https://example.com/hook","events":["asset.online"]}`)
		mustStatus(t, createResp, http.StatusCreated)
		createData := parseJSON(t, createResp)
		whID := createData["data"].(map[string]any)["id"].(string)
		if whID == "" {
			t.Fatal("created webhook should have a non-empty id")
		}

		// List — verify created webhook appears
		listResp := doReq(t, "GET", "/api/v2/webhooks", ownerToken, "")
		mustStatus(t, listResp, http.StatusOK)
		listData := parseJSON(t, listResp)
		items := listData["data"].([]any)
		found := false
		for _, item := range items {
			if item.(map[string]any)["id"] == whID {
				found = true
			}
		}
		if !found {
			t.Fatalf("created webhook %s not found in list", whID)
		}

		// Delete
		delResp := doReq(t, "DELETE", "/api/v2/webhooks/"+whID, ownerToken, "")
		mustStatus(t, delResp, http.StatusOK)
		delData := parseJSON(t, delResp)
		if delData["data"].(map[string]any)["status"] != "deleted" {
			t.Fatalf("delete webhook: expected status=deleted, got %v", delData["data"])
		}

		// Invalid URL scheme — 400
		badResp := doReq(t, "POST", "/api/v2/webhooks", ownerToken,
			`{"name":"bad-scheme","url":"ftp://evil.internal/hook","events":[]}`)
		if badResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(badResp.Body)
			badResp.Body.Close()
			t.Fatalf("webhook invalid scheme: expected 400, got %d: %s", badResp.StatusCode, string(raw))
		}
		badResp.Body.Close()

		// Scope denied (narrow key without webhooks scope)
		narrowResp := doReq(t, "POST", "/api/v2/keys", ownerToken,
			`{"name":"no-webhooks-scope","role":"operator","scopes":["assets:read"]}`)
		mustStatus(t, narrowResp, http.StatusCreated)
		narrowData := parseJSON(t, narrowResp)
		narrowKey := narrowData["data"].(map[string]any)["raw_key"].(string)

		scopeDenyResp := doReq(t, "GET", "/api/v2/webhooks", narrowKey, "")
		if scopeDenyResp.StatusCode != http.StatusForbidden {
			raw, _ := io.ReadAll(scopeDenyResp.Body)
			scopeDenyResp.Body.Close()
			t.Fatalf("webhooks without scope: expected 403, got %d: %s", scopeDenyResp.StatusCode, string(raw))
		}
		scopeDenyResp.Body.Close()
	})

	// ── 6. Schedules ───────────────────────────────────────────────────
	t.Run("schedules", func(t *testing.T) {
		// Create — 201
		createResp := doReq(t, "POST", "/api/v2/schedules", ownerToken,
			`{"name":"e2e-schedule","cron_expr":"0 * * * *","command":"uptime","targets":["srv1"]}`)
		mustStatus(t, createResp, http.StatusCreated)
		createData := parseJSON(t, createResp)
		schedID := createData["data"].(map[string]any)["id"].(string)
		if schedID == "" {
			t.Fatal("created schedule should have a non-empty id")
		}

		// List
		listResp := doReq(t, "GET", "/api/v2/schedules", ownerToken, "")
		mustStatus(t, listResp, http.StatusOK)
		listData := parseJSON(t, listResp)
		items := listData["data"].([]any)
		found := false
		for _, item := range items {
			if item.(map[string]any)["id"] == schedID {
				found = true
			}
		}
		if !found {
			t.Fatalf("created schedule %s not found in list", schedID)
		}

		// Delete
		delResp := doReq(t, "DELETE", "/api/v2/schedules/"+schedID, ownerToken, "")
		mustStatus(t, delResp, http.StatusOK)
		delData := parseJSON(t, delResp)
		if delData["data"].(map[string]any)["status"] != "deleted" {
			t.Fatalf("delete schedule: expected status=deleted, got %v", delData["data"])
		}

		// Missing cron_expr — 400
		badResp := doReq(t, "POST", "/api/v2/schedules", ownerToken,
			`{"name":"missing-cron","command":"hostname"}`)
		if badResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(badResp.Body)
			badResp.Body.Close()
			t.Fatalf("schedule missing cron: expected 400, got %d: %s", badResp.StatusCode, string(raw))
		}
		badResp.Body.Close()
	})

	// ── 7. Saved Actions ───────────────────────────────────────────────
	t.Run("saved_actions", func(t *testing.T) {
		// Create — 201
		createResp := doReq(t, "POST", "/api/v2/actions", ownerToken,
			`{"name":"e2e-action","steps":[{"name":"step1","target":"srv1","command":"uptime"}]}`)
		mustStatus(t, createResp, http.StatusCreated)
		createData := parseJSON(t, createResp)
		actID := createData["data"].(map[string]any)["id"].(string)
		if actID == "" {
			t.Fatal("created action should have a non-empty id")
		}

		// List
		listResp := doReq(t, "GET", "/api/v2/actions", ownerToken, "")
		mustStatus(t, listResp, http.StatusOK)
		listData := parseJSON(t, listResp)
		items := listData["data"].([]any)
		found := false
		for _, item := range items {
			if item.(map[string]any)["id"] == actID {
				found = true
			}
		}
		if !found {
			t.Fatalf("created action %s not found in list", actID)
		}

		// Delete
		delResp := doReq(t, "DELETE", "/api/v2/actions/"+actID, ownerToken, "")
		mustStatus(t, delResp, http.StatusOK)
		delData := parseJSON(t, delResp)
		if delData["data"].(map[string]any)["status"] != "deleted" {
			t.Fatalf("delete action: expected status=deleted, got %v", delData["data"])
		}
	})

	// ── 8. Search ──────────────────────────────────────────────────────
	t.Run("search", func(t *testing.T) {
		// Search with query — 200
		searchResp := doReq(t, "GET", "/api/v2/search?q=srv", ownerToken, "")
		mustStatus(t, searchResp, http.StatusOK)
		searchData := parseJSON(t, searchResp)
		if searchData["data"] == nil {
			t.Fatal("search: expected data in response")
		}

		// Search without query — 400
		noQueryResp := doReq(t, "GET", "/api/v2/search", ownerToken, "")
		if noQueryResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(noQueryResp.Body)
			noQueryResp.Body.Close()
			t.Fatalf("search without q: expected 400, got %d: %s", noQueryResp.StatusCode, string(raw))
		}
		noQueryResp.Body.Close()
	})

	// ── 9. Bulk Operations ─────────────────────────────────────────────
	t.Run("bulk_operations", func(t *testing.T) {
		// Invalid action rejected — 400
		invalidActionResp := doReq(t, "POST", "/api/v2/bulk/service-action", ownerToken,
			`{"action":"hax0r","service":"nginx","targets":["srv1"]}`)
		if invalidActionResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(invalidActionResp.Body)
			invalidActionResp.Body.Close()
			t.Fatalf("bulk invalid action: expected 400, got %d: %s", invalidActionResp.StatusCode, string(raw))
		}
		invalidActionResp.Body.Close()

		// Injection in service name rejected — 400
		injectionResp := doReq(t, "POST", "/api/v2/bulk/service-action", ownerToken,
			`{"action":"restart","service":"nginx; rm -rf /","targets":["srv1"]}`)
		if injectionResp.StatusCode != http.StatusBadRequest {
			raw, _ := io.ReadAll(injectionResp.Body)
			injectionResp.Body.Close()
			t.Fatalf("bulk injection: expected 400, got %d: %s", injectionResp.StatusCode, string(raw))
		}
		injectionResp.Body.Close()
	})

	// ── 10. All Delegated Endpoints ────────────────────────────────────
	t.Run("all_delegated_endpoints", func(t *testing.T) {
		wildcardKey := createWildcardKey(t, "e2e-wildcard-delegated")

		delegated := []string{
			"/api/v2/groups",
			"/api/v2/alerts",
			"/api/v2/connectors",
			"/api/v2/updates/plans",
			"/api/v2/updates/runs",
			"/api/v2/docker/hosts",
			"/api/v2/metrics/overview",
			"/api/v2/incidents",
			"/api/v2/notifications/channels",
			"/api/v2/notifications/history",
			"/api/v2/synthetic-checks",
			"/api/v2/discovery/proposals",
			"/api/v2/dependencies",
			"/api/v2/edges",
			"/api/v2/composites",
			"/api/v2/failover-pairs",
			"/api/v2/dead-letters",
			"/api/v2/audit/events",
			"/api/v2/logs/views",
			"/api/v2/hub/tls",
			"/api/v2/web-services",
			"/api/v2/agents/pending",
		}

		for _, ep := range delegated {
			ep := ep
			t.Run(strings.ReplaceAll(strings.TrimPrefix(ep, "/api/v2/"), "/", "_"), func(t *testing.T) {
				resp := doReq(t, "GET", ep, wildcardKey, "")
				notAuthError(t, resp)
			})
		}
	})

	// ── 11. Home Assistant ─────────────────────────────────────────────
	t.Run("homeassistant", func(t *testing.T) {
		// GET /api/v2/homeassistant/entities → 501 (stub)
		resp := doReq(t, "GET", "/api/v2/homeassistant/entities", ownerToken, "")
		if resp.StatusCode != http.StatusNotImplemented {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("homeassistant entities: expected 501, got %d: %s", resp.StatusCode, string(raw))
		}
		resp.Body.Close()
	})

	// ── 12. Asset Sub-paths ────────────────────────────────────────────
	t.Run("asset_sub_paths", func(t *testing.T) {
		subPaths := []string{
			"/api/v2/assets/srv1/network",
			"/api/v2/assets/srv1/disks",
			"/api/v2/assets/srv1/packages",
			"/api/v2/assets/srv1/cron",
			"/api/v2/assets/srv1/users",
			"/api/v2/assets/srv1/logs",
			"/api/v2/assets/srv1/metrics",
			"/api/v2/assets/srv1/services",
			"/api/v2/assets/srv1/processes",
		}
		for _, sp := range subPaths {
			sp := sp
			// Derive a clean sub-test name from the last path segment.
			parts := strings.Split(sp, "/")
			name := parts[len(parts)-1]
			t.Run(name, func(t *testing.T) {
				resp := doReq(t, "GET", sp, ownerToken, "")
				notAuthError(t, resp)
			})
		}
	})

	// ── 13. Power Management ───────────────────────────────────────────
	t.Run("power_management", func(t *testing.T) {
		// POST /api/v2/assets/srv1/reboot → 409 (no agent)
		rebootResp := doReq(t, "POST", "/api/v2/assets/srv1/reboot", ownerToken, "")
		if rebootResp.StatusCode != http.StatusConflict {
			raw, _ := io.ReadAll(rebootResp.Body)
			rebootResp.Body.Close()
			t.Fatalf("reboot: expected 409 (no agent), got %d: %s", rebootResp.StatusCode, string(raw))
		}
		rebootResp.Body.Close()

		// POST /api/v2/assets/srv1/shutdown → 409 (no agent)
		shutResp := doReq(t, "POST", "/api/v2/assets/srv1/shutdown", ownerToken, "")
		if shutResp.StatusCode != http.StatusConflict {
			raw, _ := io.ReadAll(shutResp.Body)
			shutResp.Body.Close()
			t.Fatalf("shutdown: expected 409 (no agent), got %d: %s", shutResp.StatusCode, string(raw))
		}
		shutResp.Body.Close()
	})
}
