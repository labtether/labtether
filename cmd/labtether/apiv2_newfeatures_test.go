package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	actionspkg "github.com/labtether/labtether/internal/hubapi/actionspkg"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
)

// --- Saved Actions ---

func seedSavedActionAsset(t *testing.T, s *apiServer, assetID string) {
	t.Helper()
	if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID,
		Name:    assetID,
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed saved-action asset %q: %v", assetID, err)
	}
}

func TestHandleV2SavedActions_Create(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	body := `{"name":"restart-web","steps":[{"name":"restart nginx","command":"systemctl restart nginx","target":"host-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2SavedActions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got: %v", resp)
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected non-empty id in response")
	}
	if data["name"] != "restart-web" {
		t.Errorf("expected name 'restart-web', got %v", data["name"])
	}
	steps, ok := data["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Errorf("expected 1 step, got %v", data["steps"])
	}
}

func TestHandleV2SavedActions_Create_MissingName(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"steps":[{"name":"restart nginx","command":"systemctl restart nginx","target":"host-01"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2SavedActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2SavedActions_Create_MissingSteps(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"name":"restart-web","steps":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2SavedActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2SavedActions_List(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	// Create one first.
	createBody := `{"name":"my-action","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "admin", "admin"))
	createRec := httptest.NewRecorder()
	s.handleV2SavedActions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// List.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/actions", nil)
	listReq = listReq.WithContext(contextWithPrincipal(listReq.Context(), "admin", "admin"))
	listRec := httptest.NewRecorder()
	s.handleV2SavedActions(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in list response")
	}
	if len(data) != 1 {
		t.Errorf("expected 1 item, got %d", len(data))
	}
}

func TestHandleV2SavedActions_ListScopesToActor(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	for _, actorID := range []string{"owner-a", "owner-b"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(
			`{"name":"`+actorID+`-action","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`,
		))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(contextWithPrincipal(req.Context(), actorID, "admin"))
		rec := httptest.NewRecorder()
		s.handleV2SavedActions(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create for %s: expected 201, got %d: %s", actorID, rec.Code, rec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/actions", nil)
	listReq = listReq.WithContext(contextWithPrincipal(listReq.Context(), "owner-a", "admin"))
	listRec := httptest.NewRecorder()
	s.handleV2SavedActions(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got: %v", resp["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 item for owner-a, got %d", len(data))
	}
	item := data[0].(map[string]any)
	if item["created_by"] != "owner-a" {
		t.Fatalf("created_by = %v, want owner-a", item["created_by"])
	}
}

func TestHandleV2SavedActions_ListSupportsLimitAndOffset(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(
			`{"name":"action-`+strconv.Itoa(i)+`","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`,
		))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
		rec := httptest.NewRecorder()
		s.handleV2SavedActions(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: expected 201, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/actions?limit=1&offset=1", nil)
	listReq = listReq.WithContext(contextWithPrincipal(listReq.Context(), "admin", "admin"))
	listRec := httptest.NewRecorder()
	s.handleV2SavedActions(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected one paged item, got %v", resp["data"])
	}
	meta, ok := resp["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %v", resp["meta"])
	}
	if int(meta["total"].(float64)) != 3 {
		t.Fatalf("meta.total = %v, want 3", meta["total"])
	}
	if int(meta["per_page"].(float64)) != 1 {
		t.Fatalf("meta.per_page = %v, want 1", meta["per_page"])
	}
	if int(meta["page"].(float64)) != 2 {
		t.Fatalf("meta.page = %v, want 2", meta["page"])
	}
}

