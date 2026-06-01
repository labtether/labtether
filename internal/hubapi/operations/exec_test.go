package operations

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

func newConnectedExecDeps(t *testing.T, captured *terminal.CommandJob) *ExecDeps {
	t.Helper()

	agentMgr := agentmgr.NewManager()
	agentMgr.Register(agentmgr.NewAgentConn(nil, "srv1", "linux"))

	return &ExecDeps{
		AgentMgr:   agentMgr,
		AssetStore: persistence.NewMemoryAssetStore(),
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			*captured = job
			return terminal.CommandResult{Status: "succeeded", Output: "ok"}
		},
		PrincipalActorID: func(context.Context) string { return "tester" },
	}
}

func TestExecOnAssetPassesTimeoutToAgentCommand(t *testing.T) {
	var captured terminal.CommandJob
	deps := newConnectedExecDeps(t, &captured)
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", nil)

	result := deps.ExecOnAsset(req, "srv1", "uptime", 42)
	if result.Error != "" {
		t.Fatalf("ExecOnAsset returned error: %s", result.Error)
	}
	if captured.TimeoutSec != 42 {
		t.Fatalf("TimeoutSec = %d, want 42", captured.TimeoutSec)
	}
}

func TestExecOnAssetNormalizesTimeoutBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  int
		want int
	}{
		{name: "default", raw: 0, want: DefaultExecTimeout},
		{name: "negative", raw: -10, want: DefaultExecTimeout},
		{name: "max", raw: MaxExecTimeout + 1, want: MaxExecTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured terminal.CommandJob
			deps := newConnectedExecDeps(t, &captured)
			req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", nil)

			result := deps.ExecOnAsset(req, "srv1", "uptime", tc.raw)
			if result.Error != "" {
				t.Fatalf("ExecOnAsset returned error: %s", result.Error)
			}
			if captured.TimeoutSec != tc.want {
				t.Fatalf("TimeoutSec = %d, want %d", captured.TimeoutSec, tc.want)
			}
		})
	}
}
