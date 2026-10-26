package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

var errJobBusinessIdentity = errors.New("job business identity is invalid or changed")

type jobBusiness struct {
	status   string
	terminal *terminal.CommandJob
	action   *actions.Job
	update   *updates.Job
	schedule *schedules.ExecutionJob
}

func lockJobBusiness(ctx context.Context, tx pgx.Tx, job *jobqueue.Job) (jobBusiness, error) {
	owner := jobBusiness{}
	var err error
	var logicalID string
	switch job.Kind {
	case jobqueue.KindTerminalCommand:
		var p terminal.CommandJob
		if err = json.Unmarshal(job.Payload, &p); err != nil {
			return owner, fmt.Errorf("%w: %v", errJobBusinessIdentity, err)
		}
		owner.terminal = &p
		logicalID = p.JobID
		err = tx.QueryRow(ctx, `SELECT status FROM terminal_commands WHERE id=$1 AND session_id=$2
   AND actor_id=$3 AND target=$4 AND body=$5 AND mode=$6 FOR UPDATE`,
			p.CommandID, p.SessionID, p.ActorID, p.Target, p.Command, p.Mode).Scan(&owner.status)
	case jobqueue.KindActionRun:
		var p actions.Job
		if err = json.Unmarshal(job.Payload, &p); err != nil {
			return owner, fmt.Errorf("%w: %v", errJobBusinessIdentity, err)
		}
		owner.action = &p
		logicalID = p.JobID
		owner.status, err = lockActionJob(ctx, tx, p)
	case jobqueue.KindUpdateRun:
		var p updates.Job
		if err = json.Unmarshal(job.Payload, &p); err != nil {
			return owner, fmt.Errorf("%w: %v", errJobBusinessIdentity, err)
		}
		owner.update = &p
		logicalID = p.JobID
		err = tx.QueryRow(ctx, `SELECT status FROM update_runs WHERE id=$1 AND plan_id=$2
   AND actor_id=$3 AND dry_run=$4 AND plan_name=$5 FOR UPDATE`, p.RunID, p.Plan.ID, p.ActorID, p.DryRun, p.Plan.Name).Scan(&owner.status)
	case jobqueue.KindScheduleRun:
		var p schedules.ExecutionJob
		if err = json.Unmarshal(job.Payload, &p); err != nil {
			return owner, fmt.Errorf("%w: %v", errJobBusinessIdentity, err)
		}
		owner.schedule = &p
		logicalID = p.JobID
		var currentJob string
		var enabled bool
		err = tx.QueryRow(ctx, `SELECT COALESCE(last_run_job_id,''),COALESCE(last_run_status,''),enabled
   FROM scheduled_tasks WHERE id=$1 FOR UPDATE`, p.ScheduleID).Scan(&currentJob, &owner.status, &enabled)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && currentJob != p.JobID) {
			owner.status = "superseded"
			err = nil
		}
		if err == nil && !enabled && owner.status == "queued" {
			owner.status = "disabled"
		}
	default:
		return owner, fmt.Errorf("%w: unsupported kind %s", errJobBusinessIdentity, job.Kind)
	}
	// Ordinary Enqueue uses a separate jq ID; the immutable payload binds its
	// logical JobID. Scheduled occurrences intentionally use the same ID.
	if logicalID == "" || (job.Kind == jobqueue.KindScheduleRun && logicalID != job.ID) {
		return owner, errJobBusinessIdentity
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = errJobBusinessIdentity
	}
	return owner, err
}

func lockActionJob(ctx context.Context, tx pgx.Tx, p actions.Job) (string, error) {
	params, err := marshalStringMap(p.Params)
	if err != nil {
		return "", err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM action_runs WHERE id=$1 AND type=$2 AND actor_id=$3
  AND COALESCE(target,'')=$4 AND COALESCE(command,'')=$5 AND COALESCE(connector_id,'')=$6
  AND COALESCE(action_id,'')=$7 AND COALESCE(params,'{}'::jsonb)=$8::jsonb AND dry_run=$9 FOR UPDATE`,
		p.RunID, p.Type, p.ActorID, p.Target, p.Command, p.ConnectorID, p.ActionID, params, p.DryRun).Scan(&status)
	return status, err
}

func (o jobBusiness) markRunning(ctx context.Context, tx pgx.Tx) error {
	var err error
	switch {
	case o.terminal != nil:
		_, err = tx.Exec(ctx, `UPDATE terminal_commands SET status='running',updated_at=NOW() WHERE id=$1`, o.terminal.CommandID)
	case o.action != nil:
		_, err = tx.Exec(ctx, `UPDATE action_runs SET status='running',updated_at=NOW() WHERE id=$1`, o.action.RunID)
	case o.update != nil:
		_, err = tx.Exec(ctx, `UPDATE update_runs SET status='running',updated_at=NOW() WHERE id=$1`, o.update.RunID)
	case o.schedule != nil:
		_, err = tx.Exec(ctx, `UPDATE scheduled_tasks SET last_run_status='running' WHERE id=$1`, o.schedule.ScheduleID)
	}
	return err
}

func (o jobBusiness) interruptedResult() JobExecutionResult {
	return o.failedResult(InterruptedJobOutcome)
}

func (o jobBusiness) failedResult(reason string) JobExecutionResult {
	r := JobExecutionResult{}
	switch {
	case o.terminal != nil:
		p := o.terminal
		r.Terminal = &terminal.CommandResult{JobID: p.JobID, SessionID: p.SessionID, CommandID: p.CommandID, Status: "failed", Output: reason}
	case o.action != nil:
		p := o.action
		r.Action = &actions.Result{JobID: p.JobID, RunID: p.RunID, Status: "failed", Error: reason}
	case o.update != nil:
		p := o.update
		r.Update = &updates.Result{JobID: p.JobID, RunID: p.RunID, Status: "failed", Error: reason}
	case o.schedule != nil:
		r.Schedule = &ScheduleExecutionResult{Status: "failed", Error: reason}
	}
	return r
}
