package bulkpkg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

// A bulk restart used to run "systemctl restart <svc>" as a shell command,
// which the default command allowlist refuses on every Hub. It must go to the
// agent's service manager like the device Services page, and say in words why
// a target failed.
func TestBulkServiceRestartUsesAgentServiceManager(t *testing.T) {
	d := newTestDeps()
	d.ExecOnAsset = func(_ *http.Request, assetID, command string, _ int) ExecResult {
		t.Errorf("restart ran as a shell command on %s: %q", assetID, command)
		return ExecResult{AssetID: assetID, Error: "command not in allowlist"}
	}
	type call struct{ asset, service, action string }
	calls := make(chan call, 2)
	d.RunServiceAction = func(_ *http.Request, assetID, service, action string) ExecResult {
		calls <- call{assetID, service, action}
		if assetID == "srv2" {
			return ExecResult{AssetID: assetID, Status: "failed", Message: "Unit nginx.service not found."}
		}
		return ExecResult{AssetID: assetID, Status: "succeeded"}
	}

	body := `{"action":"restart","service":"nginx","targets":["srv1","srv2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/service-action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := apiv2.ContextWithScopes(req.Context(), []string{"bulk:*"})
	req = req.WithContext(apiv2.ContextWithAllowedAssets(ctx, nil))
	rec := httptest.NewRecorder()
	d.HandleV2BulkServiceAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	close(calls)
	seen := map[string]call{}
	for c := range calls {
		seen[c.asset] = c
	}
	for _, asset := range []string{"srv1", "srv2"} {
		if got := seen[asset]; got.service != "nginx" || got.action != "restart" {
			t.Fatalf("agent call for %s = %#v, want nginx restart", asset, got)
		}
	}
	var response struct {
		Data struct {
			Results map[string]struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.Data.Results["srv1"]; got.Status != "ok" {
		t.Fatalf("srv1 = %#v, want ok", got)
	}
	if got := response.Data.Results["srv2"]; got.Status != "failed" || got.Message != "Unit nginx.service not found." {
		t.Fatalf("srv2 = %#v, want failed with the agent's words", got)
	}
}
