package audit

import (
	"strings"
	"testing"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	events := s.List(100)
	if len(events) != 0 {
		t.Errorf("new store should have 0 events, got %d", len(events))
	}
}

func TestNewEvent(t *testing.T) {
	ev := NewEvent("auth.login")
	if ev.ID == "" {
		t.Error("event ID should not be empty")
	}
	if !strings.HasPrefix(ev.ID, "audit_") {
		t.Errorf("event ID should start with audit_, got %q", ev.ID)
	}
	if ev.Type != "auth.login" {
		t.Errorf("event type = %q, want %q", ev.Type, "auth.login")
	}
	if ev.Timestamp.IsZero() {
		t.Error("event timestamp should not be zero")
	}
}

func TestStore_AppendAndList(t *testing.T) {
	s := NewStore()

	for i := range 5 {
		ev := NewEvent("test.event")
		ev.ActorID = "user-1"
		ev.Target = "asset-" + string(rune('A'+i))
		s.Append(ev)
	}

	all := s.List(0)
	if len(all) != 5 {
		t.Errorf("List(0) returned %d events, want 5", len(all))
	}

	// List with limit
	limited := s.List(3)
	if len(limited) != 3 {
		t.Errorf("List(3) returned %d events, want 3", len(limited))
	}

	// The limited result should be the last 3 events
	allLast3 := all[2:]
	for i := range 3 {
		if limited[i].ID != allLast3[i].ID {
			t.Errorf("limited[%d].ID = %q, want %q", i, limited[i].ID, allLast3[i].ID)
		}
	}
}

func TestStore_LimitExceedsLength(t *testing.T) {
	s := NewStore()
	s.Append(NewEvent("a"))
	s.Append(NewEvent("b"))

	// Requesting more than available should return all
	result := s.List(100)
	if len(result) != 2 {
		t.Errorf("List(100) with 2 events returned %d, want 2", len(result))
	}
}

func TestStore_Eviction(t *testing.T) {
	s := NewStore()

	// Fill beyond maxAuditEvents to trigger eviction
	total := maxAuditEvents + 100
	for range total {
		s.Append(NewEvent("flood"))
	}

	all := s.List(0)
	if len(all) > maxAuditEvents {
		t.Errorf("store grew beyond maxAuditEvents: got %d", len(all))
	}
	if len(all) == 0 {
		t.Error("store should have events after eviction")
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore()
	done := make(chan struct{})

	go func() {
		for range 200 {
			s.Append(NewEvent("write"))
		}
		close(done)
	}()

	for range 100 {
		_ = s.List(10)
	}
	<-done
}
