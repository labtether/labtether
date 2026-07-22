package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	maxScheduledTaskErrorLength          = 1024
	scheduledTaskCapacityAdvisoryLockKey = int64(0x4c5453434150) // "LTSCAP"
)

func (s *PostgresStore) CreateScheduledTask(ctx context.Context, task schedules.ScheduledTask) error {
	targetsJSON, err := json.Marshal(task.Targets)
	if err != nil {
		return fmt.Errorf("marshal targets: %w", err)
	}
	if task.Targets == nil {
		targetsJSON = []byte("[]")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Every creator takes the same transaction-scoped lock before counting and
	// inserting. Under PostgreSQL's default READ COMMITTED isolation, a waiter
	// observes the preceding creator's commit and cannot oversubscribe either
	// capacity budget.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, scheduledTaskCapacityAdvisoryLockKey); err != nil {
		return err
	}
	var globalCount, principalCount int
	if err = tx.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE created_by = $1)
		  FROM scheduled_tasks
	`, task.CreatedBy).Scan(&globalCount, &principalCount); err != nil {
		return err
	}
	if globalCount >= schedules.MaxScheduledTasksGlobal || principalCount >= schedules.MaxScheduledTasksPerPrincipal {
		return schedules.ErrScheduledTaskCapacityExceeded
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO scheduled_tasks
		 (id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at, last_run_status, last_error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		task.ID, task.Name, task.CronExpr, task.Command, targetsJSON,
		nilIfEmpty(task.GroupID), task.Enabled, task.CreatedBy, task.CreatedAt,
		task.LastRunAt, task.NextRunAt, task.LastRunStatus, task.LastError,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) GetScheduledTask(ctx context.Context, id string) (schedules.ScheduledTask, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at, last_run_status, last_error, last_run_job_id
		 FROM scheduled_tasks WHERE id = $1`, id)
	t, err := scanScheduledTaskRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return schedules.ScheduledTask{}, false, nil
		}
		return schedules.ScheduledTask{}, false, err
	}
	return t, true, nil
}

