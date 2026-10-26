package persistence

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

func TestPostgresAutoIncidentIsAtomicAndDeduplicated(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	owner := "atomic-incident-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID"))
	instanceID := owner + "-instance"
	req := incidents.CreateIncidentRequest{Title: "Atomic critical incident", Severity: incidents.SeverityCritical, CreatedBy: owner}
	var initial int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM incidents WHERE created_by=$1`, owner).Scan(&initial); err != nil || initial != 0 {
		t.Fatalf("reserved fixture identity exists: %d/%v", initial, err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `DELETE FROM incidents WHERE created_by=$1 AND source=$2`, owner, incidents.SourceAlertAuto); err != nil {
			t.Errorf("atomic incident cleanup: %v", err)
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT
			(SELECT COUNT(*) FROM incidents WHERE created_by=$1)+
			(SELECT COUNT(*) FROM incident_alert_links WHERE alert_instance_id=$2)`, owner, instanceID).Scan(&left); err != nil || left != 0 {
			t.Errorf("atomic incident fixture remains: %d/%v", left, err)
		}
	})
	type answer struct {
		incident incidents.Incident
		created  bool
		err      error
	}
	t.Run("canceled link rolls back incident", func(t *testing.T) {
		blocker, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		queryCtx, stop := context.WithCancel(ctx)
		result := make(chan answer, 1)
		started, received := false, false
		defer func() {
			stop()
			cleanup, finish := context.WithTimeout(context.Background(), 3*time.Second)
			defer finish()
			_ = blocker.Rollback(cleanup)
			if started && !received {
				select {
				case <-result:
				case <-cleanup.Done():
					t.Error("blocked incident creator did not drain")
				}
			}
		}()
		// SHARE allows the duplicate lookup, but blocks the link INSERT after
		// the new incident has been inserted inside the uncommitted transaction.
		if _, err := blocker.Exec(ctx, `LOCK TABLE incident_alert_links IN SHARE MODE`); err != nil {
			t.Fatal(err)
		}
		started = true
		go func() {
			inc, created, err := s.CreateAutoIncidentForAlertInstanceContext(queryCtx, instanceID, req)
			result <- answer{inc, created, err}
		}()
		observe, finish := context.WithTimeout(ctx, 2*time.Second)
		defer finish()
		for {
			// Activity rows are cached per transaction; refresh before observing a new backend.
			if _, err := blocker.Exec(observe, `SELECT pg_stat_clear_snapshot()`); err != nil {
				t.Fatalf("refresh lock observation snapshot: %v", err)
			}
			var waiting bool
			err := blocker.QueryRow(observe, `SELECT EXISTS (
				SELECT 1 FROM pg_locks l JOIN pg_stat_activity a ON a.pid=l.pid
				WHERE NOT l.granted AND l.locktype='relation' AND l.mode='RowExclusiveLock'
				AND l.relation='incident_alert_links'::regclass
				AND a.datid=(SELECT oid FROM pg_database WHERE datname=current_database())
				AND pg_backend_pid()=ANY(pg_blocking_pids(l.pid)))`).Scan(&waiting)
			if err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			select {
			case got := <-result:
				received = true
				t.Fatalf("creator did not reach the held link lock: %v", got.err)
			case <-observe.Done():
				t.Fatal("creator never blocked on its link insert")
			case <-time.After(5 * time.Millisecond):
			}
		}
		stop()
		select {
		case got := <-result:
			received = true
			if !errors.Is(got.err, context.Canceled) || got.created || got.incident.ID != "" {
				t.Fatalf("canceled link reported a committed incident: %+v", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("canceled creator did not return while link lock held")
		}
		var remaining int
		if err := blocker.QueryRow(ctx, `SELECT COUNT(*) FROM incidents WHERE created_by=$1`, owner).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatalf("canceled link left an orphan incident: %d/%v", remaining, err)
		}
		if err := blocker.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		requirePostgresPoolReleased(t, ctx, s.pool)
		if err := s.pool.Ping(ctx); err != nil {
			t.Fatal(err)
		}
	})
	if t.Failed() {
		return
	}

	// Competing creators must all observe one committed pair. This also
	// proves the canceled transaction released its per-instance lock.
	results := make(chan answer, 8)
	start := make(chan struct{})
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	for i := 0; i < cap(results); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			inc, created, err := s.CreateAutoIncidentForAlertInstanceContext(ctx, instanceID, req)
			results <- answer{inc, created, err}
		}()
	}
	close(start)
	createdID := ""
	for i := 0; i < cap(results); i++ {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.created {
				if createdID != "" || got.incident.Source != incidents.SourceAlertAuto {
					t.Fatal("concurrent creators made duplicate or manual incidents")
				}
				createdID = got.incident.ID
			}
		case <-ctx.Done():
			t.Fatal("concurrent creators did not finish")
		}
	}
	workers.Wait()
	var incidentCount, linkCount int
	if err := s.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM incidents WHERE created_by=$1 AND source=$2),
		(SELECT COUNT(*) FROM incident_alert_links WHERE alert_instance_id=$3 AND incident_id=$4 AND link_type=$5)`,
		owner, incidents.SourceAlertAuto, instanceID, createdID, incidents.LinkTypeTrigger).Scan(&incidentCount, &linkCount); err != nil || incidentCount != 1 || linkCount != 1 || createdID == "" {
		t.Fatalf("expected one complete automatic pair: %d/%d/%v", incidentCount, linkCount, err)
	}
	if _, created, err := s.CreateAutoIncidentForAlertInstanceContext(ctx, instanceID, req); err != nil || created {
		t.Fatalf("retry created a duplicate: %v/%v", created, err)
	}
}
