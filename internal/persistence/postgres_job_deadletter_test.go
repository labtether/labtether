package persistence

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/jobqueue"
)

func TestPostgresFailedClaimRetriesWithoutFinalizingBusiness(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job, action := actionExecutionClaim(t, s)
	q := jobqueue.New(s.pool, time.Second, 3)
	finalized := false
	q.SetDeadLetterFinalizer(func(context.Context, pgx.Tx, *jobqueue.Job) error {
		finalized = true
		return errors.New("retry must not finalize business")
	})
	before := time.Now().UTC()
	if err := q.Fail(ctx, job.ID, job.LockToken, "retryable fixture failure"); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC()
	var state, token, failure string
	var attempts int
	var updated, available time.Time
	var locked, completed *time.Time
	err := s.pool.QueryRow(ctx, `SELECT status,COALESCE(lock_token,''),error,attempts,updated_at,available_at,locked_at,completed_at
 FROM job_queue WHERE id=$1`, job.ID).Scan(&state, &token, &failure, &attempts, &updated, &available, &locked, &completed)
	if err != nil {
		t.Fatal(err)
	}
	if finalized || state != "queued" || token != "" || locked != nil || completed != nil || attempts != 1 || failure != "retryable fixture failure" {
		t.Fatalf("retry state: finalized=%v state=%s token=%q locked=%v completed=%v attempts=%d error=%q", finalized, state, token, locked, completed, attempts, failure)
	}
	if updated.Before(before.Add(-time.Microsecond)) || updated.After(after) || available.Sub(updated) != time.Second {
		t.Fatalf("retry timestamps: before=%v updated=%v available=%v after=%v", before, updated, available, after)
	}
	run, _, err := s.GetActionRun(action.RunID)
	if err != nil || run.Status != "queued" || run.Error != "" {
		t.Fatalf("retry changed business state: %+v/%v", run, err)
	}
}

func TestPostgresDeadLetterBusinessCommitIsAtomic(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx := context.Background()
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "running"}[started], func(t *testing.T) {
			job, p := actionExecutionClaim(t, s)
			if started {
				if ok, e := s.BeginJobExecution(ctx, job); e != nil || !ok {
					t.Fatalf("begin=%v/%v", ok, e)
				}
			}
			if _, e := s.pool.Exec(ctx, `UPDATE job_queue SET max_attempts=attempts WHERE id=$1`, job.ID); e != nil {
				t.Fatal(e)
			}
			q := jobqueue.New(s.pool, time.Second, 1)
			injected := errors.New("fixture failure after business write, before commit")
			q.SetDeadLetterFinalizer(func(ctx context.Context, tx pgx.Tx, j *jobqueue.Job) error {
				if e := s.FinalizeDeadJobInTransaction(ctx, tx, j); e != nil {
					return e
				}
				return injected
			})
			if e := q.Fail(ctx, job.ID, job.LockToken, "fixture failure"); !errors.Is(e, injected) {
				t.Fatalf("injected failure=%v", e)
			}
			var queueState, token string
			if e := s.pool.QueryRow(ctx, `SELECT status,lock_token FROM job_queue WHERE id=$1`, job.ID).Scan(&queueState, &token); e != nil {
				t.Fatal(e)
			}
			run, _, e := s.GetActionRun(p.RunID)
			if e != nil {
				t.Fatal(e)
			}
			expected := "queued"
			if started {
				expected = "running"
			}
			if queueState != "processing" || token != job.LockToken || run.Status != expected || run.Error != "" {
				t.Fatalf("partial dead-letter commit: queue=%s/%s run=%+v", queueState, token, run)
			}
			q.SetDeadLetterFinalizer(s.FinalizeDeadJobInTransaction)
			if e = q.Fail(ctx, job.ID, job.LockToken, "fixture failure"); e != nil {
				t.Fatal(e)
			}
			run, _, e = s.GetActionRun(p.RunID)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.pool.QueryRow(ctx, `SELECT status FROM job_queue WHERE id=$1`, job.ID).Scan(&queueState); e != nil {
				t.Fatal(e)
			}
			phrase := "not executed"
			if started {
				phrase = "outcome unknown"
			}
			if queueState != "dead_lettered" || run.Status != "failed" || !strings.Contains(run.Error, phrase) {
				t.Fatalf("dead-letter result=%s/%+v", queueState, run)
			}
		})
	}
}

