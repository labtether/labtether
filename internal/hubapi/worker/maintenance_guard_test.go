package worker

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

func TestTerminalWorkerRechecksMaintenanceBeforeExecution(t *testing.T) {
	store := persistence.NewMemoryTerminalStore()
	session, err := store.CreateSession(terminal.CreateSessionRequest{
		ActorID: "operator-1",
		Target:  "asset-1",
		Mode:    "structured",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	command, err := store.AddCommand(session.ID, terminal.CreateCommandRequest{
		ActorID: "operator-1",
		Command: "uptime",
	}, session.Target, session.Mode)
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	commandJob := terminal.CommandJob{
		JobID:       "job-1",
		SessionID:   session.ID,
		CommandID:   command.ID,
		ActorID:     "operator-1",
		Target:      "asset-1",
		Command:     "uptime",
		Mode:        "structured",
		RequestedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(commandJob)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}

	executions := 0
	deps := &Deps{
		TerminalStore: store,
		EvaluateAssetGuardrails: func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1", BlockActions: assetID == "asset-1"}, nil
		},
		ExecuteTerminalCommand: func(terminal.CommandJob) terminal.CommandResult {
			executions++
			return terminal.CommandResult{Status: "succeeded", CompletedAt: time.Now().UTC()}
		},
	}
	var processed atomic.Uint64
	err = deps.HandleTerminalCommandJob(&processed)(context.Background(), &jobqueue.Job{Payload: payload})
	if err != nil {
		t.Fatalf("handle job: %v", err)
	}
	if executions != 0 {
		t.Fatalf("executions = %d, want zero", executions)
	}
	commands, err := store.ListCommands(session.ID)
	if err != nil {
		t.Fatalf("list commands: %v", err)
	}
	if len(commands) != 1 || commands[0].Status != "failed" || !strings.Contains(commands[0].Output, "maintenance") {
		t.Fatalf("command result = %+v, want maintenance-blocked failure", commands)
	}
	if processed.Load() != 1 {
		t.Fatalf("processed = %d, want 1", processed.Load())
	}
}

func TestActionWorkerRechecksMaintenanceBeforeExecution(t *testing.T) {
	actionStore := persistence.NewMemoryActionStore()
	run, err := actionStore.CreateActionRun(actions.ExecuteRequest{
		ActorID: "operator-1",
		Type:    actions.RunTypeCommand,
		Target:  "asset-1",
		Command: "uptime",
	})
	if err != nil {
		t.Fatalf("create action run: %v", err)
	}
	actionJob := actions.Job{
		JobID:       "job-1",
		RunID:       run.ID,
		Type:        actions.RunTypeCommand,
		ActorID:     "operator-1",
		Target:      "asset-1",
		Command:     "uptime",
		RequestedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(actionJob)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}

	executions := 0
	deps := &Deps{
		ActionStore: actionStore,
		AuditStore:  persistence.NewMemoryAuditStore(),
		LogStore:    persistence.NewMemoryLogStore(),
		EvaluateAssetGuardrails: func(assetID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1", BlockActions: assetID == "asset-1"}, nil
		},
		ExecuteActionInProcess: func(actions.Job) actions.Result {
			executions++
			return actions.Result{Status: actions.StatusSucceeded, CompletedAt: time.Now().UTC()}
		},
	}
	var processed atomic.Uint64
	err = deps.HandleActionRunJob(&processed)(context.Background(), &jobqueue.Job{Payload: payload})
	if err != nil {
		t.Fatalf("handle job: %v", err)
	}
	if executions != 0 {
		t.Fatalf("executions = %d, want zero", executions)
	}
	updated, ok, err := actionStore.GetActionRun(run.ID)
	if err != nil || !ok {
		t.Fatalf("load action run: ok=%v err=%v", ok, err)
	}
	if updated.Status != actions.StatusFailed || !strings.Contains(updated.Error, "maintenance") {
		t.Fatalf("action result = %+v, want maintenance-blocked failure", updated)
	}
}
