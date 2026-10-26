package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/model"
)

// The Memory selector check cannot prove SQL subject filtering or release of a
// real blocked query. Keep this case on a stamped, disposable database.
func TestPostgresResourceCapabilityBatchIsExactAndCancellable(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prefix := "cap-batch-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	requested := make([]string, AlertCapabilityBatchSize)
	for i := range requested {
		requested[i] = prefix + fmt.Sprintf("selected-%03d", i)
	}
	foreign := make([]string, AlertCapabilityBatchSize+1)
	for i := range foreign {
		foreign[i] = prefix + fmt.Sprintf("foreign-%03d", i)
	}
	ownedIDs := append(append([]string(nil), requested...), foreign...)
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM canonical_capability_sets
 WHERE subject_id=ANY($1::text[])`, ownedIDs).Scan(&count); err != nil || count != 0 {
		t.Fatalf("owned capability identities must be absent: %d/%v", count, err)
	}
	// Register all exact IDs before any write, including both subject types.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `DELETE FROM canonical_capability_sets
 WHERE subject_id=ANY($1::text[])`, ownedIDs); err != nil {
			t.Errorf("owned capability cleanup: %v", err)
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT COUNT(*) FROM canonical_capability_sets
 WHERE subject_id=ANY($1::text[])`, ownedIDs).Scan(&left); err != nil || left != 0 {
			t.Errorf("owned capability rows remain: %d/%v", left, err)
		}
	})
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	insert := func(subject string, ids []string, capability string, updated time.Time) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, `INSERT INTO canonical_capability_sets
 (subject_type,subject_id,capabilities,updated_at)
 SELECT $1,id,jsonb_build_array(jsonb_build_object('id',$3::text,'scope','read')),$4
 FROM unnest($2::text[]) AS item(id)`, subject, ids, capability, updated); err != nil {
			t.Fatal(err)
		}
	}
	insert("resource", requested[:len(requested)-1], "selected-capability", at)
	// One provider shadows a real resource; the final requested ID has only a
	// provider row. Neither may substitute for a resource capability set.
	insert("provider", []string{requested[0], requested[len(requested)-1]}, "provider-decoy", at.Add(time.Hour))
	insert("resource", foreign, "foreign-decoy", at.Add(2*time.Hour))
	assertExact := func(rows []model.CapabilitySet, err error) {
		t.Helper()
		if err != nil || len(rows) != len(requested)-1 {
			t.Fatalf("exact resource batch: rows=%d error=%v", len(rows), err)
		}
		want := make(map[string]bool, len(requested)-1)
		for _, id := range requested[:len(requested)-1] {
			want[id] = true
		}
		for _, row := range rows {
			if !want[row.SubjectID] || row.SubjectType != "resource" || !row.UpdatedAt.Equal(at) ||
				len(row.Capabilities) != 1 || row.Capabilities[0].ID != "selected-capability" ||
				row.Capabilities[0].Scope != model.CapabilityScopeRead {
				t.Fatalf("wrong, duplicate or substituted resource capability: %+v", row)
			}
			delete(want, row.SubjectID)
		}
		if len(want) != 0 {
			t.Fatalf("requested resources omitted: %d", len(want))
		}
	}
	assertExact(s.GetResourceCapabilitySetsContext(ctx, requested))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	queryCtx, stopQuery := context.WithCancel(ctx)
	type response struct {
		rows []model.CapabilitySet
		err  error
	}
	result := make(chan response, 1)
	started, received := false, false
	defer func() {
		stopQuery()
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
		if started && !received {
			select {
			case <-result:
			case <-cleanup.Done():
				t.Error("owned capability reader did not drain")
			}
		}
	}()
	if _, err := tx.Exec(ctx, `LOCK TABLE canonical_capability_sets IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	started = true
	go func() {
		rows, err := s.GetResourceCapabilitySetsContext(queryCtx, requested)
		result <- response{rows, err}
	}()
	observe, stopObservation := context.WithTimeout(ctx, 2*time.Second)
	defer stopObservation()
	for {
		// Activity rows are cached per transaction; refresh before observing a new backend.
		if _, err := tx.Exec(observe, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatalf("refresh lock observation snapshot: %v", err)
		}
		var waiting bool
		err := tx.QueryRow(observe, `SELECT EXISTS(SELECT 1 FROM pg_locks l
 JOIN pg_stat_activity a ON a.pid=l.pid WHERE NOT l.granted
 AND l.relation='canonical_capability_sets'::regclass AND l.mode='AccessShareLock'
 AND a.datid=(SELECT oid FROM pg_database WHERE datname=current_database())
 AND pg_backend_pid()=ANY(pg_blocking_pids(l.pid)))`).Scan(&waiting)
		if err != nil {
			t.Fatalf("observe exact capability table blocker: %v", err)
		}
		if waiting {
			break
		}
		select {
		case reply := <-result:
			received = true
			t.Fatalf("capability query did not wait on the owned lock: %v", reply.err)
		case <-observe.Done():
			t.Fatal("capability query never reached the owned lock")
		case <-time.After(5 * time.Millisecond):
		}
	}
	stopQuery()
	select {
	case reply := <-result:
		received = true
		if !errors.Is(reply.err, context.Canceled) || reply.rows != nil {
			t.Fatalf("blocked cancellation returned usable data: rows=%d error=%v", len(reply.rows), reply.err)
		}
	case <-time.After(time.Second):
		t.Fatal("capability query ignored cancellation while lock remained held")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	requirePostgresPoolReleased(t, ctx, s.pool)
	if err := s.pool.Ping(ctx); err != nil {
		t.Fatalf("pool is not reusable after cancellation: %v", err)
	}
	assertExact(s.GetResourceCapabilitySetsContext(ctx, requested))
}
