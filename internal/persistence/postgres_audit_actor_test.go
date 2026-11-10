package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/audit"
)

// A reader limited to their own audit rows must get exactly those, newest
// first, and paging must count only their rows (a viewer's page 2 is their
// own next row, not whatever is left of everyone's page 2).
func TestPostgresAuditListByActorPagesOwnRows(t *testing.T) {
	s := newTestPostgresStore(t)
	suffix := time.Now().UTC().UnixNano()
	mine := fmt.Sprintf("usr-audit-actor-%d", suffix)
	other := fmt.Sprintf("usr-audit-other-%d", suffix)
	base := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{}
	for i, actor := range []string{mine, other, mine, other, mine} {
		id := fmt.Sprintf("audit-actor-%d-%d", suffix, i)
		ids = append(ids, id)
		if err := s.Append(audit.Event{ID: id, Type: "webhook.created", ActorID: actor, Target: fmt.Sprintf("wh-%d", i),
			Timestamp: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id = ANY($1::text[])`, ids)
	})

	targets := func(limit, offset int) []string {
		t.Helper()
		events, err := s.ListByActor(mine, limit, offset)
		if err != nil {
			t.Fatalf("ListByActor: %v", err)
		}
		out := []string{}
		for _, event := range events {
			if event.ActorID != mine {
				t.Fatalf("row by %q in %q's list", event.ActorID, mine)
			}
			out = append(out, event.Target)
		}
		return out
	}
	if got := fmt.Sprint(targets(10, 0)); got != "[wh-4 wh-2 wh-0]" {
		t.Fatalf("own rows = %s, want [wh-4 wh-2 wh-0]", got)
	}
	if got := fmt.Sprint(targets(1, 1)); got != "[wh-2]" {
		t.Fatalf("own page 2 = %s, want [wh-2]", got)
	}
	if none, err := s.ListByActor(" ", 10, 0); err != nil || len(none) != 0 {
		t.Fatalf("empty actor = %d rows (%v), want none", len(none), err)
	}
}
