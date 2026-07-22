package schedulespkg

import (
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
)

// Deps holds the dependencies for schedule handlers.
type Deps struct {
	ScheduleStore  persistence.ScheduleStore
	AuditStore     persistence.AuditStore
	AssetStore     persistence.AssetStore
	GroupStore     persistence.GroupStore
	AuthStore      persistence.AuthStore
	APIKeyStore    persistence.APIKeyStore
	ExecutionStore persistence.ScheduleExecutionStore
	JobQueue       *jobqueue.Queue
}
