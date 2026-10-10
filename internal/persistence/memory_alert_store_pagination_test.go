package persistence

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
)

func TestMemoryAlertRulePagesUseIDToBreakTimestampTies(t *testing.T) {
	store := NewMemoryAlertStore()
	updatedAt := time.Now().UTC()
	for _, id := range []string{"a", "c", "b"} {
		store.rules[id] = alerts.Rule{ID: id, UpdatedAt: updatedAt}
	}
	for offset, want := range []string{"c", "b", "a"} {
		page, err := store.ListAlertRules(AlertRuleFilter{Limit: 1, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].ID != want {
			t.Fatalf("offset %d: got %#v, want %q", offset, page, want)
		}
	}
}

func TestMemoryAlertInstancePagesUseIDToBreakTimestampTies(t *testing.T) {
	store := NewMemoryAlertInstanceStore()
	updatedAt := time.Now().UTC()
	for _, id := range []string{"a", "c", "b"} {
		store.instances[id] = alerts.AlertInstance{ID: id, UpdatedAt: updatedAt}
	}
	for offset, want := range []string{"c", "b", "a"} {
		page, err := store.ListAlertInstances(AlertInstanceFilter{Limit: 1, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].ID != want {
			t.Fatalf("offset %d: got %#v, want %q", offset, page, want)
		}
	}
}

func TestMemoryIncidentPagesUseIDToBreakTimestampTies(t *testing.T) {
	store := NewMemoryIncidentStore()
	updatedAt := time.Now().UTC()
	for _, id := range []string{"a", "c", "b"} {
		store.incidents[id] = incidents.Incident{ID: id, UpdatedAt: updatedAt}
	}
	for offset, want := range []string{"c", "b", "a"} {
		page, err := store.ListIncidents(IncidentFilter{Limit: 1, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].ID != want {
			t.Fatalf("offset %d: got %#v, want %q", offset, page, want)
		}
	}
}
