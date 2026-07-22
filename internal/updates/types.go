package updates

import (
	"fmt"
	"strings"
	"time"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"

	ScopeOSPackages  = "os_packages"
	ScopeDockerImage = "docker_images"
	ScopeApps        = "apps"
	ScopeFirmware    = "firmware"
)

// DefaultScopes contains only scopes with an end-to-end executor. Keep roadmap
// scope constants above so persisted legacy plans can be identified and
// rejected honestly, but never advertise an unimplemented scope as a default.
var DefaultScopes = []string{ScopeOSPackages}

// NormalizeTargets trims and deduplicates update targets. Every currently
// executable update scope is asset-bound, so accepting an empty target list
// would only create a plan that later attempts the synthetic "global" target
// and fails at execution time.
func NormalizeTargets(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one update target is required")
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		target := strings.TrimSpace(value)
		if target == "" {
			return nil, fmt.Errorf("update target must not be empty")
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	return result, nil
}

// NormalizeExecutableScopes trims, normalizes, and deduplicates update scopes.
// It rejects known roadmap scopes and unknown values until an end-to-end
// executor exists for them.
func NormalizeExecutableScopes(values []string) ([]string, error) {
	if len(values) == 0 {
		return append([]string(nil), DefaultScopes...), nil
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		scope := strings.ToLower(strings.TrimSpace(value))
		if scope == "" {
			return nil, fmt.Errorf("update scope must not be empty")
		}
		if scope != ScopeOSPackages {
			return nil, fmt.Errorf("update scope %q is not supported", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

type CreatePlanRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Targets       []string `json:"targets"`
	Scopes        []string `json:"scopes,omitempty"`
	DefaultDryRun *bool    `json:"default_dry_run,omitempty"`
}

type Plan struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Targets       []string  `json:"targets"`
	Scopes        []string  `json:"scopes"`
	DefaultDryRun bool      `json:"default_dry_run"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ExecutePlanRequest struct {
	ActorID string `json:"actor_id"`
	DryRun  *bool  `json:"dry_run,omitempty"`
}

type RunResultEntry struct {
	Target  string `json:"target"`
	Scope   string `json:"scope"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type Run struct {
	ID          string           `json:"id"`
	PlanID      string           `json:"plan_id"`
	PlanName    string           `json:"plan_name"`
	ActorID     string           `json:"actor_id"`
	DryRun      bool             `json:"dry_run"`
	Status      string           `json:"status"`
	Summary     string           `json:"summary,omitempty"`
	Error       string           `json:"error,omitempty"`
	Results     []RunResultEntry `json:"results,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

type Job struct {
	JobID       string    `json:"job_id"`
	RunID       string    `json:"run_id"`
	ActorID     string    `json:"actor_id"`
	DryRun      bool      `json:"dry_run"`
	Plan        Plan      `json:"plan"`
	RequestedAt time.Time `json:"requested_at"`
}

type Result struct {
	JobID       string           `json:"job_id"`
	RunID       string           `json:"run_id"`
	Status      string           `json:"status"`
	Summary     string           `json:"summary,omitempty"`
	Error       string           `json:"error,omitempty"`
	Results     []RunResultEntry `json:"results,omitempty"`
	CompletedAt time.Time        `json:"completed_at"`
}

func NormalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StatusQueued:
		return StatusQueued
	case StatusRunning:
		return StatusRunning
	case StatusSucceeded:
		return StatusSucceeded
	case StatusFailed:
		return StatusFailed
	default:
		return ""
	}
}
