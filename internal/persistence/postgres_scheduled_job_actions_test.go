package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/schedules"
)

func TestPostgresScheduledChildrenRetainOccurrenceAndClaimOwner(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx := context.Background()
	for _, retire := range []string{"reclaim", "supersede", "disabled-before-start"} {
		t.Run(retire, func(t *testing.T) {
			id := idgen.New("schedule-fence")
			p := schedules.ExecutionJob{JobID: idgen.New("job-fence"), ScheduleID: id, ActorID: "owner", Command: "true", Targets: []string{"fixture-target"}, ScheduledFor: time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)}
			if e := s.CreateScheduledTask(ctx, schedules.ScheduledTask{ID: id, Name: "fence", CronExpr: "@hourly", Command: p.Command, Targets: p.Targets, Enabled: true, CreatedBy: p.ActorID, CreatedAt: p.ScheduledFor, NextRunAt: &p.ScheduledFor}); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				_, _ = s.pool.Exec(ctx, `DELETE FROM action_runs WHERE params->>'schedule_job_id'=$1`, p.JobID)
				_ = s.DeleteScheduledTask(ctx, id)
			})
			now := time.Now().UTC()
			if ok, err := s.ClaimScheduledTaskExecution(ctx, schedules.ExecutionClaim{
				ScheduleID: id, DueAt: p.ScheduledFor, ClaimedAt: now, NextRunAt: now.Add(time.Hour), Job: p, MaxAttempts: 3,
			}); err != nil || !ok {
				t.Fatalf("claim occurrence=%v/%v", ok, err)
			}
			t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM job_queue WHERE id=$1`, p.JobID) })
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			claim := &jobqueue.Job{ID: p.JobID, Kind: jobqueue.KindScheduleRun, Payload: data, LockToken: idgen.New("claim"), Status: jobqueue.StatusProcessing}
			if _, err = s.pool.Exec(ctx, `UPDATE job_queue SET status='processing',lock_token=$2,locked_at=NOW(),attempts=1 WHERE id=$1`, claim.ID, claim.LockToken); err != nil {
				t.Fatal(err)
			}
			if retire == "disabled-before-start" {
				if _, err = s.pool.Exec(ctx, `UPDATE scheduled_tasks SET enabled=FALSE WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				if _, err = s.pool.Exec(ctx, `UPDATE job_queue SET max_attempts=attempts WHERE id=$1`, claim.ID); err != nil {
					t.Fatal(err)
				}
				q := jobqueue.New(s.pool, time.Second, 1)
				q.SetDeadLetterFinalizer(s.FinalizeDeadJobInTransaction)
				if err = q.Fail(ctx, claim.ID, claim.LockToken, "worker stopped before begin"); err != nil {
					t.Fatal(err)
				}
				current, _, err := s.GetScheduledTask(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				var queueState string
				if err = s.pool.QueryRow(ctx, `SELECT status FROM job_queue WHERE id=$1`, claim.ID).Scan(&queueState); err != nil {
					t.Fatal(err)
				}
				if current.LastRunStatus != "skipped" || queueState != "dead_lettered" {
					t.Fatalf("disabled occurrence orphaned: schedule=%+v queue=%s", current, queueState)
				}
				return
			}
			if ok, e := s.BeginJobExecution(ctx, claim); e != nil || !ok {
				t.Fatalf("begin=%v/%v", ok, e)
			}
			child, e := s.StartScheduledAction(ctx, claim, "fixture-target")
			if e != nil {
				t.Fatal(e)
			}
			impostor, e := s.CreateActionRun(actions.ExecuteRequest{ActorID: p.ActorID, Type: actions.RunTypeCommand, Target: child.Target, Command: p.Command, Params: child.Params})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.pool.Exec(ctx, `UPDATE action_runs SET status='running' WHERE id=$1`, impostor.ID); e != nil {
				t.Fatal(e)
			}
			late := actions.Result{JobID: child.JobID, RunID: child.RunID, Status: "succeeded", Output: "old"}
			if retire == "reclaim" {
				next := *claim
				next.LockToken = idgen.New("new-claim")
				if _, e = s.pool.Exec(ctx, `UPDATE job_queue SET lock_token=$2 WHERE id=$1`, claim.ID, next.LockToken); e != nil {
					t.Fatal(e)
				}
				if e = s.FinishScheduledAction(ctx, claim, child, late); !errors.Is(e, jobqueue.ErrClaimLost) {
					t.Fatalf("old child result=%v", e)
				}
				if _, e = s.StartScheduledAction(ctx, claim, "fixture-other"); !errors.Is(e, jobqueue.ErrClaimLost) {
					t.Fatalf("old child dispatch=%v", e)
				}
				if ok, e := s.BeginJobExecution(ctx, &next); e != nil || ok {
					t.Fatalf("reclaimed schedule replayed=%v/%v", ok, e)
				}
			} else {
				known, e := s.StartScheduledAction(ctx, claim, "fixture-other")
				if e != nil {
					t.Fatal(e)
				}
				if _, e = s.pool.Exec(ctx, `UPDATE scheduled_tasks SET last_run_job_id='successor-occurrence',last_run_status='queued' WHERE id=$1`, id); e != nil {
					t.Fatal(e)
				}
				if e = s.FinishScheduledAction(ctx, claim, known, actions.Result{JobID: known.JobID, RunID: known.RunID, Status: "succeeded"}); e != nil {
					t.Fatal(e)
				}
				if e = s.FinishJobExecution(ctx, claim, JobExecutionResult{Schedule: &ScheduleExecutionResult{Status: "failed"}}); e != nil {
					t.Fatal(e)
				}
				current, _, e := s.GetScheduledTask(ctx, id)
				if e != nil {
					t.Fatal(e)
				}
				if current.LastRunJobID != "successor-occurrence" || current.LastRunStatus != "queued" {
					t.Fatalf("overwrote successor: %+v", current)
				}
				run, _, e := s.GetActionRun(known.RunID)
				if e != nil || run.Status != "succeeded" {
					t.Fatalf("known child result lost: %+v/%v", run, e)
				}
			}
			unrelated, _, e := s.GetActionRun(impostor.ID)
			if e != nil || unrelated.Status != "running" {
				t.Fatalf("caller params forged child ownership: %+v/%v", unrelated, e)
			}
			run, _, e := s.GetActionRun(child.RunID)
			if e != nil {
				t.Fatal(e)
			}
			if run.Status != "failed" || !strings.Contains(run.Error, "outcome unknown") {
				t.Fatalf("orphan child: %+v", run)
			}
			if e = s.FinishScheduledAction(ctx, claim, child, late); !errors.Is(e, jobqueue.ErrClaimLost) {
				t.Fatalf("late child overwrote retirement: %v", e)
			}
		})
	}
}
