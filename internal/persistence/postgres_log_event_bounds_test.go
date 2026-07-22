package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/logs"
)

func TestPostgresLogEventBoundsAreAtomicAndBindSafe(t *testing.T) {
	store := newTestPostgresStore(t)
	prefix := fmt.Sprintf("ltqa-log-bounds-%d", time.Now().UTC().UnixNano())
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM log_events WHERE id LIKE $1`, prefix+"%")
	})

	exact := make([]logs.Event, logs.MaxEventsPerBatch)
	for index := range exact {
		exact[index] = logs.Event{ID: fmt.Sprintf("%s-exact-%04d", prefix, index), Source: "agent", Level: "info", Message: "event"}
	}
	if err := store.AppendEvents(exact); err != nil {
		t.Fatalf("append exact bind-safe batch: %v", err)
	}
	assertPostgresLogPrefixCount(t, store, prefix+"-exact-", logs.MaxEventsPerBatch)

	overCount := make([]logs.Event, logs.MaxEventsPerBatch+1)
	for index := range overCount {
		overCount[index] = logs.Event{ID: fmt.Sprintf("%s-over-%04d", prefix, index), Message: "event"}
	}
	if err := store.AppendEvents(overCount); !errors.Is(err, logs.ErrEventBatchLimitExceeded) {
		t.Fatalf("over-count batch error = %v", err)
	}
	assertPostgresLogPrefixCount(t, store, prefix+"-over-", 0)

	if err := store.AppendEvents([]logs.Event{
		{ID: prefix + "-atomic-valid", Message: "valid"},
		{ID: prefix + "-atomic-invalid", Message: "bad\x00message"},
	}); !errors.Is(err, logs.ErrInvalidEventText) {
		t.Fatalf("invalid atomic batch error = %v", err)
	}
	assertPostgresLogPrefixCount(t, store, prefix+"-atomic-", 0)
	if err := store.AppendEvents([]logs.Event{
		{ID: prefix + "-utf8-valid", Message: "valid"},
		{ID: prefix + "-utf8-invalid", Message: string([]byte{0xff})},
	}); !errors.Is(err, logs.ErrInvalidEventText) {
		t.Fatalf("invalid UTF-8 atomic batch error = %v", err)
	}
	assertPostgresLogPrefixCount(t, store, prefix+"-utf8-", 0)

	if err := store.AppendEvent(logs.Event{ID: prefix + "-oversized", Message: strings.Repeat("x", logs.MaxEventMessageBytes+1)}); !errors.Is(err, logs.ErrEventLimitExceeded) {
		t.Fatalf("oversized individual error = %v", err)
	}
	assertPostgresLogPrefixCount(t, store, prefix+"-oversized", 0)
}

func assertPostgresLogPrefixCount(t *testing.T, store *PostgresStore, prefix string, want int) {
	t.Helper()
	var got int
	if err := store.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM log_events WHERE id LIKE $1`, prefix+"%").Scan(&got); err != nil {
		t.Fatalf("count log prefix %q: %v", prefix, err)
	}
	if got != want {
		t.Fatalf("log prefix %q count=%d, want %d", prefix, got, want)
	}
}
