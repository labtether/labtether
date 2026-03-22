package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/audit"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// HandleTerminalCommandJob returns a handler that executes terminal commands
// and writes results directly to stores.
func (d *Deps) HandleTerminalCommandJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return func(ctx interface{ Deadline() (time.Time, bool) }, job *jobqueue.Job) error {
		var cmdJob terminal.CommandJob
		if err := json.Unmarshal(job.Payload, &cmdJob); err != nil {
			return fmt.Errorf("decode terminal command job: %w", err)
		}

		processed.Add(1)

		// Route via WebSocket if agent is connected; otherwise fall through to SSH/simulated/local.
		var result terminal.CommandResult
		if d.AgentMgr != nil && d.AgentMgr.IsConnected(cmdJob.Target) && d.ExecuteViaAgent != nil {
			result = d.ExecuteViaAgent(cmdJob)
		} else {
			result = opspkg.ExecuteCommand(cmdJob)
		}

		if err := d.TerminalStore.UpdateCommandResult(result.SessionID, result.CommandID, result.Status, result.Output); err != nil {
			return fmt.Errorf("update command result: %w", err)
		}

		auditEvent := audit.NewEvent("terminal.command.completed")
		auditEvent.ID = fmt.Sprintf("audit_command_completed_%s", result.JobID)
		auditEvent.SessionID = result.SessionID
		auditEvent.CommandID = result.CommandID
		auditEvent.Decision = result.Status
		auditEvent.Details = map[string]any{
			"output": result.Output,
			"job_id": result.JobID,
		}
		if d.AuditStore != nil {
			if err := d.AuditStore.Append(auditEvent); err != nil {
				log.Printf("labtether worker: failed to append command completion audit: %v", err)
			}
		}

		if d.LogStore != nil {
			if err := d.LogStore.AppendEvent(logs.Event{
				ID:        fmt.Sprintf("log_command_completed_%s", result.JobID),
				Source:    "terminal",
				Level:     d.mapLevel(result.Status),
				Message:   fmt.Sprintf("command %s completed (%s)", result.CommandID, result.Status),
				Timestamp: result.CompletedAt,
				Fields: map[string]string{
					"session_id": result.SessionID,
					"command_id": result.CommandID,
					"status":     result.Status,
				},
			}); err != nil {
				log.Printf("labtether worker: failed to append command completion log: %v", err)
			}
		}

		// Also publish audit for the execution itself.
		execAudit := audit.NewEvent("terminal.command.executed")
		execAudit.ID = fmt.Sprintf("audit_command_executed_%s", cmdJob.JobID)
		execAudit.ActorID = cmdJob.ActorID
		execAudit.Target = cmdJob.Target
		execAudit.SessionID = cmdJob.SessionID
		execAudit.CommandID = cmdJob.CommandID
		execAudit.Decision = result.Status
		execAudit.Details = map[string]any{"job_id": result.JobID}
		if d.AuditStore != nil {
			if err := d.AuditStore.Append(execAudit); err != nil {
				log.Printf("labtether worker: failed to append command executed audit: %v", err)
			}
		}

		if d.Broadcast != nil {
			d.Broadcast("job.completed", map[string]any{"kind": "terminal_command"})
		}

		return nil
	}
}

// HandleActionRunJob returns a handler that executes action runs
// and writes results directly to stores.
func (d *Deps) HandleActionRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return func(ctx interface{ Deadline() (time.Time, bool) }, job *jobqueue.Job) error {
		var actionJob actions.Job
		if err := json.Unmarshal(job.Payload, &actionJob); err != nil {
			return fmt.Errorf("decode action job: %w", err)
		}

		processed.Add(1)
		result := d.ExecuteActionInProcess(actionJob)

		if err := d.ActionStore.ApplyActionResult(result); err != nil {
			return fmt.Errorf("apply action result: %w", err)
		}

		auditEvent := audit.NewEvent("actions.run.completed")
		auditEvent.ID = fmt.Sprintf("audit_action_completed_%s", result.JobID)
		auditEvent.SessionID = result.RunID
		auditEvent.Decision = result.Status
		auditEvent.Details = map[string]any{
			"output": result.Output,
			"job_id": result.JobID,
			"error":  result.Error,
		}
		if err := d.AuditStore.Append(auditEvent); err != nil {
			log.Printf("labtether worker: failed to append action completion audit: %v", err)
		}

		if err := d.LogStore.AppendEvent(logs.Event{
			ID:        fmt.Sprintf("log_action_completed_%s", result.JobID),
			Source:    "actions",
			Level:     d.mapLevel(result.Status),
			Message:   fmt.Sprintf("action run %s completed (%s)", result.RunID, result.Status),
			Timestamp: result.CompletedAt,
			Fields: map[string]string{
				"run_id": result.RunID,
				"job_id": result.JobID,
				"status": result.Status,
			},
		}); err != nil {
			log.Printf("labtether worker: failed to append action completion log: %v", err)
		}

		// Also publish audit for the execution itself.
		execAudit := audit.NewEvent("actions.run.executed")
		execAudit.ID = fmt.Sprintf("audit_action_executed_%s", actionJob.JobID)
		execAudit.ActorID = actionJob.ActorID
		execAudit.Target = actionJob.Target
		execAudit.SessionID = actionJob.RunID
		execAudit.Decision = result.Status
		execAudit.Details = map[string]any{
			"job_id":       result.JobID,
			"type":         actionJob.Type,
			"connector_id": actionJob.ConnectorID,
			"action_id":    actionJob.ActionID,
		}
		if err := d.AuditStore.Append(execAudit); err != nil {
			log.Printf("labtether worker: failed to append action executed audit: %v", err)
		}

		if d.Broadcast != nil {
			d.Broadcast("job.completed", map[string]any{"kind": "action_run"})
		}

		return nil
	}
}

