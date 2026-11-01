package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
)

func (o jobBusiness) writeResult(ctx context.Context, tx pgx.Tx, result JobExecutionResult) error {
	count := 0
	if result.Terminal != nil {
		count++
	}
	if result.Action != nil {
		count++
	}
	if result.Update != nil {
		count++
	}
	if result.Schedule != nil {
		count++
	}
	if count != 1 {
		return errors.New("execution result must identify exactly one business record")
	}
	switch {
	case o.terminal != nil && result.Terminal != nil:
		p, r := o.terminal, result.Terminal
		if r.JobID != p.JobID || r.SessionID != p.SessionID || r.CommandID != p.CommandID {
			return errors.New("terminal result identity mismatch")
		}
		if !finalJobStatus(r.Status) {
			return errors.New("terminal result is not final")
		}
		_, err := tx.Exec(ctx, `UPDATE terminal_commands SET status=$2,output=$3,exit_code=$4,updated_at=NOW() WHERE id=$1`, p.CommandID, r.Status, r.Output, r.ExitCode)
		// Do not lock the session after its child: session deletion locks in the
		// opposite order. last_action_at was already recorded at command creation.
		return err
	case o.action != nil && result.Action != nil:
		p, r := o.action, result.Action
		if r.JobID != p.JobID || r.RunID != p.RunID {
			return errors.New("action result identity mismatch")
		}
		return writeActionJobResult(ctx, tx, *r)
	case o.update != nil && result.Update != nil:
		p, r := o.update, result.Update
		if r.JobID != p.JobID || r.RunID != p.RunID {
			return errors.New("update result identity mismatch")
		}
		if !finalJobStatus(r.Status) {
			return errors.New("update result is not final")
		}
		data, err := marshalUpdateRunResults(r.Results)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE update_runs SET status=$2,summary=$3,error=$4,results=$5::jsonb,
   updated_at=$6,completed_at=$6 WHERE id=$1`, p.RunID, r.Status, r.Summary, r.Error, data, jobCompletionTime(r.CompletedAt))
		return err
	case o.schedule != nil && result.Schedule != nil:
		r := result.Schedule
		switch r.Status {
		case "succeeded", "failed", "partial", "blocked", "skipped", "cancelled":
		default:
			return errors.New("schedule result is not final")
		}
		// A reclaimed occurrence must also close its surviving child records. It
		// never rewrites a completed child's known outcome or replays that command.
		err := closeInterruptedScheduleChildren(ctx, tx, o)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE scheduled_tasks SET last_run_status=$2,last_error=$3 WHERE id=$1`, o.schedule.ScheduleID, r.Status, truncateScheduledTaskError(r.Error))
		return err
	}
	return errors.New("execution result kind mismatch")
}

func finalJobStatus(status string) bool {
	return status == actions.StatusSucceeded || status == actions.StatusFailed
}
func jobCompletionTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func writeActionJobResult(ctx context.Context, tx pgx.Tx, r actions.Result) error {
	if !finalJobStatus(r.Status) {
		return errors.New("action result is not final")
	}
	completed := jobCompletionTime(r.CompletedAt)
	if _, err := tx.Exec(ctx, `UPDATE action_runs SET status=$2,output=$3,error=$4,updated_at=$5,completed_at=$5 WHERE id=$1`, r.RunID, r.Status, strings.TrimSpace(r.Output), strings.TrimSpace(r.Error), completed); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM action_run_steps WHERE run_id=$1`, r.RunID); err != nil {
		return err
	}
	for _, step := range r.Steps {
		status := actions.NormalizeStatus(step.Status)
		if status == "" {
			status = actions.StatusFailed
		}
		if _, err := tx.Exec(ctx, `INSERT INTO action_run_steps(id,run_id,name,status,output,error,exit_code,created_at,updated_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)`, idgen.New("actstep"), r.RunID, strings.TrimSpace(step.Name), status, strings.TrimSpace(step.Output), strings.TrimSpace(step.Error), step.ExitCode, completed); err != nil {
			return err
		}
	}
	return nil
}

func closeInterruptedScheduleChildren(ctx context.Context, tx pgx.Tx, o jobBusiness) error {
	if o.schedule == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE action_runs SET status='failed',error=$3,updated_at=NOW(),completed_at=NOW()
 WHERE params->>'schedule_job_id'=$1 AND params->>'schedule_id'=$2 AND status='running'
 AND left(id,9)='schedrun_' AND actor_id=$4 AND command=$5 AND type='command'`,
		o.schedule.JobID, o.schedule.ScheduleID, InterruptedJobOutcome, o.schedule.ActorID, o.schedule.Command)
	return err
}
