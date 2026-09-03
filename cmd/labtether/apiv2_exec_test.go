package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
)

func TestHandleV2AssetExec_NoAgent(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "srv1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	body := `{"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	// No agent connected → 409
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 (no agent), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2AssetExec_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no exec scope
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetExec_MissingCommand(t *testing.T) {
	s := newTestAPIServer(t)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetExec(rec, req, "srv1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleV2Exec_MultiTarget(t *testing.T) {
	s := newTestAPIServer(t)
	for _, id := range []string{"srv1", "srv2"} {
		_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id, Name: id, Status: "online", Platform: "linux",
			Source: "agent", Type: "host",
		})
		if err != nil {
			t.Fatalf("failed to create test asset %s: %v", id, err)
		}
	}

	body := `{"targets":["srv1","srv2"],"command":"uptime"}`
	req := httptest.NewRequest("POST", "/api/v2/exec", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "admin", "admin")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2ExecMulti(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := resp["data"].(map[string]any)
	if data["results"] == nil {
		t.Error("should have results")
	}
	if data["summary"] == nil {
		t.Error("should have summary")
	}
}

func TestV2ExecRoutesRespectGroupMaintenance(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupMaintenanceStore = &maintenanceOnlyGroupMaintenanceStore{}
	groupID := mustCreateGroup(t, sut, "Exec Maintenance", "exec-maintenance")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "srv1", Status: "online", Platform: "linux",
		Source: "agent", Type: "host", GroupID: groupID,
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.groupMaintenanceStore.CreateGroupMaintenanceWindow(groupID, groupmaintenance.CreateMaintenanceWindowRequest{
		Name:         "Block exec",
		StartAt:      time.Now().UTC().Add(-time.Minute),
		EndAt:        time.Now().UTC().Add(time.Minute),
		BlockActions: true,
	}); err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		body    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{
			name: "single asset",
			path: "/api/v2/assets/srv1/exec",
			body: `{"command":"uptime"}`,
			handler: func(w http.ResponseWriter, r *http.Request) {
				sut.handleV2AssetExec(w, r, "srv1")
			},
		},
		{
			name:    "multi asset",
			path:    "/api/v2/exec",
			body:    `{"targets":["srv1"],"command":"uptime"}`,
			handler: sut.handleV2ExecMulti,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
			rec := httptest.NewRecorder()

			tc.handler(rec, req)

			if rec.Code != http.StatusLocked || !strings.Contains(rec.Body.String(), "maintenance_blocked") {
				t.Fatalf("status = %d, want 423 maintenance_blocked; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestActionAndV2ExecRoutesShareRateLimit(t *testing.T) {
	sut := newTestAPIServer(t)
	const sharedExecLimit = 120
	for index := 0; index < sharedExecLimit; index++ {
		var path string
		var handler http.HandlerFunc
		if index%2 == 0 {
			path = "/actions/execute"
			handler = sut.handleActionExecute
		} else {
			path = "/api/v2/exec"
			handler = sut.handleV2ExecMulti
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d was limited before shared budget was exhausted", index+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/exec", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	sut.handleV2ExecMulti(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after shared command budget; body=%s", rec.Code, rec.Body.String())
	}
}
