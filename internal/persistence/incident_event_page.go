package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

const IncidentEventPageDefault = 50
const IncidentEventPageMaximum = 500

var ErrInvalidIncidentEventCursor = errors.New("invalid incident timeline cursor")

// The ordering key is event time plus the unique ID. ThroughCreatedAt excludes
// later inserts, including backfilled events. Updates and retention can still
// change a later page; this is a resumable browse, not a database snapshot.
type IncidentEventPageCursor struct {
	IncidentID       string    `json:"incident_id"`
	ThroughCreatedAt time.Time `json:"through_created_at"`
	BeforeOccurredAt time.Time `json:"before_occurred_at"`
	BeforeID         string    `json:"before_id"`
}

type IncidentEventPage struct {
	Events []incidents.IncidentEvent
	Next   *IncidentEventPageCursor
}

type IncidentEventPageStore interface {
	ListIncidentEventPage(context.Context, string, IncidentEventPageCursor, int) (IncidentEventPage, error)
}

func ValidateIncidentEventCursor(incidentID string, cursor IncidentEventPageCursor) error {
	if !correlationExactID(incidentID) {
		return ErrInvalidIncidentEventCursor
	}
	if cursor == (IncidentEventPageCursor{}) {
		return nil
	}
	if cursor.IncidentID != incidentID || cursor.ThroughCreatedAt.IsZero() || cursor.BeforeOccurredAt.IsZero() || !correlationExactID(cursor.BeforeID) {
		return ErrInvalidIncidentEventCursor
	}
	return nil
}
