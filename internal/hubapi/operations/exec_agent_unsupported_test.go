package operations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// Fault: exec, saved actions and bulk exec dispatch a command to an explicit
// agent that never advertised commands, and the caller waits for a timeout.
func TestAssetExecRefusesExplicitAgentWithoutCommands(t *testing.T) {
	for name, metadata := range map[string]map[string]string{
		"explicit":  {agentcaps.MetaMode: agentcaps.ModeExplicit, "cap_disks": "list"},
		"no signal": nil,
	} {
		t.Run(name, func(t *testing.T) {
			dispatched := false
			var captured terminal.CommandJob
			deps := newConnectedExecDeps(t, &captured)
			conn, _ := deps.AgentMgr.Get("srv1")
			if metadata != nil {
				agentcaps.Record(conn, metadata)
			}
			deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
				dispatched = true
				return terminal.CommandResult{Status: "succeeded"}
			}
			deps.DecodeJSONBody = func(_ http.ResponseWriter, r *http.Request, dst any) error {
				return json.NewDecoder(r.Body).Decode(dst)
			}
			deps.ScopesFromContext = func(context.Context) []string { return []string{"assets:exec"} }
			deps.AppendAuditEventBestEffort = func(audit.Event, string) {}
			rec := httptest.NewRecorder()
			deps.HandleAssetExec(rec, httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/exec", strings.NewReader(`{"command":"uptime"}`)), "srv1")
			if metadata == nil {
				if rec.Code != http.StatusOK || !dispatched {
					t.Fatalf("agent without the signal: status %d dispatched %v", rec.Code, dispatched)
				}
				return
			}
			var body struct{ Error, Message string }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusUnprocessableEntity || body.Error != apiv2.AgentUnsupportedCode || body.Message != servicehttp.AgentUnsupportedMessage || dispatched {
				t.Fatalf("got %d %s dispatched=%v, want 422 agent_unsupported and no dispatch", rec.Code, rec.Body.String(), dispatched)
			}
		})
	}
}
