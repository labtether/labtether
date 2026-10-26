package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPostgresIncidentTimelinePagesRetainedEventsAndCancelsBlockedRead(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM incidents)+(SELECT COUNT(*) FROM incident_events)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("requires empty owned incident tables: %d/%v", count, err)
	}
	prefix := "timeline-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	incidentID, foreignID := prefix+"main", prefix+"foreign"
	owners := []string{incidentID, foreignID}
	// Both exact parent intents exist before writes. Their FK cascade owns only
	// these events, including a later inserted fixture; no shared table truncate.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `DELETE FROM incidents WHERE id=ANY($1::text[])`, owners); err != nil {
			t.Errorf("owned timeline cleanup: %v", err)
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT (SELECT COUNT(*) FROM incidents WHERE id=ANY($1::text[]))+(SELECT COUNT(*) FROM incident_events WHERE incident_id=ANY($1::text[]))`, owners).Scan(&left); err != nil || left != 0 {
			t.Errorf("owned timeline rows remain: %d/%v", left, err)
		}
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, id := range owners {
		exec(`INSERT INTO incidents(id,title,status,severity,source,created_by,opened_at,created_at,updated_at) VALUES($1,$1,'open','medium','manual','timeline-fixture',$2,$2,$2)`, id, at)
	}
	insert := func(id, owner string, occurredAt, createdAt time.Time) {
		t.Helper()
		exec(`INSERT INTO incident_events(id,incident_id,event_type,source_ref,summary,occurred_at,created_at) VALUES($1,$2,'audit',$1,'Retained history',$3,$4)`, id, owner, occurredAt, createdAt)
	}
	var want []string
	for i := 0; i < 513; i++ {
		id := fmt.Sprintf("%sevent-%03d", prefix, i)
		if i == 0 {
			id = prefix + "event-Ä"
		}
		insert(id, incidentID, at, at)
		want = append(want, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(want)))
	// The schema's legacy nullable creation time must not hide retained rows
	// or be replaced with an invented creation timestamp.
	legacyID := want[len(want)-1]
	exec(`UPDATE incident_events SET created_at=NULL WHERE id=$1`, legacyID)
	insert(prefix+"foreign-event", foreignID, at.Add(time.Hour), at)
	first, err := s.ListIncidentEventPage(ctx, incidentID, IncidentEventPageCursor{}, 999)
	if err != nil || len(first.Events) != 500 || first.Next == nil {
		t.Fatalf("bounded first page: %d/%v", len(first.Events), err)
	}
	// A new backfill cannot enter the existing browse just because its event
	// timestamp is old. A refresh with a new cutoff can see it later.
	insert(prefix+"later-backfill", incidentID, at.Add(-time.Hour), first.Next.ThroughCreatedAt.Add(time.Second))
	second, err := s.ListIncidentEventPage(ctx, incidentID, *first.Next, 128)
	if err != nil || len(second.Events) != 13 || second.Next != nil {
		t.Fatalf("history beyond500 or final EOF: %d/%v", len(second.Events), err)
	}
	got := append(first.Events, second.Events...)
	for i, row := range got {
		if row.ID != want[i] || row.IncidentID != incidentID || row.Summary != "Retained history" || !row.OccurredAt.Equal(at) {
			t.Fatalf("history gap, duplicate or foreign row at %d: %+v", i, row)
		}
		if row.ID == legacyID && !row.CreatedAt.IsZero() {
			t.Fatal("unknown creation time was fabricated")
		}
	}
	if _, err := s.ListIncidentEventPage(ctx, foreignID, *first.Next, 50); !errors.Is(err, ErrInvalidIncidentEventCursor) {
		t.Fatalf("cursor crossed incident: %v", err)
	}
	// Observe the read waiting on our table lock before cancelling it; simply
	// passing an expired context would not prove in-flight query cancellation.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, `LOCK TABLE incident_events IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithCancel(ctx)
	defer stop()
	result := make(chan error, 1)
	go func() {
		_, err := s.ListIncidentEventPage(blocked, incidentID, IncidentEventPageCursor{}, 50)
		result <- err
	}()
	observe, finishObservation := context.WithTimeout(ctx, 2*time.Second)
	defer finishObservation()
	for {
		var waiting bool
		err := tx.QueryRow(observe, `SELECT EXISTS(SELECT 1 FROM pg_locks
 WHERE relation='incident_events'::regclass AND NOT granted AND pid<>pg_backend_pid()
 AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting)
		if err != nil {
			t.Fatalf("did not observe blocked timeline read: %v", err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("timeline query returned before lock wait: %v", err)
		case <-observe.Done():
			t.Fatal("timeline read never reached owned lock")
		case <-time.After(5 * time.Millisecond):
		}
	}
	stop()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked SQL ignored cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled timeline read did not return")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	requirePostgresPoolReleased(t, ctx, s.pool)
}
