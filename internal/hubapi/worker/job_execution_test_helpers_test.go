package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// Existing policy/redaction tests use memory stores. This explicit test-only
// adapter preserves that scope; durable claim/race proof uses real Postgres.
type handlerTestExecutionStore struct{ d *Deps }

func bindHandlerTestExecution(d *Deps) {
	d.JobExecutionStore = handlerTestExecutionStore{d}
	d.ExecuteViaAgentContext = func(_ context.Context, j terminal.CommandJob) terminal.CommandResult { return d.ExecuteViaAgent(j) }
	d.ExecuteTerminalCommandContext = func(_ context.Context, j terminal.CommandJob) terminal.CommandResult {
		return d.ExecuteTerminalCommand(j)
	}
	d.ExecuteActionInProcessContext = func(_ context.Context, j actions.Job) actions.Result { return d.ExecuteActionInProcess(j) }
	d.ExecuteUpdateScopeContext = func(_ context.Context, j updates.Job, t, s string) updates.RunResultEntry {
		return d.ExecuteUpdateScope(j, t, s)
	}
}
func (s handlerTestExecutionStore) BeginJobExecution(ctx context.Context, j *jobqueue.Job) (bool, error) {
	if j.Kind != jobqueue.KindScheduleRun {
		return true, nil
	}
	var p schedules.ExecutionJob
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return false, err
	}
	current, found, err := s.d.ScheduleStore.GetScheduledTask(ctx, p.ScheduleID)
	if err != nil {
		return false, err
	}
	if !found || current.LastRunJobID != p.JobID {
		j.Status = jobqueue.StatusCompleted
		return false, nil
	}
	switch current.LastRunStatus {
	case "queued":
		return s.d.ScheduleExecutionStore.BeginScheduledTaskExecution(ctx, p.ScheduleID, p.JobID)
	case "running":
		err = s.FinishJobExecution(ctx, j, persistence.JobExecutionResult{Schedule: &persistence.ScheduleExecutionResult{Status: "failed", Error: persistence.InterruptedJobOutcome}})
	default:
		j.Status = jobqueue.StatusCompleted
	}
	return false, err
}
func (s handlerTestExecutionStore) FinishJobExecution(ctx context.Context, j *jobqueue.Job, r persistence.JobExecutionResult) error {
	var err error
	switch {
	case r.Terminal != nil:
		p := r.Terminal
		err = s.d.TerminalStore.UpdateCommandResult(p.SessionID, p.CommandID, p.Status, p.Output)
	case r.Action != nil:
		err = s.d.ActionStore.ApplyActionResult(*r.Action)
	case r.Update != nil:
		err = s.d.UpdateStore.ApplyUpdateResult(*r.Update)
	case r.Schedule != nil:
		var p schedules.ExecutionJob
		if err = json.Unmarshal(j.Payload, &p); err == nil {
			err = s.d.ScheduleExecutionStore.CompleteScheduledTaskExecution(ctx, p.ScheduleID, p.JobID, r.Schedule.Status, r.Schedule.Error, time.Now().UTC())
		}
	}
	if err == nil {
		j.Status = jobqueue.StatusCompleted
	}
	return err
}
func (s handlerTestExecutionStore) StartScheduledAction(_ context.Context, j *jobqueue.Job, target string) (actions.Job, error) {
	var p schedules.ExecutionJob
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return actions.Job{}, err
	}
	run, err := s.d.ActionStore.CreateActionRun(actions.ExecuteRequest{Type: actions.RunTypeCommand, ActorID: p.ActorID, Target: target, Command: p.Command, Params: map[string]string{"schedule_id": p.ScheduleID, "schedule_job_id": p.JobID}})
	return actions.Job{JobID: idgen.New("schedact"), RunID: run.ID, Type: run.Type, ActorID: run.ActorID, Target: target, Command: run.Command, Params: run.Params, TimeoutSec: 30}, err
}
func (s handlerTestExecutionStore) FinishScheduledAction(_ context.Context, _ *jobqueue.Job, _ actions.Job, r actions.Result) error {
	return s.d.ActionStore.ApplyActionResult(r)
}
