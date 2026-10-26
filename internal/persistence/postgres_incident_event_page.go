package persistence

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

func (s *PostgresStore) ListIncidentEventPage(ctx context.Context, incidentID string, cursor IncidentEventPageCursor, limit int) (IncidentEventPage, error) {
	if err := ValidateIncidentEventCursor(incidentID, cursor); err != nil {
		return IncidentEventPage{}, err
	}
	ctx, cancel, err := correlationContext(ctx)
	if err != nil {
		return IncidentEventPage{}, err
	}
	defer cancel()
	if limit <= 0 {
		limit = IncidentEventPageDefault
	}
	if limit > IncidentEventPageMaximum {
		limit = IncidentEventPageMaximum
	}
	through := cursor.ThroughCreatedAt
	var before any
	if through.IsZero() {
		through = time.Now().UTC()
	} else {
		before = cursor.BeforeOccurredAt.UTC()
	}
	// Legacy NULL creation times are unknown, not absent history. Normal writes
	// supply a timestamp; these legacy rows cannot participate in that cutoff.
	// Fetch one extra row to distinguish a full final page from more history.
	rows, err := s.pool.Query(ctx, "SELECT "+incidentEventColumns+` FROM incident_events
 WHERE incident_id=$1 AND (created_at <= $2 OR created_at IS NULL)
 AND ($3::timestamptz IS NULL OR (occurred_at, id COLLATE "C") < ($3, $4 COLLATE "C"))
 ORDER BY occurred_at DESC, id COLLATE "C" DESC LIMIT $5`, incidentID, through, before, cursor.BeforeID, limit+1)
	if err != nil {
		return IncidentEventPage{}, err
	}
	defer rows.Close()
	page := IncidentEventPage{Events: make([]incidents.IncidentEvent, 0, limit)}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return IncidentEventPage{}, err
		}
		event, err := scanIncidentEvent(rows)
		if err != nil {
			return IncidentEventPage{}, err
		}
		if len(page.Events) == limit {
			last := page.Events[len(page.Events)-1]
			page.Next = &IncidentEventPageCursor{IncidentID: incidentID, ThroughCreatedAt: through, BeforeOccurredAt: last.OccurredAt, BeforeID: last.ID}
			break
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return IncidentEventPage{}, err
	}
	return page, nil
}
