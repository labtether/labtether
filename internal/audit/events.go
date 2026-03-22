package audit

import (
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
)

// Event captures one auditable action or state transition.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	ActorID   string         `json:"actor_id,omitempty"`
	Target    string         `json:"target,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	CommandID string         `json:"command_id,omitempty"`
	Decision  string         `json:"decision,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

const maxAuditEvents = 5_000

// Store keeps recent audit events in memory for MVP scaffolding.
type Store struct {
	mu     sync.RWMutex
	events []Event
}

func NewStore() *Store {
	return &Store{events: make([]Event, 0, 256)}
}

func NewEvent(eventType string) Event {
	return Event{
		ID:        idgen.New("audit"),
		Type:      eventType,
		Timestamp: time.Now().UTC(),
	}
}

func (s *Store) Append(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if len(s.events) > maxAuditEvents {
		dropCount := maxAuditEvents / 5
		s.events = append(s.events[:0:0], s.events[dropCount:]...)
	}
}

func (s *Store) List(limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit >= len(s.events) {
		out := make([]Event, len(s.events))
		copy(out, s.events)
		return out
	}

	start := len(s.events) - limit
	out := make([]Event, limit)
	copy(out, s.events[start:])
	return out
}
