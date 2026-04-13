package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/schedules"
)

func (s *PostgresStore) CreateScheduledTask(ctx context.Context, task schedules.ScheduledTask) error {
	targetsJSON, err := json.Marshal(task.Targets)
	if err != nil {
		return fmt.Errorf("marshal targets: %w", err)
	}
	if task.Targets == nil {
		targetsJSON = []byte("[]")
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO scheduled_tasks
		 (id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		task.ID, task.Name, task.CronExpr, task.Command, targetsJSON,
		nilIfEmpty(task.GroupID), task.Enabled, task.CreatedBy, task.CreatedAt,
		task.LastRunAt, task.NextRunAt,
	)
	return err
}

func (s *PostgresStore) GetScheduledTask(ctx context.Context, id string) (schedules.ScheduledTask, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at
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

func (s *PostgresStore) ListScheduledTasks(ctx context.Context) ([]schedules.ScheduledTask, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, cron_expr, command, targets, group_id, enabled, created_by, created_at, last_run_at, next_run_at
		 FROM scheduled_tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []schedules.ScheduledTask
	for rows.Next() {
		t, err := scanScheduledTaskRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateScheduledTask(ctx context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, groupID *string, enabled *bool) error {
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
	err := row.Scan(
		&t.ID, &t.Name, &t.CronExpr, &t.Command, &targetsJSON,
		&groupID, &t.Enabled, &t.CreatedBy, &t.CreatedAt,
		&t.LastRunAt, &t.NextRunAt,
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
	return t, nil
}
