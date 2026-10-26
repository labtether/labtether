package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/incidents"
)

// AlertAutoIncidentContextStore creates the incident and its trigger link
// together. The bool is true only for a newly committed pair. An existing
// automatic incident returns a zero incident and false, without another write.
type AlertAutoIncidentContextStore interface {
	CreateAutoIncidentForAlertInstanceContext(context.Context, string, incidents.CreateIncidentRequest) (incidents.Incident, bool, error)
}
