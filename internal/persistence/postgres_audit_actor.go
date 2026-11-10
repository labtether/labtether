package persistence

import (
	"context"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/audit"
)

// AuditActorStore lists one actor's audit rows, newest first. Paging happens
// in the store, so a reader limited to their own rows gets full pages.
type AuditActorStore interface {
	ListByActor(actorID string, limit, offset int) ([]audit.Event, error)
}

// ListByActor returns the audit rows written for actorID, newest first. An
// empty actor matches nothing.
func (s *PostgresStore) ListByActor(actorID string, limit, offset int) ([]audit.Event, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return []audit.Event{}, nil
	}
	return s.listAuditEvents(actorID, limit, offset)
}

// listAuditEvents reads shown audit rows, newest first; a non-empty actorID
// limits them to that actor.
func (s *PostgresStore) listAuditEvents(actorID string, limit, offset int) ([]audit.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp
		 FROM audit_events WHERE ` + auditListShown
	args := []any{limit, offset}
	if actorID != "" {
		query += ` AND actor_id = $3`
		args = append(args, actorID)
	}
	query += ` ORDER BY timestamp DESC LIMIT $1 OFFSET $2`

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (audit.Event, error) {
		event := audit.Event{}
		var details []byte
		if err := row.Scan(&event.ID, &event.Type, &event.ActorID, &event.Target, &event.SessionID,
			&event.CommandID, &event.Decision, &event.Reason, &details, &event.Timestamp); err != nil {
			return audit.Event{}, err
		}
		unmarshalAuditDetails(&event, details)
		return event, nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out, nil
}

// ListByActor returns the audit rows written for actorID, newest first.
func (m *MemoryAuditStore) ListByActor(actorID string, limit, offset int) ([]audit.Event, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || offset < 0 {
		return []audit.Event{}, nil
	}
	own := make([]audit.Event, 0)
	for _, event := range m.store.List(0) {
		if strings.TrimSpace(event.ActorID) == actorID {
			own = append(own, event)
		}
	}
	if offset >= len(own) {
		return []audit.Event{}, nil
	}
	own = own[offset:]
	if limit > 0 && len(own) > limit {
		own = own[:limit]
	}
	return own, nil
}
