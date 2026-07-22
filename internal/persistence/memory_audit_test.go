package persistence

import (
	"fmt"
	"testing"

	"github.com/labtether/labtether/internal/audit"
)

func TestMemoryAuditStorePaginationIsNewestFirstAndNonOverlapping(t *testing.T) {
	store := NewMemoryAuditStore()
	for index := range 5 {
		event := audit.NewEvent("test.event")
		event.ID = fmt.Sprintf("event-%d", index+1)
		if err := store.Append(event); err != nil {
			t.Fatalf("append event %d: %v", index+1, err)
		}
	}

	firstPage, err := store.List(2, 0)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	secondPage, err := store.List(2, 2)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}

	if got, want := eventIDs(firstPage), []string{"event-5", "event-4"}; !equalStrings(got, want) {
		t.Fatalf("first page = %v, want %v", got, want)
	}
	if got, want := eventIDs(secondPage), []string{"event-3", "event-2"}; !equalStrings(got, want) {
		t.Fatalf("second page = %v, want %v", got, want)
	}
}

func eventIDs(events []audit.Event) []string {
	ids := make([]string, len(events))
	for index, event := range events {
		ids[index] = event.ID
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
