package actionspkg

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/audit"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
)

// ExecResult holds the result of executing a command on a single asset.
// It mirrors the fields of v2ExecResult in cmd/labtether without creating
// a circular import.
type ExecResult struct {
	AssetID    string `json:"asset_id"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Deps holds the dependencies for saved-action handlers and action-execution handlers.
type Deps struct {
	// SavedActionStore is the store for API v2 saved actions.
	SavedActionStore persistence.SavedActionStore
	// AuditStore is used for saved-action and action-execution audit events.
	AuditStore persistence.AuditStore
	// ExecOnAsset is injected by the bridge to execute a command on a single asset.
	// It is set to s.v2ExecOnAsset as a method value, which avoids importing
	// cmd/labtether (which would be circular).
	ExecOnAsset func(r *http.Request, assetID, command string, timeoutSec int) ExecResult

	// Stores required by the v1 action-execution handlers.
	ActionStore persistence.ActionStore
	AssetStore  persistence.AssetStore
	GroupStore  persistence.GroupStore
	JobQueue    *jobqueue.Queue
	LogStore    persistence.LogStore

	// EvaluateGuardrails checks maintenance-window constraints for a group.
	// Injected as s.ensureGroupFeaturesDeps().EvaluateGuardrails.
	EvaluateGuardrails func(groupID string, now time.Time) (groupfeatures.GroupMaintenanceGuardrails, error)

	// EnforceRateLimit applies a per-client rate limit for the given bucket.
	// Returns true when the request is within the limit; false (and writes 429)
	// when it is exceeded.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// ResolveGroupIDForAction resolves the group ID for an action execution request.
	// Injected as s.ensureGroupFeaturesDeps().ResolveGroupIDForAction.
	ResolveGroupIDForAction func(req actions.ExecuteRequest) (string, error)

	// GetPolicyConfig returns the current policy evaluator configuration.
	GetPolicyConfig func() policy.EvaluatorConfig

	// AppendAuditEventBestEffort records an audit event, logging on store error.
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)
}
