package jobqueue

import (
	"fmt"
	"time"
)

// JobKind identifies the type of work a job performs.
type JobKind string

const (
	KindTerminalCommand JobKind = "terminal_command"
	KindActionRun       JobKind = "action_run"
	KindUpdateRun       JobKind = "update_run"
)

// JobStatus tracks lifecycle state of a job.
type JobStatus string

const (
	StatusQueued       JobStatus = "queued"
	StatusProcessing   JobStatus = "processing"
	StatusCompleted    JobStatus = "completed"
	StatusFailed       JobStatus = "failed"
	StatusDeadLettered JobStatus = "dead_lettered"
)

// Job represents a unit of work in the Postgres job queue.
type Job struct {
	ID          string     `json:"id"`
	Kind        JobKind    `json:"kind"`
	Status      JobStatus  `json:"status"`
	Payload     []byte     `json:"payload"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LockedAt    *time.Time `json:"locked_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ValidKinds is the set of valid job kinds.
var ValidKinds = map[JobKind]bool{
	KindTerminalCommand: true,
	KindActionRun:       true,
	KindUpdateRun:       true,
}

// ValidStatuses is the set of valid job statuses.
var ValidStatuses = map[JobStatus]bool{
	StatusQueued:       true,
	StatusProcessing:   true,
	StatusCompleted:    true,
	StatusFailed:       true,
	StatusDeadLettered: true,
}

// ValidateKind returns an error if kind is not a known job kind.
func ValidateKind(kind JobKind) error {
	if !ValidKinds[kind] {
		return fmt.Errorf("invalid job kind: %q", kind)
	}
	return nil
}

// ValidateStatus returns an error if status is not a known job status.
func ValidateStatus(status JobStatus) error {
	if !ValidStatuses[status] {
		return fmt.Errorf("invalid job status: %q", status)
	}
	return nil
}

// HandlerFunc processes a claimed job. Return nil on success, error to fail/retry.
type HandlerFunc func(ctx interface{ Deadline() (time.Time, bool) }, job *Job) error