// Recovery selects all expired claims, so prove database isolation before the
// shared helper can apply migrations or any fixture can call recovery.
func requireOwnedRecoveryDatabase(t *testing.T) {
	t.Helper()
	name, jobID := os.Getenv("LABTETHER_TEST_DATABASE"), os.Getenv("LABTETHER_JOB_ID")
	if name == "" {
		t.Skip("explicitly owned temporary database required for stale recovery")
	}
	if !regexp.MustCompile(`^\d{8}T\d{6}Z-[a-f0-9]{12}$`).MatchString(jobID) ||
		name != "ltqa_claim_"+strings.ToLower(strings.ReplaceAll(jobID, "-", "_")) {
		t.Fatal("temporary database name does not match the resource job")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	config, err := pgx.ParseConfig(databaseURL)
	if databaseURL == "" || err != nil || config.Database != name || config.Host != "127.0.0.1" || config.Port != 55432 || config.User != "labtether" || len(config.Fallbacks) != 0 {
		t.Fatal("DATABASE_URL does not identify the exact temporary task database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("cannot verify temporary database ownership")
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = conn.Close(closeCtx)
	}()
	var actual, owner, comment string
	err = conn.QueryRow(ctx, `SELECT current_database(),pg_get_userbyid(datdba),
 COALESCE(shobj_description(oid,'pg_database'),'') FROM pg_database WHERE datname=current_database()`).Scan(&actual, &owner, &comment)
	if err != nil || actual != name || owner != "labtether" || comment != "LabTether QA "+jobID {
		t.Fatal("temporary database ownership stamp is missing or changed")
	}
}

func TestPostgresStaleRecoveryCommitsBusinessAndQueue(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var existing int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_queue`).Scan(&existing); err != nil || existing != 0 {
		t.Fatal("stale recovery fixture requires an empty owned queue")
	}
	exhausted, action := actionExecutionClaim(t, s)
	retry, _ := actionExecutionClaim(t, s)
	fresh, _ := actionExecutionClaim(t, s)
	locked, _ := actionExecutionClaim(t, s)
	if ok, err := s.BeginJobExecution(ctx, exhausted); err != nil || !ok {
		t.Fatalf("begin=%v/%v", ok, err)
	}
	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx, `UPDATE job_queue SET locked_at=$1 WHERE id=ANY($2::text[])`,
		now.Add(-time.Minute), []string{exhausted.ID, retry.ID, locked.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE job_queue SET max_attempts=attempts WHERE id=$1`, exhausted.ID); err != nil {
		t.Fatal(err)
	}
	hold, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackJobExecution(hold)
	if _, err = hold.Exec(ctx, `SELECT id FROM job_queue WHERE id=$1 FOR UPDATE`, locked.ID); err != nil {
		t.Fatal(err)
	}
	q := jobqueue.New(s.pool, time.Second, 3)
	injected := errors.New("fixture failure after recovery business write")
	q.SetDeadLetterFinalizer(func(ctx context.Context, tx pgx.Tx, job *jobqueue.Job) error {
		if err := s.FinalizeDeadJobInTransaction(ctx, tx, job); err != nil {
			return err
		}
		return injected
	})
	if _, err = q.RecoverStaleClaims(ctx, now); !errors.Is(err, injected) {
		t.Fatalf("recovery rollback error=%v", err)
	}
	checkQueue := func(job *jobqueue.Job, state, token string) {
		t.Helper()
		var gotState, gotToken string
		if err := s.pool.QueryRow(ctx, `SELECT status,COALESCE(lock_token,'') FROM job_queue WHERE id=$1`, job.ID).Scan(&gotState, &gotToken); err != nil {
			t.Fatal(err)
		}
		if gotState != state || gotToken != token {
			t.Fatalf("queue %s state/token=%s/%s, want %s/%s", job.ID, gotState, gotToken, state, token)
		}
	}
	for _, job := range []*jobqueue.Job{exhausted, retry, fresh, locked} {
		checkQueue(job, "processing", job.LockToken)
	}
	run, _, err := s.GetActionRun(action.RunID)
	if err != nil || run.Status != "running" || run.Error != "" {
		t.Fatalf("recovery business rollback=%+v/%v", run, err)
	}
	q.SetDeadLetterFinalizer(s.FinalizeDeadJobInTransaction)
	dead, err := q.RecoverStaleClaims(ctx, now)
	if err != nil || len(dead) != 1 || dead[0].ID != exhausted.ID {
		t.Fatalf("recovered dead jobs=%+v/%v", dead, err)
	}
	checkQueue(exhausted, "dead_lettered", "")
	checkQueue(retry, "queued", "")
	checkQueue(fresh, "processing", fresh.LockToken)
	checkQueue(locked, "processing", locked.LockToken)
	var available time.Time
	if err := s.pool.QueryRow(ctx, `SELECT available_at FROM job_queue WHERE id=$1`, retry.ID).Scan(&available); err != nil || !available.After(now) {
		t.Fatalf("retry backoff=%v/%v", available, err)
	}
	run, _, err = s.GetActionRun(action.RunID)
	if err != nil || run.Status != "failed" || !strings.Contains(run.Error, "outcome unknown") {
		t.Fatalf("recovered business result=%+v/%v", run, err)
	}
}
