package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/alerts"
)

// AlertFiringSnapshotStore reads all matching firing rows from one snapshot.
// A nil rule list means all rules. The visitor must not reenter or mutate the
// store: the Memory owner holds its read lock until every visit is finished.
type AlertFiringSnapshotStore interface {
	ReadFiringAlertInstances(context.Context, []string, func(alerts.AlertInstance) error) error
}
