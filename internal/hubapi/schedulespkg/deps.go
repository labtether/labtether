package schedulespkg

import "github.com/labtether/labtether/internal/persistence"

// Deps holds the dependencies for schedule handlers.
type Deps struct {
	ScheduleStore persistence.ScheduleStore
	AuditStore    persistence.AuditStore
}
