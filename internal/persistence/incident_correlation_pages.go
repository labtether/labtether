package persistence

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
)

const CorrelationPageSize = 128

// CorrelationCursor is a fixed key range, not a cross-page database snapshot.
// Adopt Started/UpperID immediately. Commit AfterID only after successful effects
// (or an explicit rejected-row decision). New/changed rows require fresh sweeps.
type CorrelationCursor struct {
	AfterID string
	UpperID string
	Started bool
}

type CorrelationWindow struct{ From, Through time.Time }

// IncidentCorrelationScope is supplied by the scope owner. Empty sets never
// imply global access. Callers must restart a sweep when this scope changes.
type IncidentCorrelationScope struct {
	IncidentID        string
	GroupID           string
	AssetIDs          []string
	OperationAssetIDs []string
	AlertLinks        []incidents.AlertLink
}

type CorrelationIncident struct {
	ID, Status, GroupID, PrimaryAssetID string
	OpenedAt                            time.Time
}
type CorrelationAction struct {
	ID, Type, Target string
	CreatedAt        time.Time
}
type CorrelationUpdate struct {
	ID, PlanID, Target string
	CreatedAt          time.Time
}
type CorrelationAlert struct {
	ID, RuleID, Fingerprint, Severity string
	FirstFiredAt                      time.Time
	FiringScope                       *alerts.FiringScope
	// Rejected exposes only the ID of malformed stored evidence. It cannot
	// produce an event, but allows an explicit skip without poisoning a page.
	Rejected bool
}
type CorrelationAudit struct {
	ID, Type, ActorID, Target string
	Timestamp                 time.Time
}

// Done means genuine EOF, not a time/row budget. A full page needs a further
// read even when its last ID equals UpperID. Errors never advance a cursor.
type IncidentCorrelationPager interface {
	ReadActiveIncidentCorrelationPage(context.Context, time.Time, CorrelationCursor) ([]CorrelationIncident, CorrelationCursor, bool, error)
	ReadIncidentCorrelation(context.Context, string) (CorrelationIncident, bool, error)
}
type ActionCorrelationPager interface {
	ReadActionCorrelationPage(context.Context, CorrelationWindow, IncidentCorrelationScope, CorrelationCursor) ([]CorrelationAction, CorrelationCursor, bool, error)
}
type UpdateCorrelationPager interface {
	ReadUpdateCorrelationPage(context.Context, CorrelationWindow, IncidentCorrelationScope, CorrelationCursor) ([]CorrelationUpdate, CorrelationCursor, bool, error)
}
type AlertCorrelationPager interface {
	ReadAlertCorrelationPage(context.Context, CorrelationWindow, IncidentCorrelationScope, CorrelationCursor) ([]CorrelationAlert, CorrelationCursor, bool, error)
}
type AuditCorrelationPager interface {
	ReadAuditCorrelationPage(context.Context, CorrelationWindow, IncidentCorrelationScope, CorrelationCursor) ([]CorrelationAudit, CorrelationCursor, bool, error)
}
type IncidentEventContextStore interface {
	UpsertIncidentEventContext(context.Context, incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error)
}
