package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	actionspkg "github.com/labtether/labtether/internal/hubapi/actionspkg"
	"github.com/labtether/labtether/internal/policy"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
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