func TestHandleV2SavedActions_Create_ValidatesSteps(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"name":"bad-action","steps":[{"name":"step1","command":"uptime","target":"   "}]}` // blank target after trim
	req := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2SavedActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2SavedActionActions_Delete(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	// Create one first.
	createBody := `{"name":"to-delete","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "admin", "admin"))
	createRec := httptest.NewRecorder()
	s.handleV2SavedActions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("setup: unmarshal: %v", err)
	}
	createData, ok := createResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("setup: expected data object in response, got: %v", createResp)
	}
	id, _ := createData["id"].(string)
	if id == "" {
		t.Fatal("setup: expected non-empty id")
	}

	// Delete it.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v2/actions/"+id, nil)
	delReq.URL.Path = "/api/v2/actions/" + id
	delReq = delReq.WithContext(contextWithPrincipal(delReq.Context(), "admin", "admin"))
	delRec := httptest.NewRecorder()
	s.handleV2SavedActionActions(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", delRec.Code, delRec.Body.String())
	}

	var delResp map[string]any
	if err := json.Unmarshal(delRec.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("delete unmarshal: %v", err)
	}
	delData, ok := delResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in delete response, got: %v", delResp)
	}
	if delData["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %v", delData["status"])
	}
}

func TestHandleV2SavedActionActions_DeleteOtherActorReturnsNotFound(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(
		`{"name":"owner-action","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`,
	))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "owner-a", "admin"))
	createRec := httptest.NewRecorder()
	s.handleV2SavedActions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("setup: unmarshal: %v", err)
	}
	id := createResp["data"].(map[string]any)["id"].(string)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v2/actions/"+id, nil)
	delReq.URL.Path = "/api/v2/actions/" + id
	delReq = delReq.WithContext(contextWithPrincipal(delReq.Context(), "owner-b", "admin"))
	delRec := httptest.NewRecorder()
	s.handleV2SavedActionActions(delRec, delReq)
	if delRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", delRec.Code, delRec.Body.String())
	}
}

func TestHandleV2SavedActionActions_GetOtherActorReturnsNotFound(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(
		`{"name":"owner-action","steps":[{"name":"step1","command":"uptime","target":"host-01"}]}`,
	))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "owner-a", "admin"))
	createRec := httptest.NewRecorder()
	s.handleV2SavedActions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("setup: unmarshal: %v", err)
	}
	id := createResp["data"].(map[string]any)["id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v2/actions/"+id, nil)
	getReq.URL.Path = "/api/v2/actions/" + id
	getReq = getReq.WithContext(contextWithPrincipal(getReq.Context(), "owner-b", "admin"))
	getRec := httptest.NewRecorder()
	s.handleV2SavedActionActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

func TestHandleV2SavedActionActions_RunAppliesPolicyChecksPerStep(t *testing.T) {
	s := newTestAPIServer(t)
	seedSavedActionAsset(t, s, "host-01")

	cfg := policy.DefaultEvaluatorConfig()
	cfg.AllowlistMode = true
	s.policyState = newPolicyRuntimeState(cfg)

	deps := s.ensureActionsDeps()
	deps.ExecOnAsset = func(r *http.Request, assetID, command string, timeoutSec int) actionspkg.ExecResult {
		t.Fatalf("ExecOnAsset should not be called for a policy-denied saved action step")
		return actionspkg.ExecResult{}
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions", strings.NewReader(
		`{"name":"blocked-action","steps":[{"name":"step1","command":"echo blocked","target":"host-01"}]}`,
	))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "admin", "admin"))
	createRec := httptest.NewRecorder()
	s.handleV2SavedActions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("setup: unmarshal: %v", err)
	}
	id := createResp["data"].(map[string]any)["id"].(string)

	runReq := httptest.NewRequest(http.MethodPost, "/api/v2/actions/"+id+"/run", nil)
	runReq.URL.Path = "/api/v2/actions/" + id + "/run"
	runReq = runReq.WithContext(contextWithPrincipal(runReq.Context(), "admin", "admin"))
	runRec := httptest.NewRecorder()
	s.handleV2SavedActionActions(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", runRec.Code, runRec.Body.String())
	}

	var runResp map[string]any
	if err := json.Unmarshal(runRec.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("run unmarshal: %v", err)
	}
	steps := runResp["data"].(map[string]any)["steps"].([]any)
	first := steps[0].(map[string]any)
	if first["error"] != "policy_denied" {
		t.Fatalf("step error = %v, want policy_denied", first["error"])
	}
}

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

