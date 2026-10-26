package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

func seedExecutionClaim(t *testing.T, s *PostgresStore, kind jobqueue.JobKind, id string, payload any) *jobqueue.Job {
	t.Helper()
	if kind != jobqueue.KindScheduleRun {
		id = idgen.New("jq-fixture")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	j := &jobqueue.Job{ID: id, Kind: kind, Status: jobqueue.StatusProcessing, Payload: data, LockToken: idgen.New("claim"), Attempts: 1}
	_, err = s.pool.Exec(context.Background(), `INSERT INTO job_queue(id,kind,status,payload,lock_token,locked_at,attempts,max_attempts,created_at,updated_at)
 VALUES($1,$2,'processing',$3::jsonb,$4,NOW(),1,3,NOW(),NOW())`, id, string(kind), string(data), j.LockToken)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM job_queue WHERE id=$1`, id) })
	return j
}

func actionExecutionClaim(t *testing.T, s *PostgresStore) (*jobqueue.Job, actions.Job) {
	t.Helper()
	run, err := s.CreateActionRun(actions.ExecuteRequest{ActorID: "queue-fence-fixture", Type: actions.RunTypeCommand, Target: "fixture-target", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.DeleteActionRun(run.ID) })
	p := actions.Job{JobID: idgen.New("fence-job"), RunID: run.ID, Type: run.Type, ActorID: run.ActorID, Target: run.Target, Command: run.Command}
	return seedExecutionClaim(t, s, jobqueue.KindActionRun, p.JobID, p), p
}

// This proves actual database lock ownership, not a token precheck. While a
// business row blocks finalization, the queue row must remain locked too.
func TestPostgresJobCompletionFencesReclaimAndResult(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job, p := actionExecutionClaim(t, s)
	if ok, err := s.BeginJobExecution(ctx, job); err != nil || !ok {
		t.Fatalf("begin=%v/%v", ok, err)
	}
	hold, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackJobExecution(hold)
	if _, err = hold.Exec(ctx, `SELECT id FROM action_runs WHERE id=$1 FOR UPDATE`, p.RunID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- s.FinishJobExecution(ctx, job, JobExecutionResult{Action: &actions.Result{JobID: p.JobID, RunID: p.RunID, Status: "succeeded", Output: "real-result", Steps: []actions.StepResult{{Name: "execute", Status: "succeeded"}}}})
	}()
	// Probe only this fixture row. NOWAIT must fail once Finish owns its queue.
	deadline := time.Now().Add(2 * time.Second)
	for {
		probe, e := s.pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = probe.Exec(ctx, `SELECT id FROM job_queue WHERE id=$1 FOR UPDATE NOWAIT`, job.ID)
		rollbackJobExecution(probe)
		var pgErr *pgconn.PgError
		if errors.As(e, &pgErr) && pgErr.Code == "55P03" {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if time.Now().After(deadline) {
			t.Fatal("finalizer did not retain queue lock while business row was blocked")
		}
		time.Sleep(5 * time.Millisecond)
	}
	reclaimed := make(chan int64, 1)
	go func() {
		tag, e := s.pool.Exec(ctx, `UPDATE job_queue SET lock_token='successor' WHERE id=$1 AND status='processing' AND lock_token=$2`, job.ID, job.LockToken)
		if e != nil {
			reclaimed <- -1
			return
		}
		reclaimed <- tag.RowsAffected()
	}()
	if err = hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if n := <-reclaimed; n != 0 {
		t.Fatalf("reclaimed committed queue: %d", n)
	}
	run, _, err := s.GetActionRun(p.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || run.Output != "real-result" || len(run.Steps) != 1 || job.Status != jobqueue.StatusCompleted {
		t.Fatalf("non-atomic result: %+v queue=%s", run, job.Status)
	}
	if err = jobqueue.New(s.pool, time.Second, 3).RenewClaim(ctx, job.ID, job.LockToken); !errors.Is(err, jobqueue.ErrClaimLost) {
		t.Fatalf("completed claim renewal=%v", err)
	}
}

func TestPostgresLostAttemptCannotPublishOrReplay(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx := context.Background()
	for _, kind := range []jobqueue.JobKind{jobqueue.KindTerminalCommand, jobqueue.KindActionRun, jobqueue.KindUpdateRun} {
		t.Run(string(kind), func(t *testing.T) {
			var job *jobqueue.Job
			var result JobExecutionResult
			var read func() (string, string)
			switch kind {
			case jobqueue.KindActionRun:
				j, p := actionExecutionClaim(t, s)
				job = j
				result.Action = &actions.Result{JobID: p.JobID, RunID: p.RunID, Status: "succeeded", Output: "stale"}
				read = func() (string, string) {
					r, _, e := s.GetActionRun(p.RunID)
					if e != nil {
						t.Fatal(e)
					}
					return r.Status, r.Error
				}
			case jobqueue.KindTerminalCommand:
				session, e := s.CreateSession(terminal.CreateSessionRequest{ActorID: "owner", Target: "fixture-target", Mode: "structured"})
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { _ = s.DeleteTerminalSession(session.ID) })
				cmd, e := s.AddCommand(session.ID, terminal.CreateCommandRequest{ActorID: "owner", Command: "true"}, session.Target, session.Mode)
				if e != nil {
					t.Fatal(e)
				}
				p := terminal.CommandJob{JobID: idgen.New("fence-job"), SessionID: session.ID, CommandID: cmd.ID, ActorID: cmd.ActorID, Target: cmd.Target, Command: cmd.Body, Mode: cmd.Mode}
				job = seedExecutionClaim(t, s, kind, p.JobID, p)
				result.Terminal = &terminal.CommandResult{JobID: p.JobID, SessionID: p.SessionID, CommandID: p.CommandID, Status: "succeeded", Output: "stale"}
				read = func() (string, string) {
					r, _, e := s.GetCommand(cmd.ID)
					if e != nil {
						t.Fatal(e)
					}
					return r.Status, r.Output
				}
			case jobqueue.KindUpdateRun:
				plan, e := s.CreateUpdatePlan(updates.CreatePlanRequest{Name: "fence-fixture", Targets: []string{"fixture-target"}})
				if e != nil {
					t.Fatal(e)
				}
				run, e := s.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "owner"})
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { _ = s.DeleteUpdateRun(run.ID); _ = s.DeleteUpdatePlan(plan.ID) })
				p := updates.Job{JobID: idgen.New("fence-job"), RunID: run.ID, ActorID: run.ActorID, DryRun: run.DryRun, Plan: plan}
				job = seedExecutionClaim(t, s, kind, p.JobID, p)
				result.Update = &updates.Result{JobID: p.JobID, RunID: p.RunID, Status: "succeeded", Summary: "stale"}
				read = func() (string, string) {
					r, _, e := s.GetUpdateRun(run.ID)
					if e != nil {
						t.Fatal(e)
					}
					return r.Status, r.Error
				}
			}
			if ok, e := s.BeginJobExecution(ctx, job); e != nil || !ok {
				t.Fatalf("begin=%v/%v", ok, e)
			}
			successor := *job
			successor.LockToken = idgen.New("claim")
			if _, e := s.pool.Exec(ctx, `UPDATE job_queue SET lock_token=$2,attempts=2 WHERE id=$1`, job.ID, successor.LockToken); e != nil {
				t.Fatal(e)
			}
			if e := s.FinishJobExecution(ctx, job, result); !errors.Is(e, jobqueue.ErrClaimLost) {
				t.Fatalf("stale result=%v", e)
			}
			if ok, e := s.BeginJobExecution(ctx, &successor); e != nil || ok {
				t.Fatalf("replayed running attempt=%v/%v", ok, e)
			}
			state, reason := read()
			if state != "failed" || !strings.Contains(reason, "outcome unknown") || successor.Status != jobqueue.StatusCompleted {
				t.Fatalf("result=%s/%s queue=%s", state, reason, successor.Status)
			}
			if e := s.FinishJobExecution(ctx, job, result); !errors.Is(e, jobqueue.ErrClaimLost) {
				t.Fatalf("late result=%v", e)
			}
		})
	}
}

func TestPostgresJobClaimCannotRedirectBusinessIdentity(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx := context.Background()
	job, p := actionExecutionClaim(t, s)
	_, other := actionExecutionClaim(t, s)
	forged := *job
	redirected := p
	redirected.RunID = other.RunID
	forged.Payload, _ = json.Marshal(redirected)
	if _, err := s.BeginJobExecution(ctx, &forged); !errors.Is(err, jobqueue.ErrClaimLost) {
		t.Fatalf("altered payload accepted: %v", err)
	}
	// Even the exact durable payload cannot dispatch a row whose target changed.
	if _, err := s.pool.Exec(ctx, `UPDATE action_runs SET target='different-target' WHERE id=$1`, p.RunID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.BeginJobExecution(ctx, job); err == nil || ok {
		t.Fatalf("changed business identity accepted: %v/%v", ok, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE action_runs SET target=$2 WHERE id=$1`, p.RunID, p.Target); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.BeginJobExecution(ctx, job); err != nil || !ok {
		t.Fatalf("begin=%v/%v", ok, err)
	}
	if err := s.FinishJobExecution(ctx, job, JobExecutionResult{Action: &actions.Result{JobID: p.JobID, RunID: other.RunID, Status: "succeeded"}}); err == nil {
		t.Fatal("redirected result accepted")
	}
	for _, id := range []string{p.RunID, other.RunID} {
		run, _, err := s.GetActionRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == "succeeded" {
			t.Fatal("wrong business record mutated")
		}
	}
	if err := s.FinishJobExecution(ctx, job, JobExecutionResult{Action: &actions.Result{JobID: p.JobID, RunID: p.RunID, Status: "succeeded"}}); err != nil {
		t.Fatal(err)
	}
}
