package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/alerts"
)

// AlertInstanceFiringStore persists the condition's scope in the same atomic
// transition as the first firing timestamp. It never upgrades unknown history.
type AlertInstanceFiringStore interface {
	FireAlertInstance(context.Context, string, alerts.FiringScope) (alerts.AlertInstance, error)
}