// ─── GET /api/v2/assets/{id}/metrics/latest ───────────────────────────────

func TestHandleV2AssetMetricsLatest_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/assets/host-01/metrics/latest", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetMetricsLatest(rec, req, "host-01")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetMetricsLatest_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/host-01/metrics/latest", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2AssetMetricsLatest(rec, req, "host-01")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2AssetMetricsLatest_OK(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/assets/host-01/metrics/latest", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2AssetMetricsLatest(rec, req, "host-01")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["data"] == nil {
		t.Error("expected data field in response")
	}
}

// Routes metrics/latest through the asset actions handler.
func TestHandleV2AssetActions_MetricsLatestSubPath(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "host-ml-01",
		Name:    "metrics-latest-host",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/assets/host-ml-01/metrics/latest", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2AssetActions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── GET /api/v2/metrics/query ────────────────────────────────────────────

func TestHandleV2MetricsQuery_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/metrics/query?asset_ids=x&metric=cpu_percent", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2MetricsQuery_MissingAssetIDs(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/metrics/query?metric=cpu_percent", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2MetricsQuery_MissingMetric(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/metrics/query?asset_ids=host-01", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2MetricsQuery_InvalidTimeRange(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v2/metrics/query?asset_ids=host-01&metric=cpu_percent&from=2024-01-02T00:00:00Z&to=2024-01-01T00:00:00Z", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for from >= to, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2MetricsQuery_OK(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/metrics/query?asset_ids=host-01&metric=cpu_percent", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["data"] == nil {
		t.Error("expected data in response")
	}
}

func TestHandleV2MetricsQuery_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/metrics/query", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2MetricsQueryRejectsExcessiveAssetsAndWindow(t *testing.T) {
	s := newTestAPIServer(t)
	ids := make([]string, 0, maxMetricsQueryAssets+1)
	for i := 0; i < maxMetricsQueryAssets+1; i++ {
		ids = append(ids, "asset-"+strconv.Itoa(i))
	}
	for _, rawURL := range []string{
		"/api/v2/metrics/query?asset_ids=" + strings.Join(ids, ",") + "&metric=cpu_percent",
		"/api/v2/metrics/query?asset_ids=asset-1&metric=cpu_percent&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:01Z",
		"/api/v2/metrics/query?asset_ids=asset-1&metric=not_a_supported_metric",
	} {
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
		req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
		rec := httptest.NewRecorder()
		s.handleV2MetricsQuery(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d, want 400; body=%s", rawURL, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleV2MetricsQueryClampsStepToBoundResponsePoints(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/metrics/query?asset_ids=asset-1&metric=cpu_percent&from=2026-07-13T00:00:00Z&to=2026-07-14T00:00:00Z&step=1s", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2MetricsQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Step string `json:"step"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	step, err := time.ParseDuration(response.Data.Step)
	if err != nil {
		t.Fatalf("parse step %q: %v", response.Data.Step, err)
	}
	minimum := metricsQueryMinimumStep(24 * time.Hour)
	if step < minimum {
		t.Fatalf("step=%s, want at least %s", step, minimum)
	}
}

// ─── POST/GET /api/v2/file-transfers ──────────────────────────────────────

type v2FileTransferListStore struct {
	transfers []persistence.FileTransfer
	actorID   string
	status    string
	limit     int
	offset    int
}

func (s *v2FileTransferListStore) GetFileTransfer(_ context.Context, id string) (*persistence.FileTransfer, error) {
	for i := range s.transfers {
		if s.transfers[i].ID == id {
			transfer := s.transfers[i]
			return &transfer, nil
		}
	}
	return nil, persistence.ErrNotFound
}

func (s *v2FileTransferListStore) CreateFileTransfer(_ context.Context, transfer *persistence.FileTransfer) error {
	s.transfers = append(s.transfers, *transfer)
	return nil
}

func (s *v2FileTransferListStore) UpdateFileTransfer(_ context.Context, transfer *persistence.FileTransfer) error {
	for i := range s.transfers {
		if s.transfers[i].ID == transfer.ID {
			s.transfers[i] = *transfer
			return nil
		}
	}
	return persistence.ErrNotFound
}

func (s *v2FileTransferListStore) ListFileTransfers(_ context.Context, actorID, status string, limit, offset int) ([]persistence.FileTransfer, int, error) {
	s.actorID = actorID
	s.status = status
	s.limit = limit
	s.offset = offset
	filtered := make([]persistence.FileTransfer, 0, len(s.transfers))
	for _, transfer := range s.transfers {
		if transfer.ActorID == actorID && (status == "" || transfer.Status == status) {
			filtered = append(filtered, transfer)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	total := len(filtered)
	if offset >= total {
		return []persistence.FileTransfer{}, total, nil
	}
	return append([]persistence.FileTransfer(nil), filtered[offset:min(offset+limit, total)]...), total, nil
}

func (s *v2FileTransferListStore) ListActiveFileTransfers(context.Context) ([]persistence.FileTransfer, error) {
	return []persistence.FileTransfer{}, nil
}

func TestHandleV2FileTransfers_GetScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransfers_GetDelegatesActorScopedFilteredPage(t *testing.T) {
	s := newTestAPIServer(t)
	store := &v2FileTransferListStore{transfers: []persistence.FileTransfer{
		{ID: "ftx_500", ActorID: "actor-b", Status: "completed"},
		{ID: "ftx_400", ActorID: "actor-a", Status: "pending"},
		{ID: "ftx_300", ActorID: "actor-a", Status: "completed"},
		{ID: "ftx_100", ActorID: "actor-a", Status: "completed"},
	}}
	s.fileTransferStore = store
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers?status=completed&limit=1&offset=1", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "actor-a", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Transfers []persistence.FileTransfer `json:"transfers"`
			Total     int                        `json:"total"`
			Limit     int                        `json:"limit"`
			Offset    int                        `json:"offset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "" {
		t.Fatal("v2 response is missing request_id")
	}
	if response.Data.Total != 2 || response.Data.Limit != 1 || response.Data.Offset != 1 {
		t.Fatalf("pagination=%+v", response.Data)
	}
	if len(response.Data.Transfers) != 1 || response.Data.Transfers[0].ID != "ftx_100" {
		t.Fatalf("transfers=%+v, want second matching actor-a record", response.Data.Transfers)
	}
	if strings.Contains(rec.Body.String(), "ftx_500") || strings.Contains(rec.Body.String(), "actor-a") || strings.Contains(rec.Body.String(), "actor-b") {
		t.Fatalf("v2 response disclosed actor data: %s", rec.Body.String())
	}
	if store.actorID != "actor-a" || store.status != "completed" || store.limit != 1 || store.offset != 1 {
		t.Fatalf("delegated query actor=%q status=%q limit=%d offset=%d", store.actorID, store.status, store.limit, store.offset)
	}
}

func TestHandleV2FileTransfers_GetRejectsOutOfBoundsPagination(t *testing.T) {
	s := newTestAPIServer(t)
	store := &v2FileTransferListStore{}
	s.fileTransferStore = store
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers?limit=101", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "actor-a", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2FileTransfers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.actorID != "" {
		t.Fatalf("invalid pagination reached persistence for actor %q", store.actorID)
	}
}

