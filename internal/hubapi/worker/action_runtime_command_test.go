package worker

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
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

func TestExecuteCommandActionRechecksCurrentPolicyBeforeDispatch(t *testing.T) {
	agentMgr := agentmgr.NewManager()
	agentMgr.Register(agentmgr.NewAgentConn(nil, "asset-1", "linux"))
	dispatched := false
	deps := &Deps{
		AgentMgr: agentMgr,
		ExecuteViaAgent: func(terminal.CommandJob) terminal.CommandResult {
			dispatched = true
			return terminal.CommandResult{Status: "succeeded"}
		},
		GetPolicyConfig: func() policy.EvaluatorConfig {
			cfg := policy.DefaultEvaluatorConfig()
			cfg.StructuredEnabled = false
			return cfg
		},
	}

	result := deps.ExecuteCommandAction(actions.Job{
		JobID: "job-policy", RunID: "run-policy", ActorID: "operator",
		Type: actions.RunTypeCommand, Target: "asset-1", Command: "uptime",
	})

	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "denied by current policy") {
		t.Fatalf("result = %#v", result)
	}
	if dispatched {
		t.Fatal("policy-denied queued action reached execution backend")
	}
}

func TestExecuteCommandActionFailsClosedWithoutPolicy(t *testing.T) {
	agentMgr := agentmgr.NewManager()
	agentMgr.Register(agentmgr.NewAgentConn(nil, "asset-1", "linux"))
	dispatched := false
	deps := &Deps{
		AgentMgr: agentMgr,
		ExecuteViaAgent: func(terminal.CommandJob) terminal.CommandResult {
			dispatched = true
			return terminal.CommandResult{Status: "succeeded"}
		},
	}

	result := deps.ExecuteCommandAction(actions.Job{
		JobID: "job-no-policy", RunID: "run-no-policy", ActorID: "operator",
		Type: actions.RunTypeCommand, Target: "asset-1", Command: "uptime",
	})

	if result.Status != actions.StatusFailed || !strings.Contains(result.Error, "policy is unavailable") {
		t.Fatalf("result = %#v", result)
	}
	if dispatched {
		t.Fatal("queued action reached execution backend without a policy")
	}
}
