package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

// --- Saved Actions ---

func TestHandleV2SavedActions_Create(t *testing.T) {
	s := newTestAPIServer(t)

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

func TestHandleV2SavedActionActions_Delete(t *testing.T) {
	s := newTestAPIServer(t)

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

// ─── POST/GET /api/v2/file-transfers ──────────────────────────────────────

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

func TestHandleV2FileTransfers_GetReturns501(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
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