func TestV2OpenAPIFileTransferListContractIsImplementedAndBounded(t *testing.T) {
	var document struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal([]byte(v2OpenAPISpec), &document); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	operation, ok := document.Paths["/api/v2/file-transfers"]["get"].(map[string]any)
	if !ok {
		t.Fatal("GET /api/v2/file-transfers missing from OpenAPI")
	}
	responses, ok := operation["responses"].(map[string]any)
	if !ok || responses["200"] == nil || responses["501"] != nil {
		t.Fatalf("responses=%v, want implemented 200 contract without 501", operation["responses"])
	}
	parameters, ok := operation["parameters"].([]any)
	if !ok || len(parameters) != 3 {
		t.Fatalf("parameters=%v, want status, limit, and offset", operation["parameters"])
	}
	seen := make(map[string]map[string]any, len(parameters))
	for _, rawParameter := range parameters {
		parameter, _ := rawParameter.(map[string]any)
		name, _ := parameter["name"].(string)
		schema, _ := parameter["schema"].(map[string]any)
		seen[name] = schema
	}
	if seen["limit"]["maximum"] != float64(persistence.FileTransferListMaxLimit) || seen["offset"]["maximum"] != float64(persistence.FileTransferListMaxOffset) {
		t.Fatalf("pagination schemas=%v, want limits %d/%d", seen, persistence.FileTransferListMaxLimit, persistence.FileTransferListMaxOffset)
	}
	statusEnum, _ := seen["status"]["enum"].([]any)
	if len(statusEnum) != 4 {
		t.Fatalf("status schema=%v, want four persisted states", seen["status"])
	}
	if description, _ := operation["description"].(string); !strings.Contains(description, "authenticated actor") || !strings.Contains(description, "newest-first") {
		t.Fatalf("description=%q is missing isolation or ordering semantics", description)
	}
}

