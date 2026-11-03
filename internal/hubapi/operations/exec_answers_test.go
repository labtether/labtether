package operations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// A missing or empty group used to get "targets or group required", which
// says nothing about the group the user named. Neither may run a command.
func TestHandleExecMultiSaysWhatIsWrongWithTheGroup(t *testing.T) {
	deps := newExecMultiTestDeps(t, nil, func(terminal.CommandJob) terminal.CommandResult {
		t.Error("a command ran for a group without devices")
		return terminal.CommandResult{}
	})
	groupStore := persistence.NewMemoryGroupStore()
	empty, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Empty", Slug: "empty"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	deps.GroupStore = groupStore
	cases := []struct {
		group    string
		wantCode int
		wantText string
	}{
		{"grp-missing", http.StatusNotFound, "group not found"},
		{empty.ID, http.StatusBadRequest, "group has no devices"},
	}
	for _, tc := range cases {
		rec := handleExecMultiTestRequest(t, deps, ExecMultiRequest{Group: tc.group, Command: "uptime"})
		if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantText) {
			t.Fatalf("group %q: got %d %s, want %d %q", tc.group, rec.Code, rec.Body.String(), tc.wantCode, tc.wantText)
		}
	}
}

// exec on an ID that names no device said "<id> agent is not connected"
// (409), as if a device existed. It must be 404 "device not found"; a known
// device whose agent is away stays 409.
func TestHandleAssetExecTellsUnknownFromOffline(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	deps.AgentMgr = agentmgr.NewManager()
	deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	deps.ScopesFromContext = func(context.Context) []string { return nil }
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "srv-away", Status: "online"}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	cases := []struct {
		device, wantCode, wantText string
		wantStatus                 int
	}{
		{"srv-missing", "asset_not_found", "device not found", http.StatusNotFound},
		{"srv-away", "asset_offline", "agent is not connected", http.StatusConflict},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/"+tc.device+"/exec", strings.NewReader(`{"command":"uptime"}`))
		rec := httptest.NewRecorder()
		deps.HandleAssetExec(rec, req, tc.device)
		body := rec.Body.String()
		if rec.Code != tc.wantStatus || !strings.Contains(body, tc.wantCode) || !strings.Contains(body, tc.wantText) {
			t.Fatalf("%s: got %d %s, want %d %s %q", tc.device, rec.Code, body, tc.wantStatus, tc.wantCode, tc.wantText)
		}
	}
}
