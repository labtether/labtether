package persistence

import (
	"errors"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/logs"
)

func TestMemoryLogStoreEnforcesEnvelopeBoundsAndClonesFields(t *testing.T) {
	store := NewMemoryLogStore()
	fields := map[string]string{"component": "agent"}
	if err := store.AppendEvent(logs.Event{
		Source:  "agent",
		Level:   "WARNING",
		Message: strings.Repeat("x", logs.MaxEventMessageBytes),
		Fields:  fields,
	}); err != nil {
		t.Fatalf("append exact-limit event: %v", err)
	}
	fields["component"] = "mutated"
	if got := store.events[0].Fields["component"]; got != "agent" {
		t.Fatalf("stored fields alias caller map: got %q", got)
	}
	if store.events[0].Level != "warning" || store.events[0].ID == "" || store.events[0].Timestamp.IsZero() {
		t.Fatalf("memory normalization diverged from PostgreSQL: %+v", store.events[0])
	}

	before := len(store.events)
	if err := store.AppendEvent(logs.Event{Message: strings.Repeat("x", logs.MaxEventMessageBytes+1)}); !errors.Is(err, logs.ErrEventLimitExceeded) {
		t.Fatalf("oversized event error = %v", err)
	}
	if len(store.events) != before {
		t.Fatalf("oversized event mutated store: before=%d after=%d", before, len(store.events))
	}
	if err := store.AppendEvent(logs.Event{Message: string([]byte{0xff})}); !errors.Is(err, logs.ErrInvalidEventText) {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
	if err := store.AppendEvent(logs.Event{Fields: map[string]string{"key": "bad\x00value"}}); !errors.Is(err, logs.ErrInvalidEventText) {
		t.Fatalf("NUL field error = %v", err)
	}
}

func TestMemoryLogStoreRejectsInvalidBatchWithoutPartialWrite(t *testing.T) {
	store := NewMemoryLogStore()
	if err := store.AppendEvents([]logs.Event{
		{ID: "valid-before-invalid", Message: "valid"},
		{ID: "invalid", Message: "bad\x00message"},
	}); !errors.Is(err, logs.ErrInvalidEventText) {
		t.Fatalf("invalid batch error = %v", err)
	}
	if len(store.events) != 0 {
		t.Fatalf("invalid batch partially wrote %d events", len(store.events))
	}

	exact := make([]logs.Event, logs.MaxEventsPerBatch)
	if err := store.AppendEvents(exact); err != nil {
		t.Fatalf("exact batch count rejected: %v", err)
	}
	if len(store.events) != logs.MaxEventsPerBatch {
		t.Fatalf("exact batch count stored=%d", len(store.events))
	}
	if err := store.AppendEvents(make([]logs.Event, logs.MaxEventsPerBatch+1)); !errors.Is(err, logs.ErrEventBatchLimitExceeded) {
		t.Fatalf("over batch count error = %v", err)
	}
	if len(store.events) != logs.MaxEventsPerBatch {
		t.Fatalf("oversized batch mutated store: %d", len(store.events))
	}
}

func TestLogBatchCapStaysBelowPostgresParameterCeiling(t *testing.T) {
	const (
		logColumns                = 7
		postgresMaxBindParameters = 65535
	)
	if got := logs.MaxEventsPerBatch * logColumns; got >= postgresMaxBindParameters {
		t.Fatalf("log batch uses %d parameters, must stay below %d", got, postgresMaxBindParameters)
	}
}
