package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

// --- scan helpers ---

type incidentEventScanner interface {
	Scan(dest ...any) error
}

func scanIncidentEvent(row incidentEventScanner) (incidents.IncidentEvent, error) {
	e := incidents.IncidentEvent{}
	var severity *string
	var metadata []byte
	if err := row.Scan(
		&e.ID,
		&e.IncidentID,
		&e.EventType,
		&e.SourceRef,
		&e.Summary,
		&severity,
		&metadata,
		&e.OccurredAt,
		&e.CreatedAt,
	); err != nil {
		return incidents.IncidentEvent{}, err
	}
	if severity != nil {
		e.Severity = *severity
	}
	e.Metadata = unmarshalAnyMap(metadata)
	e.OccurredAt = e.OccurredAt.UTC()
	e.CreatedAt = e.CreatedAt.UTC()
	return e, nil
}

// --- columns ---

const incidentEventColumns = `id, incident_id, event_type, source_ref, summary, severity, metadata, occurred_at, created_at`

// --- store methods ---

func (s *PostgresStore) UpsertIncidentEvent(req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error) {
	now := time.Now().UTC()

	occurredAt := req.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = now
	}

	eventType := incidents.NormalizeEventType(req.EventType)
	if eventType == "" {
		eventType = req.EventType // allow pass-through for extensibility
	}

	metadataPayload, err := marshalAnyMap(req.Metadata)
	if err != nil {
		return incidents.IncidentEvent{}, err
	}

	return scanIncidentEvent(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO incident_events (%s)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)
		 ON CONFLICT (incident_id, source_ref) DO UPDATE
		 SET summary = EXCLUDED.summary,
		     severity = EXCLUDED.severity,
		     metadata = EXCLUDED.metadata,
		     occurred_at = EXCLUDED.occurred_at
		 RETURNING %s`, incidentEventColumns, incidentEventColumns),
		idgen.New("ievt"),
		strings.TrimSpace(req.IncidentID),
		eventType,
		strings.TrimSpace(req.SourceRef),
		strings.TrimSpace(req.Summary),
		nullIfBlank(req.Severity),
		metadataPayload,
		occurredAt.UTC(),
		now,
	))
}

func (s *PostgresStore) ListIncidentEvents(incidentID string, limit int) ([]incidents.IncidentEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM incident_events
		 WHERE incident_id = $1 ORDER BY occurred_at DESC LIMIT $2`, incidentEventColumns),
		strings.TrimSpace(incidentID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]incidents.IncidentEvent, 0)
	for rows.Next() {
		e, scanErr := scanIncidentEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
