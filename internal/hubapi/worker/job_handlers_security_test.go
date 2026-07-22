package worker

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

func TestTerminalCommandCredentialsResolvedOnlyAfterQueueClaim(t *testing.T) {
	store := persistence.NewMemoryTerminalStore()
	session, err := store.CreateSession(terminal.CreateSessionRequest{
		ActorID: "owner",
		Target:  "asset-1",
		Mode:    "structured",
	})
	if err != nil {
		t.Fatal(err)
	}
	command, err := store.AddCommand(session.ID, terminal.CreateCommandRequest{
		ActorID: "owner",
		Command: "uptime",
	}, session.Target, session.Mode)
	if err != nil {
		t.Fatal(err)
	}

	queued := terminal.CommandJob{
		JobID:     "job-1",
		SessionID: session.ID,
		CommandID: command.ID,
		ActorID:   "owner",
		Target:    session.Target,
		Command:   command.Body,
		Mode:      session.Mode,
	}
	payload, err := json.Marshal(queued)
	if err != nil {
		t.Fatal(err)
	}

	prepared := false
	executed := false
	deps := &Deps{
		TerminalStore: store,
		PrepareTerminalCommand: func(job *terminal.CommandJob) error {
			prepared = true
			job.SSHConfig = &terminal.SSHConfig{Password: "memory-only-secret"}
			return nil
		},
		ExecuteTerminalCommand: func(job terminal.CommandJob) terminal.CommandResult {
			executed = true
			if job.SSHConfig == nil || job.SSHConfig.Password != "memory-only-secret" {
				t.Fatal("executor did not receive in-memory credentials")
			}
			return terminal.CommandResult{
				JobID:       job.JobID,
				SessionID:   job.SessionID,
				CommandID:   job.CommandID,
				Status:      "succeeded",
				Output:      "ok",
				CompletedAt: time.Now().UTC(),
			}
		},
	}
	var processed atomic.Uint64
	handler := deps.HandleTerminalCommandJob(&processed)
	if err := handler(context.Background(), &jobqueue.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if !prepared || !executed {
		t.Fatalf("prepared=%t executed=%t", prepared, executed)
	}
	if processed.Load() != 1 {
		t.Fatalf("processed = %d", processed.Load())
	}
}

func TestTerminalCompletionAuditAndLogsExcludeRawOutput(t *testing.T) {
	terminalStore := persistence.NewMemoryTerminalStore()
	session, err := terminalStore.CreateSession(terminal.CreateSessionRequest{ActorID: "owner", Target: "asset-1", Mode: "structured"})
	if err != nil {
		t.Fatal(err)
	}
	command, err := terminalStore.AddCommand(session.ID, terminal.CreateCommandRequest{ActorID: "owner", Command: "safe-command"}, session.Target, session.Mode)
	if err != nil {
		t.Fatal(err)
	}
	jobPayload, err := json.Marshal(terminal.CommandJob{
		JobID: "job-redaction", SessionID: session.ID, CommandID: command.ID,
		ActorID: "owner", Target: session.Target, Command: command.Body, Mode: session.Mode,
	})
	if err != nil {
		t.Fatal(err)
	}
	auditStore := persistence.NewMemoryAuditStore()
	logStore := persistence.NewMemoryLogStore()
	secretOutput := "LTQA_TERMINAL_OUTPUT_SECRET_e2c1"
	deps := &Deps{
		TerminalStore: terminalStore,
		AuditStore:    auditStore,
		LogStore:      logStore,
		ExecuteTerminalCommand: func(job terminal.CommandJob) terminal.CommandResult {
			return terminal.CommandResult{JobID: job.JobID, SessionID: job.SessionID, CommandID: job.CommandID, Status: "succeeded", Output: secretOutput, CompletedAt: time.Now().UTC()}
		},
	}
	var processed atomic.Uint64
	if err := deps.HandleTerminalCommandJob(&processed)(context.Background(), &jobqueue.Job{Payload: jobPayload}); err != nil {
		t.Fatal(err)
	}

	events, err := auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	logEvents, err := logStore.QueryEvents(logs.QueryRequest{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Audit any `json:"audit"`
		Logs  any `json:"logs"`
	}{events, logEvents})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretOutput) {
		t.Fatalf("audit/log boundary persisted raw terminal output: %s", encoded)
	}
}

