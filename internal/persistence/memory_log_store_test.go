package persistence

import (
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/logs"
)

func TestMemoryLogStoreQueryEventsExcludeFields(t *testing.T) {
	store := NewMemoryLogStore()

	if err := store.AppendEvent(logs.Event{
		ID:      "log-1",
		Source:  "agent",
		Level:   "info",
		Message: "hello",
		Fields: map[string]string{
			"component": "collector",
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	events, err := store.QueryEvents(logs.QueryRequest{
		From:          time.Now().UTC().Add(-time.Minute),
		To:            time.Now().UTC().Add(time.Minute),
		Limit:         10,
		ExcludeFields: true,
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Fields != nil {
		t.Fatalf("expected fields to be nil when ExcludeFields=true")
	}
}

func TestMemoryLogStoreAppendEvents(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	err := store.AppendEvents([]logs.Event{
		{ID: "batch-1", Source: "agent", Level: "info", Message: "one", Timestamp: now.Add(-2 * time.Second)},
		{ID: "batch-2", Source: "agent", Level: "warning", Message: "two", Timestamp: now.Add(-time.Second)},
	})
	if err != nil {
		t.Fatalf("append events failed: %v", err)
	}

	events, err := store.QueryEvents(logs.QueryRequest{
		From:  now.Add(-time.Minute),
		To:    now.Add(time.Minute),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "batch-2" || events[1].ID != "batch-1" {
		t.Fatalf("expected newest-first batch events, got %q then %q", events[0].ID, events[1].ID)
	}
}

func TestMemoryLogStoreQueryEventsFieldKeysProjection(t *testing.T) {
	store := NewMemoryLogStore()

	if err := store.AppendEvent(logs.Event{
		ID:      "log-projected-1",
		Source:  "agent",
		Level:   "warn",
		Message: "projection check",
		Fields: map[string]string{
			"group_id":  "group-a",
			"component": "collector",
			"ignored":   "value",
		},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	events, err := store.QueryEvents(logs.QueryRequest{
		From:      time.Now().UTC().Add(-time.Minute),
		To:        time.Now().UTC().Add(time.Minute),
		Limit:     10,
		FieldKeys: []string{"group_id", "group_id", " component "},
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if len(events[0].Fields) != 2 {
		t.Fatalf("expected exactly 2 projected fields, got %d", len(events[0].Fields))
	}
	if events[0].Fields["group_id"] != "group-a" {
		t.Fatalf("expected projected group_id to be retained")
	}
	if events[0].Fields["component"] != "collector" {
		t.Fatalf("expected projected component to be retained")
	}
	if _, ok := events[0].Fields["ignored"]; ok {
		t.Fatalf("expected ignored field to be removed by projection")
	}
}

func TestMemoryLogStoreQueryEventsGroupFilterWithAssetFallback(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	appendEvent := func(event logs.Event) {
		if err := store.AppendEvent(event); err != nil {
			t.Fatalf("append event failed: %v", err)
		}
	}

	appendEvent(logs.Event{
		ID:        "group-hit-via-asset",
		AssetID:   "asset-group-a",
		Source:    "agent",
		Level:     "info",
		Message:   "asset-scoped hit",
		Timestamp: now.Add(-10 * time.Second),
	})
	appendEvent(logs.Event{
		ID:      "group-hit-via-field",
		Source:  "agent",
		Level:   "warn",
		Message: "field-scoped hit",
		Fields: map[string]string{
			"group_id": "group-a",
		},
		Timestamp: now.Add(-5 * time.Second),
	})
	appendEvent(logs.Event{
		ID:      "group-miss",
		AssetID: "asset-group-b",
		Source:  "agent",
		Level:   "warn",
		Message: "other group",
		Fields: map[string]string{
			"group_id": "group-b",
		},
		Timestamp: now.Add(-2 * time.Second),
	})

	events, err := store.QueryEvents(logs.QueryRequest{
		From:          now.Add(-time.Minute),
		To:            now.Add(time.Minute),
		Limit:         20,
		GroupID:       "group-a",
		GroupAssetIDs: []string{"asset-group-a", "asset-group-a", "  "},
		FieldKeys:     []string{"group_id"},
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 group-filtered events, got %d", len(events))
	}
	if events[0].ID != "group-hit-via-field" {
		t.Fatalf("expected newest event first, got %q", events[0].ID)
	}
	if events[1].ID != "group-hit-via-asset" {
		t.Fatalf("expected asset fallback event second, got %q", events[1].ID)
	}
}

func TestMemoryLogStoreQueryEventsFiltersByGroupAssetIDsWithoutGroupID(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	_ = store.AppendEvent(logs.Event{
		ID:        "asset-only-hit",
		AssetID:   "asset-a",
		Source:    "agent",
		Level:     "info",
		Message:   "kept",
		Timestamp: now,
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "asset-only-miss",
		AssetID:   "asset-b",
		Source:    "agent",
		Level:     "info",
		Message:   "filtered",
		Timestamp: now,
	})

	events, err := store.QueryEvents(logs.QueryRequest{
		From:          now.Add(-time.Minute),
		To:            now.Add(time.Minute),
		Limit:         10,
		GroupAssetIDs: []string{"asset-a"},
	})
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
	if events[0].ID != "asset-only-hit" {
		t.Fatalf("expected asset-only-hit, got %q", events[0].ID)
	}
}

func TestMemoryLogStoreQueryDeadLetterEventsProjectsAndFilters(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	_ = store.AppendEvent(logs.Event{
		ID:      "dead-letter-hit",
		Source:  "dead_letter",
		Level:   "error",
		Message: "decode failed",
		Fields: map[string]string{
			"event_id":    "dlq-1",
			"component":   "worker.command.decode",
			"subject":     "terminal.commands.requested",
			"deliveries":  "4",
			"error":       "bad payload",
			"payload_b64": "YQ==",
		},
		Timestamp: now,
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "dead-letter-wrong-level",
		Source:    "dead_letter",
		Level:     "info",
		Message:   "ignore",
		Timestamp: now,
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "non-dead-letter",
		Source:    "labtether",
		Level:     "error",
		Message:   "ignore",
		Timestamp: now,
	})

	events, err := store.QueryDeadLetterEvents(now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("query dead-letter events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one projected dead-letter event, got %d", len(events))
	}

	event := events[0]
	if event.ID != "dlq-1" {
		t.Fatalf("expected event id dlq-1, got %q", event.ID)
	}
	if event.Component != "worker.command.decode" {
		t.Fatalf("unexpected component: %q", event.Component)
	}
	if event.Subject != "terminal.commands.requested" {
		t.Fatalf("unexpected subject: %q", event.Subject)
	}
	if event.Deliveries != 4 {
		t.Fatalf("expected deliveries=4, got %d", event.Deliveries)
	}
	if event.Error != "bad payload" {
		t.Fatalf("unexpected error message: %q", event.Error)
	}
	if event.PayloadB64 != "YQ==" {
		t.Fatalf("unexpected payload_b64: %q", event.PayloadB64)
	}
}

func TestMemoryLogStoreCountDeadLetterEvents(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	_ = store.AppendEvent(logs.Event{
		ID:        "dead-letter-count-1",
		Source:    "dead_letter",
		Level:     "error",
		Message:   "first",
		Timestamp: now.Add(-30 * time.Second),
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "dead-letter-count-2",
		Source:    "dead_letter",
		Level:     "error",
		Message:   "second",
		Timestamp: now.Add(-10 * time.Second),
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "dead-letter-count-ignore",
		Source:    "dead_letter",
		Level:     "info",
		Message:   "ignore",
		Timestamp: now.Add(-10 * time.Second),
	})

	total, err := store.CountDeadLetterEvents(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("count dead-letter events failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected dead-letter count=2, got %d", total)
	}
}

func TestMemoryLogStoreEviction(t *testing.T) {
	store := NewMemoryLogStore()
	for i := 0; i < 12000; i++ {
		_ = store.AppendEvent(logs.Event{
			Message:   fmt.Sprintf("event %d", i),
			Timestamp: time.Now().UTC(),
		})
	}
	store.mu.RLock()
	count := len(store.events)
	store.mu.RUnlock()
	if count > maxMemoryLogEvents {
		t.Errorf("expected at most %d events after eviction, got %d", maxMemoryLogEvents, count)
	}
	if count == 0 {
		t.Fatal("expected some events after eviction")
	}
	// Most recent events should be preserved
	events, err := store.QueryEvents(logs.QueryRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].Message != "event 11999" {
		t.Errorf("expected most recent event preserved, got %q", events[0].Message)
	}
}

func TestMemoryLogStoreListSourcesSinceAndWatermark(t *testing.T) {
	store := NewMemoryLogStore()
	now := time.Now().UTC()

	_ = store.AppendEvent(logs.Event{
		ID:        "source-since-old",
		Source:    "old",
		Level:     "info",
		Message:   "old",
		Timestamp: now.Add(-2 * time.Hour),
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "source-since-new-1",
		Source:    "new",
		Level:     "info",
		Message:   "new 1",
		Timestamp: now.Add(-30 * time.Second),
	})
	_ = store.AppendEvent(logs.Event{
		ID:        "source-since-new-2",
		Source:    "new",
		Level:     "info",
		Message:   "new 2",
		Timestamp: now.Add(-10 * time.Second),
	})

	sources, err := store.ListSourcesSince(10, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("list sources since failed: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected one source in window, got %d", len(sources))
	}
	if sources[0].Source != "new" {
		t.Fatalf("expected source=new, got %q", sources[0].Source)
	}
	if sources[0].Count != 2 {
		t.Fatalf("expected source count=2, got %d", sources[0].Count)
	}

	watermark, err := store.LogEventsWatermark()
	if err != nil {
		t.Fatalf("log watermark failed: %v", err)
	}
	if watermark.Before(now.Add(-20 * time.Second)) {
		t.Fatalf("expected watermark near latest event, got %s", watermark)
	}
}
