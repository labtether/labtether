package persistence

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labtether/labtether/internal/alerts"
)

// Real SQL proof stays separate from Memory/handler checks. Observe a query
// waiting on our exact lock before canceling; pre-canceled input is insufficient.
func TestPostgresAlertEvaluationContextCancelsStoreWaits(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var existing int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM alert_rules)+
 (SELECT COUNT(*) FROM alert_instances)+(SELECT COUNT(*) FROM alert_evaluations)`).Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("requires empty owned alert tables: %d/%v", existing, err)
	}
	prefix := "eval-cancel-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID"))
	ruleID, instanceID, evaluationID := prefix+"-rule", prefix+"-instance", prefix+"-evaluation"
	// Exact identities are reserved before either insert. The rule FK cascade
	// owns only this fixture's instances/evaluations, including a failed write.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `DELETE FROM alert_rules WHERE id=$1`, ruleID); err != nil {
			t.Errorf("owned evaluation cleanup: %v", err)
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT (SELECT COUNT(*) FROM alert_rules WHERE id=$1)+
 (SELECT COUNT(*) FROM alert_instances WHERE rule_id=$1)+(SELECT COUNT(*) FROM alert_evaluations WHERE rule_id=$1)`, ruleID).Scan(&left); err != nil || left != 0 {
			t.Errorf("owned evaluation rows remain: %d/%v", left, err)
		}
	})
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	if _, err := s.pool.Exec(ctx, `INSERT INTO alert_rules
 (id,name,status,kind,severity,target_scope,condition,created_by,created_at,updated_at)
 VALUES($1,$1,'active','metric_threshold','high','global','{}','owned-context-fixture',$2,$2)`, ruleID, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO alert_instances
 (id,rule_id,fingerprint,status,severity,labels,annotations,started_at,last_fired_at,created_at,updated_at)
 VALUES($1,$2,$1,'pending','high','{}','{}',$3,$3,$3,$3)`, instanceID, ruleID, at); err != nil {
		t.Fatal(err)
	}

	blocked := func(t *testing.T, lock string, args []any, call func(context.Context) error) {
		t.Helper()
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		queryCtx, stop := context.WithCancel(ctx)
		result := make(chan error, 1)
		started, received := false, false
		defer func() {
			stop()
			cleanup, finish := context.WithTimeout(context.Background(), 2*time.Second)
			defer finish()
			_ = tx.Rollback(cleanup)
			if started && !received {
				select {
				case <-result:
				case <-cleanup.Done():
					t.Error("owned blocked operation did not drain")
				}
			}
		}()
		if _, err := tx.Exec(ctx, lock, args...); err != nil {
			t.Fatal(err)
		}
		started = true
		go func() { result <- call(queryCtx) }()
		observe, finishObservation := context.WithTimeout(ctx, 2*time.Second)
		defer finishObservation()
		for {
			// Activity rows are cached per transaction; refresh before observing a new backend.
			if _, err := tx.Exec(observe, `SELECT pg_stat_clear_snapshot()`); err != nil {
				t.Fatalf("refresh lock observation snapshot: %v", err)
			}
			var waiting bool
			err := tx.QueryRow(observe, `SELECT EXISTS(SELECT 1 FROM pg_locks l JOIN pg_stat_activity a ON a.pid=l.pid
 WHERE NOT l.granted AND l.pid<>pg_backend_pid()
 AND a.datid=(SELECT oid FROM pg_database WHERE datname=current_database())
 AND pg_backend_pid()=ANY(pg_blocking_pids(l.pid)))`).Scan(&waiting)
			if err != nil {
				t.Fatalf("observe exact owned blocker: %v", err)
			}
			if waiting {
				break
			}
			select {
			case err := <-result:
				received = true
				t.Fatalf("operation did not wait for held SQL lock: %v", err)
			case <-observe.Done():
				t.Fatal("operation never reached held SQL lock")
			case <-time.After(5 * time.Millisecond):
			}
		}
		stop()
		select {
		case err := <-result:
			received = true
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("blocked SQL ignored cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("canceled SQL did not return while lock was held")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		requirePostgresPoolReleased(t, ctx, s.pool)
		if err := s.pool.Ping(ctx); err != nil {
			t.Fatalf("pool not reusable after cancellation: %v", err)
		}
	}
	t.Run("asset inventory read", func(t *testing.T) {
		blocked(t, `LOCK TABLE assets IN ACCESS EXCLUSIVE MODE`, nil, func(ctx context.Context) error {
			_, err := s.ListAssetsContext(ctx)
			return err
		})
	})
	t.Run("evaluation transaction rollback", func(t *testing.T) {
		// NO KEY UPDATE permits the insert's FK check, then blocks the second
		// statement updating last_evaluated_at. Cancellation must roll both back.
		blocked(t, `SELECT id FROM alert_rules WHERE id=$1 FOR NO KEY UPDATE`, []any{ruleID}, func(ctx context.Context) error {
			_, err := s.RecordAlertEvaluationContext(ctx, ruleID, alerts.Evaluation{
				ID: evaluationID, Status: alerts.EvaluationStatusTriggered, EvaluatedAt: at.Add(time.Hour),
			})
			return err
		})
		var count int
		var unchanged bool
		if err := s.pool.QueryRow(ctx, `SELECT
 (SELECT COUNT(*) FROM alert_evaluations WHERE rule_id=$1),
 last_evaluated_at IS NULL AND last_auto_attempt_at IS NULL FROM alert_rules WHERE id=$1`, ruleID).Scan(&count, &unchanged); err != nil || count != 0 || !unchanged {
			t.Fatalf("canceled evaluation partially committed: %d/%v/%v", count, unchanged, err)
		}
	})
	t.Run("instance transition", func(t *testing.T) {
		blocked(t, `SELECT id FROM alert_instances WHERE id=$1 FOR UPDATE`, []any{instanceID}, func(ctx context.Context) error {
			_, err := s.UpdateAlertInstanceStatusContext(ctx, instanceID, alerts.InstanceStatusFiring)
			return err
		})
		var unchanged bool
		if err := s.pool.QueryRow(ctx, `SELECT status='pending' AND first_fired_at IS NULL
 AND firing_scope IS NULL AND resolved_at IS NULL AND last_fired_at=$2 AND updated_at=$2
 FROM alert_instances WHERE id=$1`, instanceID, at).Scan(&unchanged); err != nil || !unchanged {
			t.Fatalf("canceled transition changed instance: %v/%v", unchanged, err)
		}
	})
	t.Run("exhausted pool admission", func(t *testing.T) {
		cfg := s.pool.Config()
		cfg.MaxConns, cfg.MinConns = 1, 0
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		held, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Release()
		limited := &PostgresStore{pool: pool}
		before := pool.Stat().CanceledAcquireCount()
		waiting, stop := context.WithTimeout(ctx, 100*time.Millisecond)
		defer stop()
		result := make(chan error, 1)
		received := false
		go func() { _, err := limited.ListAssetsContext(waiting); result <- err }()
		defer func() {
			stop()
			held.Release()
			if !received {
				select {
				case <-result:
				case <-time.After(time.Second):
					t.Error("owned pool waiter did not drain")
				}
			}
		}()
		select {
		case err := <-result:
			received = true
			if !errors.Is(err, context.DeadlineExceeded) || pool.Stat().CanceledAcquireCount() != before+1 {
				t.Fatalf("pool wait did not cancel at the caller deadline: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("exhausted pool ignored caller deadline")
		}
		held.Release()
		if err := pool.Ping(ctx); err != nil || pool.Stat().AcquiredConns() != 0 {
			t.Fatalf("pool admission leaked or cannot be reused: %v", err)
		}
	})
}
