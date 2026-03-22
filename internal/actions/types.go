package actions

import (
	"strings"
	"time"
)

const (
	RunTypeCommand         = "command"
	RunTypeConnectorAction = "connector_action"

	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

type ExecuteRequest struct {
	ActorID     string            `json:"actor_id"`
	Type        string            `json:"type"`
	Target      string            `json:"target,omitempty"`
	Command     string            `json:"command,omitempty"`
	ConnectorID string            `json:"connector_id,omitempty"`
	ActionID    string            `json:"action_id,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	DryRun      bool              `json:"dry_run,omitempty"`
}

type Run struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	ActorID     string            `json:"actor_id"`
	Target      string            `json:"target,omitempty"`
	Command     string            `json:"command,omitempty"`
	ConnectorID string            `json:"connector_id,omitempty"`
	ActionID    string            `json:"action_id,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	DryRun      bool              `json:"dry_run"`
	Status      string            `json:"status"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Steps       []RunStep         `json:"steps,omitempty"`
}

type RunStep struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Job struct {
	JobID       string            `json:"job_id"`
	RunID       string            `json:"run_id"`
	Type        string            `json:"type"`
	ActorID     string            `json:"actor_id"`
	Target      string            `json:"target,omitempty"`
	Command     string            `json:"command,omitempty"`
	ConnectorID string            `json:"connector_id,omitempty"`
	ActionID    string            `json:"action_id,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	DryRun      bool              `json:"dry_run,omitempty"`
	RequestedAt time.Time         `json:"requested_at"`
}

type StepResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Result struct {
	JobID       string       `json:"job_id"`
	RunID       string       `json:"run_id"`
	Status      string       `json:"status"`
	Output      string       `json:"output,omitempty"`
	Error       string       `json:"error,omitempty"`
	Steps       []StepResult `json:"steps,omitempty"`
	CompletedAt time.Time    `json:"completed_at"`
}

func NormalizeRunType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RunTypeCommand:
		return RunTypeCommand
	case RunTypeConnectorAction:
		return RunTypeConnectorAction
	default:
		return ""
	}
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