func TestActionCompletionAuditAndLogsExcludeRawOutputAndError(t *testing.T) {
	actionStore := persistence.NewMemoryActionStore()
	run, err := actionStore.CreateActionRun(actions.ExecuteRequest{ActorID: "owner", Type: actions.RunTypeCommand, Target: "asset-1", Command: "safe-command"})
	if err != nil {
		t.Fatal(err)
	}
	job := actions.Job{JobID: "action-job-redaction", RunID: run.ID, Type: actions.RunTypeCommand, ActorID: "owner", Target: "asset-1", Command: "safe-command"}
	jobPayload, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	auditStore := persistence.NewMemoryAuditStore()
	logStore := persistence.NewMemoryLogStore()
	secretOutput := "LTQA_ACTION_OUTPUT_SECRET_900a"
	secretError := "LTQA_ACTION_ERROR_SECRET_116c"
	var actionCompleted map[string]any
	deps := &Deps{
		ActionStore: actionStore,
		AuditStore:  auditStore,
		LogStore:    logStore,
		ExecuteActionInProcess: func(actions.Job) actions.Result {
			return actions.Result{JobID: job.JobID, RunID: run.ID, Status: actions.StatusFailed, Output: secretOutput, Error: secretError, CompletedAt: time.Now().UTC()}
		},
		Broadcast: func(eventType string, data map[string]any) {
			if eventType == "action.completed" {
				actionCompleted = data
			}
		},
	}
	var processed atomic.Uint64
	if err := deps.HandleActionRunJob(&processed)(context.Background(), &jobqueue.Job{Payload: jobPayload}); err != nil {
		t.Fatal(err)
	}

	events, err := auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	logEvents, err := logStore.QueryEvents(logs.QueryRequest{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Audit any `json:"audit"`
		Logs  any `json:"logs"`
	}{events, logEvents})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretOutput) || strings.Contains(string(encoded), secretError) {
		t.Fatalf("audit/log boundary persisted raw action output/error: %s", encoded)
	}
	if actionCompleted["job_id"] != job.JobID || actionCompleted["run_id"] != run.ID || actionCompleted["status"] != actions.StatusFailed {
		t.Fatalf("action.completed event = %#v", actionCompleted)
	}
}

func TestUpdateCompletionPublishesAdvertisedWebhookEvent(t *testing.T) {
	updateStore := persistence.NewMemoryUpdateStore()
	plan, err := updateStore.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Webhook update plan",
		Targets: []string{"asset-1"},
		Scopes:  []string{updates.ScopeOSPackages},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	run, err := updateStore.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "owner"})
	if err != nil {
		t.Fatalf("create update run: %v", err)
	}
	job := updates.Job{
		JobID: "update-job-webhook", RunID: run.ID, ActorID: "owner", DryRun: true,
		Plan: plan, RequestedAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	var updateCompleted map[string]any
	deps := &Deps{
		UpdateStore: updateStore,
		AuditStore:  persistence.NewMemoryAuditStore(),
		LogStore:    persistence.NewMemoryLogStore(),
		ExecuteUpdateScope: func(_ updates.Job, target, scope string) updates.RunResultEntry {
			return updates.RunResultEntry{Target: target, Scope: scope, Status: updates.StatusSucceeded, Summary: "validated"}
		},
		Broadcast: func(eventType string, data map[string]any) {
			if eventType == "update.completed" {
				updateCompleted = data
			}
		},
	}
	var processed atomic.Uint64
	if err := deps.HandleUpdateRunJob(&processed)(context.Background(), &jobqueue.Job{Payload: payload}); err != nil {
		t.Fatalf("handle update job: %v", err)
	}
	if updateCompleted["job_id"] != job.JobID || updateCompleted["run_id"] != run.ID || updateCompleted["status"] != updates.StatusSucceeded {
		t.Fatalf("update.completed event = %#v", updateCompleted)
	}
}
