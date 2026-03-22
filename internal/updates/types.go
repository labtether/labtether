package updates

import (
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

var DefaultScopes = []string{ScopeOSPackages, ScopeDockerImage}

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
