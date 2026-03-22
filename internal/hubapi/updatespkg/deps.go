package updatespkg

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/audit"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
)

// Deps holds the dependencies for update-plan and update-run handlers.
type Deps struct {
	// Core stores.
	UpdateStore persistence.UpdateStore
	AssetStore  persistence.AssetStore
	GroupStore  persistence.GroupStore
	JobQueue    *jobqueue.Queue
	LogStore    persistence.LogStore

	// EvaluateGuardrails checks maintenance-window constraints for a group.
	// Injected as s.ensureGroupFeaturesDeps().EvaluateGuardrails.
	EvaluateGuardrails func(groupID string, now time.Time) (groupfeatures.GroupMaintenanceGuardrails, error)

	// ResolveGroupIDsForTargets maps a slice of target IDs to a set of group IDs.
	// Injected as s.ensureGroupFeaturesDeps().ResolveGroupIDsForTargets.
	ResolveGroupIDsForTargets func(targets []string) (map[string]struct{}, error)

	// EnforceRateLimit applies a per-client rate limit for the given bucket.
	// Returns true when the request is within the limit; false (and writes 429)
	// when it is exceeded.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// AppendAuditEventBestEffort records an audit event, logging on store error.
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)
}