// HandleUpdateRunJob returns a handler that executes update runs
// and writes results directly to stores.
func (d *Deps) HandleUpdateRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return func(ctx interface{ Deadline() (time.Time, bool) }, job *jobqueue.Job) error {
		var updateJob updates.Job
		if err := json.Unmarshal(job.Payload, &updateJob); err != nil {
			return fmt.Errorf("decode update job: %w", err)
		}

		processed.Add(1)
		result := opspkg.ExecuteUpdateWithExecutor(updateJob, d.ExecuteUpdateScope)

		if err := d.UpdateStore.ApplyUpdateResult(result); err != nil {
			return fmt.Errorf("apply update result: %w", err)
		}

		auditEvent := audit.NewEvent("updates.run.completed")
		auditEvent.ID = fmt.Sprintf("audit_update_completed_%s", result.JobID)
		auditEvent.SessionID = result.RunID
		auditEvent.Decision = result.Status
		auditEvent.Details = map[string]any{
			"summary": result.Summary,
			"job_id":  result.JobID,
			"error":   result.Error,
		}
		if err := d.AuditStore.Append(auditEvent); err != nil {
			log.Printf("labtether worker: failed to append update completion audit: %v", err)
		}

		if err := d.LogStore.AppendEvent(logs.Event{
			ID:        fmt.Sprintf("log_update_completed_%s", result.JobID),
			Source:    "updates",
			Level:     d.mapLevel(result.Status),
			Message:   fmt.Sprintf("update run %s completed (%s)", result.RunID, result.Status),
			Timestamp: result.CompletedAt,
			Fields: map[string]string{
				"run_id": result.RunID,
				"job_id": result.JobID,
				"status": result.Status,
			},
		}); err != nil {
			log.Printf("labtether worker: failed to append update completion log: %v", err)
		}

		// Also publish audit for the execution itself.
		execAudit := audit.NewEvent("updates.run.executed")
		execAudit.ID = fmt.Sprintf("audit_update_executed_%s", updateJob.JobID)
		execAudit.ActorID = updateJob.ActorID
		execAudit.SessionID = updateJob.RunID
		execAudit.Decision = result.Status
		execAudit.Details = map[string]any{
			"job_id":   result.JobID,
			"plan_id":  updateJob.Plan.ID,
			"dry_run":  updateJob.DryRun,
			"summary":  result.Summary,
			"task_cnt": len(result.Results),
		}
		if err := d.AuditStore.Append(execAudit); err != nil {
			log.Printf("labtether worker: failed to append update executed audit: %v", err)
		}

		if d.Broadcast != nil {
			d.Broadcast("job.completed", map[string]any{"kind": "update_run"})
		}

		return nil
	}
}

// RecordDeadLetter is the callback invoked when a job exceeds max attempts.
// It writes dead-letter entries to log_events and audit_events consumed by
// the /queue/dead-letters endpoint.
func (d *Deps) RecordDeadLetter(ctx context.Context, job *jobqueue.Job, jobErr error) {
	errMsg := "processing failed"
	if jobErr != nil {
		errMsg = strings.TrimSpace(jobErr.Error())
	}

	dlqEvent := jobqueue.NewDeadLetterEvent(
		"worker."+string(job.Kind),
		string(job.Kind),
		d.IntToUint64NonNegative(job.Attempts),
		job.Payload,
		jobErr,
	)

	if d.LogStore != nil {
		if err := d.LogStore.AppendEvent(logs.Event{
			ID:      fmt.Sprintf("log_dead_letter_%s", dlqEvent.ID),
			Source:  "dead_letter",
			Level:   "error",
			Message: fmt.Sprintf("dead-letter from worker.%s", job.Kind),
			Fields: map[string]string{
				"event_id":   dlqEvent.ID,
				"component":  dlqEvent.Component,
				"subject":    dlqEvent.Subject,
				"deliveries": fmt.Sprintf("%d", dlqEvent.Deliveries),
				"error":      errMsg,
			},
			Timestamp: dlqEvent.CreatedAt,
		}); err != nil {
			log.Printf("labtether worker: failed to persist dead-letter log event: %v", err)
		}
	}

	auditEvent := audit.NewEvent("queue.dead_letter")
	auditEvent.ID = fmt.Sprintf("audit_dead_letter_%s", dlqEvent.ID)
	auditEvent.Decision = "dead_lettered"
	auditEvent.Reason = errMsg
	auditEvent.Details = map[string]any{
		"event_id":   dlqEvent.ID,
		"component":  dlqEvent.Component,
		"subject":    dlqEvent.Subject,
		"deliveries": dlqEvent.Deliveries,
		"job_id":     job.ID,
	}
	if d.AuditStore != nil {
		if err := d.AuditStore.Append(auditEvent); err != nil {
			log.Printf("labtether worker: failed to persist dead-letter audit event: %v", err)
		}
	}
}

// mapLevel delegates to the injected MapCommandLevel function if set,
// falling back to a built-in mapping so callers don't need to wire the
// function when running tests without the full cmd/labtether stack.
func (d *Deps) mapLevel(status string) string {
	if d.MapCommandLevel != nil {
		return d.MapCommandLevel(status)
	}
	return defaultMapCommandLevel(status)
}

func defaultMapCommandLevel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success":
		return "info"
	case "failed", "error":
		return "error"
	default:
		return "warn"
	}
}
