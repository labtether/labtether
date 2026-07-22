package schedules

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	// MaxScheduledTasksGlobal is the durable upper bound on recurring command
	// definitions in one hub. It bounds scheduler/database work even when many
	// authenticated principals create definitions concurrently.
	MaxScheduledTasksGlobal = 4096
	// MaxScheduledTasksPerPrincipal prevents one API key or user from consuming
	// the entire global definition budget.
	MaxScheduledTasksPerPrincipal = 512
	// MaxScheduledTaskPageSize bounds one operator API/MCP response.
	MaxScheduledTaskPageSize = 100
)

// ErrScheduledTaskCapacityExceeded is returned atomically by persistence when
// either the global or creating-principal definition budget is exhausted.
var ErrScheduledTaskCapacityExceeded = errors.New("scheduled task capacity exceeded")

// ScheduledTask represents a recurring command definition executed by the hub.
type ScheduledTask struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	CronExpr      string     `json:"cron_expr"`
	Command       string     `json:"command"`
	Targets       []string   `json:"targets"`
	GroupID       string     `json:"group_id,omitempty"`
	Enabled       bool       `json:"enabled"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LastRunJobID  string     `json:"last_run_job_id,omitempty"`
}

// CreateRequest is the API request body for creating a scheduled task.
type CreateRequest struct {
	Name     string   `json:"name"`
	CronExpr string   `json:"cron_expr"`
	Command  string   `json:"command"`
	Targets  []string `json:"targets,omitempty"`
	GroupID  string   `json:"group_id,omitempty"`
	Enabled  *bool    `json:"enabled,omitempty"`
}

// NextRunUpdate distinguishes leaving next_run_at untouched from explicitly
// setting it to NULL. Schedule definition edits that do not change cadence or
// enabled state must not write a next-run value read before a concurrent due
// occurrence was claimed.
type NextRunUpdate struct {
	Set   bool
	Value *time.Time
}

// ExecutionJob is the immutable snapshot placed in the durable job queue for
// one claimed schedule occurrence. The queue job ID is deterministic for the
// (schedule, due time) pair so replaying a claim cannot create a second job.
type ExecutionJob struct {
	JobID        string    `json:"job_id"`
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	ScheduledFor time.Time `json:"scheduled_for"`
	Command      string    `json:"command"`
	Targets      []string  `json:"targets"`
	GroupID      string    `json:"group_id,omitempty"`
	ActorID      string    `json:"actor_id"`
}

// ExecutionClaim advances a due task and inserts its ExecutionJob atomically.
type ExecutionClaim struct {
	ScheduleID  string
	DueAt       time.Time
	ClaimedAt   time.Time
	NextRunAt   time.Time
	MaxAttempts int
	Job         ExecutionJob
}

// ExecutionJobID returns a stable, opaque identifier for an occurrence.
func ExecutionJobID(scheduleID string, dueAt time.Time) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(scheduleID) + "\x00" + dueAt.UTC().Format(time.RFC3339Nano)))
	return "schedrun_" + hex.EncodeToString(sum[:20])
}