func TestHandleV2FileTransfers_PostScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"source_type":"connection","source_id":"x","source_path":"/x","dest_type":"connection","dest_id":"y","dest_path":"/y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/file-transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"files:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransfers_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v2/file-transfers", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── GET /api/v2/file-transfers/{id} ──────────────────────────────────────

func TestHandleV2FileTransferActions_MissingID(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers/", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing ID, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2FileTransferActions_GetScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers/ft_abc", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransferActions_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v2/file-transfers/ft_abc", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── POST /api/v2/bulk/file-push ──────────────────────────────────────────

func TestHandleV2BulkFilePush_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"source_connection_id":"c1","source_path":"/x","targets":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"files:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkFilePush(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2BulkFilePush_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/bulk/file-push", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2BulkFilePush(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2BulkFilePush_RejectsTooManyTargets(t *testing.T) {
	s := newTestAPIServer(t)
	targets := make([]map[string]string, 0, maxBulkFilePushTargets+1)
	for i := 0; i < maxBulkFilePushTargets+1; i++ {
		targets = append(targets, map[string]string{
			"dest_connection_id": "conn",
			"dest_path":          "/tmp/out",
		})
	}
	body, err := json.Marshal(map[string]any{
		"source_connection_id": "c1",
		"source_path":          "/x",
		"targets":              targets,
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"files:write"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.handleV2BulkFilePush(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkFilePush_ValidationErrors(t *testing.T) {
	s := newTestAPIServer(t)

	cases := []struct {
		name string
		body string
		code int
	}{
		{
			name: "missing source_connection_id",
			body: `{"source_path":"/x","targets":[{"dest_connection_id":"c2","dest_path":"/y"}]}`,
			code: http.StatusBadRequest,
		},
		{
			name: "missing source_path",
			body: `{"source_connection_id":"c1","targets":[{"dest_connection_id":"c2","dest_path":"/y"}]}`,
			code: http.StatusBadRequest,
		},
		{
			name: "empty targets",
			body: `{"source_connection_id":"c1","source_path":"/x","targets":[]}`,
			code: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
			rec := httptest.NewRecorder()
			s.handleV2BulkFilePush(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, rec.Code, rec.Body.String())
			}
		})
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

// ─── GET /api/v2/openapi.json ─────────────────────────────────────────────

func TestHandleV2OpenAPI_OK(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/openapi.json", nil)
	// No auth required.
	rec := httptest.NewRecorder()
	s.handleV2OpenAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	// Verify the response is valid JSON with openapi field.
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %v", doc["openapi"])
	}
	if doc["paths"] == nil {
		t.Error("expected paths object in spec")
	}
}

func TestHandleV2OpenAPIAdvertisesImplementedAssetSubpaths(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(v2OpenAPISpec), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}

	expected := map[string][]string{
		"/api/v2/assets/{id}/exec":                    {"post"},
		"/api/v2/assets/{id}/files":                   {"get", "delete"},
		"/api/v2/assets/{id}/files/read":              {"get"},
		"/api/v2/assets/{id}/files/write":             {"post"},
		"/api/v2/assets/{id}/files/mkdir":             {"post"},
		"/api/v2/assets/{id}/files/rename":            {"post"},
		"/api/v2/assets/{id}/files/copy":              {"post"},
		"/api/v2/assets/{id}/processes":               {"get"},
		"/api/v2/assets/{id}/processes/kill":          {"post"},
		"/api/v2/assets/{id}/services":                {"get"},
		"/api/v2/assets/{id}/services/{name}/start":   {"post"},
		"/api/v2/assets/{id}/services/{name}/stop":    {"post"},
		"/api/v2/assets/{id}/services/{name}/restart": {"post"},
		"/api/v2/assets/{id}/network":                 {"get"},
		"/api/v2/assets/{id}/disks":                   {"get"},
		"/api/v2/assets/{id}/packages":                {"get"},
		"/api/v2/assets/{id}/packages/upgradable":     {"get"},
		"/api/v2/assets/{id}/packages/install":        {"post"},
		"/api/v2/assets/{id}/packages/update":         {"post"},
		"/api/v2/assets/{id}/packages/upgrade":        {"post"},
		"/api/v2/assets/{id}/cron":                    {"get"},
		"/api/v2/assets/{id}/users":                   {"get"},
		"/api/v2/assets/{id}/logs":                    {"get"},
	}
	for path, methods := range expected {
		pathItem, ok := doc.Paths[path]
		if !ok {
			t.Errorf("implemented asset path %q is absent from OpenAPI", path)
			continue
		}
		for _, method := range methods {
			if _, ok := pathItem[method]; !ok {
				t.Errorf("implemented asset operation %s %s is absent from OpenAPI", method, path)
			}
		}
	}
}

func TestHandleV2OpenAPIMatchesImplementedAdvancedMethods(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(v2OpenAPISpec), &doc); err != nil {
		t.Fatalf("openapi spec is not valid JSON: %v", err)
	}

	expected := map[string][]string{
		"/api/v2/assets/{id}":                       {"delete", "get", "patch", "put"},
		"/api/v2/groups/{id}":                       {"delete", "get", "patch", "put"},
		"/api/v2/docker/hosts":                      {"get"},
		"/api/v2/docker/hosts/{id}":                 {"get"},
		"/api/v2/docker/containers/{id}/action":     {"post"},
		"/api/v2/docker/stacks/{id}/action":         {"post"},
		"/api/v2/updates/plans/{id}":                {"delete", "get"},
		"/api/v2/updates/plans/{id}/execute":        {"post"},
		"/api/v2/updates/runs/{id}":                 {"delete", "get"},
		"/api/v2/alerts/{id}":                       {"delete", "get"},
		"/api/v2/alerts/{id}/ack":                   {"post"},
		"/api/v2/alerts/{id}/resolve":               {"post"},
		"/api/v2/alerts/rules/{id}":                 {"delete", "get", "patch", "put"},
		"/api/v2/incidents/{id}":                    {"delete", "get", "patch", "put"},
		"/api/v2/connectors/{id}/test":              {"post"},
		"/api/v2/connectors/{id}/discover":          {"get"},
		"/api/v2/connectors/{id}/health":            {"get"},
		"/api/v2/connectors/{id}/actions":           {"get"},
		"/api/v2/credentials/profiles/{id}":         {"delete", "get"},
		"/api/v2/credentials/profiles/{id}/rotate":  {"post"},
		"/api/v2/terminal/snippets/{id}":            {"delete", "get", "put"},
		"/api/v2/agents/{id}/settings":              {"get", "patch"},
		"/api/v2/hub/tailscale":                     {"get", "post"},
		"/api/v2/web-services":                      {"get", "post"},
		"/api/v2/web-services/{id}":                 {"delete", "patch", "put"},
		"/api/v2/collectors/{id}":                   {"delete", "get", "patch", "put"},
		"/api/v2/collectors/{id}/run":               {"post"},
		"/api/v2/notifications/channels":            {"get", "post"},
		"/api/v2/synthetic-checks/{id}":             {"delete", "get", "patch", "put"},
		"/api/v2/discovery/proposals/{id}/accept":   {"post"},
		"/api/v2/discovery/proposals/{id}/dismiss":  {"post"},
		"/api/v2/dependencies/{id}":                 {"delete", "get"},
		"/api/v2/dependencies/batch":                {"get"},
		"/api/v2/dependencies/graph":                {"get"},
		"/api/v2/edges":                             {"get", "post"},
		"/api/v2/edges/{id}":                        {"delete", "get", "patch"},
		"/api/v2/edges/tree":                        {"get"},
		"/api/v2/edges/ancestors":                   {"get"},
		"/api/v2/composites":                        {"post"},
		"/api/v2/composites/{id}":                   {"get", "patch"},
		"/api/v2/composites/{id}/members/{assetId}": {"delete"},
		"/api/v2/topology/zones":                    {"post"},
		"/api/v2/topology/zones/{id}":               {"delete", "put"},
		"/api/v2/topology/zones/{id}/members":       {"put"},
		"/api/v2/topology/zones/reorder":            {"put"},
		"/api/v2/topology/connections":              {"post"},
		"/api/v2/topology/connections/{id}":         {"delete", "put"},
		"/api/v2/topology/viewport":                 {"put"},
		"/api/v2/failover-pairs/{id}":               {"delete", "get", "patch", "put"},
		"/api/v2/logs/views/{id}":                   {"delete", "get", "patch", "put"},
		"/api/v2/settings/prometheus":               {"get", "patch"},
		"/api/v2/keys/{id}":                         {"delete", "get", "patch"},
	}

	for path, want := range expected {
		pathItem, ok := doc.Paths[path]
		if !ok {
			t.Errorf("implemented path %q is absent from OpenAPI", path)
			continue
		}
		got := make([]string, 0, len(pathItem))
		for key := range pathItem {
			if key != "parameters" {
				got = append(got, key)
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("OpenAPI methods for %s = %v, want %v", path, got, want)
		}
	}

	for _, path := range []string{
		"/api/v2/agents/{id}",
		"/api/v2/connectors/{id}",
		"/api/v2/discovery/proposals/{id}",
		"/api/v2/docker/stacks/{id}",
	} {
		if _, ok := doc.Paths[path]; ok {
			t.Errorf("OpenAPI still advertises unimplemented path %q", path)
		}
	}
}

func TestHandleV2WebServicesPostCreatesManualService(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/web-services", strings.NewReader(
		`{"name":"Lab UI","category":"Infrastructure","url":"https://lab.example.test"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2WebServices(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"Lab UI"`) {
		t.Fatalf("created service missing from response: %s", rec.Body.String())
	}
}

func TestHandleV2WebServicesRejectsUnadvertisedMethods(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/web-services", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2WebServices(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2HubStatusRejectsMutatingMethods(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/hub/status", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2HubStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
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

func TestHandleV2OpenAPI_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/openapi.json", nil)
	rec := httptest.NewRecorder()
	s.handleV2OpenAPI(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── metricsQueryAutoStep helper ──────────────────────────────────────────

func TestMetricsQueryAutoStep(t *testing.T) {
	cases := []struct {
		window time.Duration
		max    time.Duration
	}{
		{15 * time.Minute, time.Minute},
		{time.Hour, 5 * time.Minute},
		{4 * time.Hour, 10 * time.Minute},
		{12 * time.Hour, 15 * time.Minute},
		{48 * time.Hour, time.Hour},
	}
	for _, tc := range cases {
		step := metricsQueryAutoStep(tc.window)
		if step > tc.max {
			t.Errorf("window=%v: step %v exceeds expected max %v", tc.window, step, tc.max)
		}
		if step <= 0 {
			t.Errorf("window=%v: step must be positive, got %v", tc.window, step)
		}
	}
}