func (s *PostgresStore) ListScheduledTasks(ctx context.Context, limit, offset int) ([]schedules.ScheduledTask, int, error) {
	limit, offset = normalizeScheduledTaskListBounds(limit, offset)
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM scheduled_tasks`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at, last_run_status, last_error, last_run_job_id
		 FROM scheduled_tasks
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]schedules.ScheduledTask, 0, min(limit, total))
	for rows.Next() {
		t, err := scanScheduledTaskRow(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, t)
	}
	return result, total, rows.Err()
}

func (s *PostgresStore) ListScheduledTasksForEvaluation(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]schedules.ScheduledTask, error) {
	limit, _ = normalizeScheduledTaskListBounds(limit, 0)
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at, last_run_status, last_error, last_run_job_id
		   FROM scheduled_tasks
		  WHERE enabled = TRUE
		    AND (next_run_at IS NULL OR next_run_at <= $1)
		  ORDER BY next_run_at ASC NULLS FIRST, id ASC
		  LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]schedules.ScheduledTask, 0, limit)
	for rows.Next() {
		task, scanErr := scanScheduledTaskRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateScheduledTask(ctx context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, groupID *string, enabled *bool, nextRun schedules.NextRunUpdate) error {
	var setClauses []string
	var args []any
	argIdx := 1

	if name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *name)
		argIdx++
	}
	if cronExpr != nil {
		setClauses = append(setClauses, fmt.Sprintf("cron_expr = $%d", argIdx))
		args = append(args, *cronExpr)
		argIdx++
	}
	if command != nil {
		setClauses = append(setClauses, fmt.Sprintf("command = $%d", argIdx))
		args = append(args, *command)
		argIdx++
	}
	if targets != nil {
		targetsJSON, err := json.Marshal(*targets)
		if err != nil {
			return fmt.Errorf("marshal targets: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("targets = $%d", argIdx))
		args = append(args, targetsJSON)
		argIdx++
	}
	if groupID != nil {
		setClauses = append(setClauses, fmt.Sprintf("group_id = $%d", argIdx))
		args = append(args, nilIfEmpty(*groupID))
		argIdx++
	}
	if enabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *enabled)
		argIdx++
		if !*enabled {
			setClauses = append(setClauses, "last_run_status = 'cancelled'", "last_run_job_id = NULL")
		}
	}
	if nextRun.Set {
		setClauses = append(setClauses, fmt.Sprintf("next_run_at = $%d", argIdx))
		args = append(args, nextRun.Value)
		argIdx++
	}
	if cronExpr != nil || command != nil || targets != nil || groupID != nil || enabled != nil {
		setClauses = append(setClauses, "last_error = ''")
	}

	if len(setClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf("UPDATE scheduled_tasks SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, id)
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// InitializeScheduledTaskNextRun initializes legacy enabled definitions that
// pre-date automatic execution. The conditional update is safe across multiple
// scheduler instances.
func (s *PostgresStore) InitializeScheduledTaskNextRun(ctx context.Context, id string, nextRunAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE scheduled_tasks
		 SET next_run_at = $2, last_error = ''
		 WHERE id = $1 AND enabled = TRUE AND next_run_at IS NULL`,
		strings.TrimSpace(id), nextRunAt.UTC(),
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimScheduledTaskExecution advances a due occurrence and inserts its queue
// job in one transaction. The due-time compare-and-swap prevents duplicate
// dispatch across hub instances; job_queue supplies the execution lease and
// bounded retry lifecycle after commit.
func (s *PostgresStore) ClaimScheduledTaskExecution(ctx context.Context, claim schedules.ExecutionClaim) (bool, error) {
	if strings.TrimSpace(claim.ScheduleID) == "" || strings.TrimSpace(claim.Job.JobID) == "" {
		return false, errors.New("schedule execution claim is incomplete")
	}
	if claim.DueAt.IsZero() || claim.ClaimedAt.IsZero() || claim.NextRunAt.IsZero() || !claim.NextRunAt.After(claim.ClaimedAt) {
		return false, errors.New("schedule execution claim has invalid timestamps")
	}
	if claim.MaxAttempts < 1 {
		claim.MaxAttempts = 1
	}
	if claim.MaxAttempts > 100 {
		claim.MaxAttempts = 100
	}
	payload, err := json.Marshal(claim.Job)
	if err != nil {
		return false, fmt.Errorf("marshal schedule execution job: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE scheduled_tasks
		 SET last_run_at = $3,
		     next_run_at = $4,
		     last_run_status = 'queued',
		     last_error = '',
		     last_run_job_id = $5
		 WHERE id = $1
		   AND enabled = TRUE
		   AND next_run_at = $2
		   AND last_run_status NOT IN ('queued', 'running')`,
		claim.ScheduleID,
		claim.DueAt.UTC(),
		claim.ClaimedAt.UTC(),
		claim.NextRunAt.UTC(),
		claim.Job.JobID,
	)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO job_queue
		 (id, kind, status, payload, attempts, max_attempts, error, created_at, updated_at, available_at, lock_token)
		 VALUES ($1, $2, $3, $4::jsonb, 0, $5, '', $6, $6, $6, NULL)`,
		claim.Job.JobID,
		string(jobqueue.KindScheduleRun),
		string(jobqueue.StatusQueued),
		payload,
		claim.MaxAttempts,
		claim.ClaimedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("insert schedule execution job: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_notify('new_job', '')"); err != nil {
		return false, fmt.Errorf("notify schedule execution workers: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// BeginScheduledTaskExecution is the final pre-dispatch CAS. It prevents a
// duplicate queue delivery from executing an already-finalized occurrence.
func (s *PostgresStore) BeginScheduledTaskExecution(ctx context.Context, scheduleID, jobID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE scheduled_tasks
		 SET last_run_status = 'running'
		 WHERE id = $1
		   AND enabled = TRUE
		   AND last_run_job_id = $2
		   AND last_run_status = 'queued'`,
		strings.TrimSpace(scheduleID), strings.TrimSpace(jobID),
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) MarkScheduledTaskInvalid(ctx context.Context, id, errorMessage string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE scheduled_tasks
		 SET enabled = FALSE,
		     next_run_at = NULL,
		     last_run_status = 'invalid',
		     last_error = $2
		 WHERE id = $1`,
		strings.TrimSpace(id), truncateScheduledTaskError(errorMessage),
	)
	return err
}

func (s *PostgresStore) CompleteScheduledTaskExecution(ctx context.Context, scheduleID, jobID, status, errorMessage string, completedAt time.Time) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE scheduled_tasks
		 SET last_run_status = $3,
		     last_error = $4
		 WHERE id = $1 AND last_run_job_id = $2`,
		strings.TrimSpace(scheduleID),
		strings.TrimSpace(jobID),
		status,
		truncateScheduledTaskError(errorMessage),
	)
	return err
}

func truncateScheduledTaskError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxScheduledTaskErrorLength {
		return value
	}
	return value[:maxScheduledTaskErrorLength]
}

func (s *PostgresStore) DeleteScheduledTask(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM scheduled_tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// nilIfEmpty returns nil if s is the empty string, otherwise returns &s.
// Used to store optional TEXT columns as NULL rather than empty string.
func nilIfEmpty(str string) *string {
	if str == "" {
		return nil
	}
	return &str
}

type scheduledTaskScanner interface {
	Scan(dest ...any) error
}

func scanScheduledTaskRow(row scheduledTaskScanner) (schedules.ScheduledTask, error) {
	var t schedules.ScheduledTask
	var targetsJSON []byte
	var groupID *string
	var lastRunJobID *string
	err := row.Scan(
		&t.ID, &t.Name, &t.CronExpr, &t.Command, &targetsJSON,
		&groupID, &t.Enabled, &t.CreatedBy, &t.CreatedAt,
		&t.LastRunAt, &t.NextRunAt, &t.LastRunStatus, &t.LastError, &lastRunJobID,
	)
	if err != nil {
		return schedules.ScheduledTask{}, err
	}
	if targetsJSON != nil {
		if err := json.Unmarshal(targetsJSON, &t.Targets); err != nil {
			return schedules.ScheduledTask{}, fmt.Errorf("corrupt targets JSON for scheduled task %s: %w", t.ID, err)
		}
	}
	if t.Targets == nil {
		t.Targets = []string{}
	}
	if groupID != nil {
		t.GroupID = *groupID
	}
	if lastRunJobID != nil {
		t.LastRunJobID = *lastRunJobID
	}
	return t, nil
}
