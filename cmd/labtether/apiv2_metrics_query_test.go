package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/assets"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

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
