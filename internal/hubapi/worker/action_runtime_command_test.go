package worker

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestExecuteCommandActionFailsClosedForDisconnectedTarget(t *testing.T) {
	deps := &Deps{AgentMgr: agentmgr.NewManager()}
	result := deps.ExecuteCommandAction(actions.Job{
		JobID:   "job-1",
		RunID:   "run-1",
		Type:    actions.RunTypeCommand,
		Target:  "offline-asset",
		Command: "uptime",
	})

	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "not connected") {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Steps) != 1 || result.Steps[0].Status != actions.StatusFailed {
		t.Fatalf("steps = %#v", result.Steps)
	}
}
