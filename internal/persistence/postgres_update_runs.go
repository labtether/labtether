package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/updates"
	"strings"
	"time"
)

func (s *PostgresStore) CreateUpdateRun(plan updates.Plan, req updates.ExecutePlanRequest) (updates.Run, error) {
	now := time.Now().UTC()
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = "owner"
	}

	dryRun := plan.DefaultDryRun
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	resultsPayload, err := marshalUpdateRunResults(nil)
	if err != nil {
		return updates.Run{}, err
	}

	run := updates.Run{
		ID:        idgen.New("uprun"),
		PlanID:    plan.ID,
		PlanName:  plan.Name,
		ActorID:   actorID,
		DryRun:    dryRun,
		Status:    updates.StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO update_runs (id, plan_id, plan_name, actor_id, dry_run, status, summary, error, results, created_at, updated_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, '', '', $7::jsonb, $8, $8, NULL)`,
		run.ID,
		run.PlanID,
		run.PlanName,
		run.ActorID,
		run.DryRun,
		run.Status,
		resultsPayload,
		run.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			// Keep the Postgres contract aligned with MemoryUpdateStore when a
			// concurrent plan deletion wins before this run can acquire its FK
			// key-share lock.
			return updates.Run{}, ErrNotFound
		}
		return updates.Run{}, err
	}

	return run, nil
}

func (s *PostgresStore) GetUpdateRun(id string) (updates.Run, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, plan_id, plan_name, actor_id, dry_run, status, summary, error, results, created_at, updated_at, completed_at
		 FROM update_runs
		 WHERE id = $1`,
		id,
	)

	run, err := scanUpdateRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return updates.Run{}, false, nil
		}
		return updates.Run{}, false, err
	}
	return run, true, nil
}

func (s *PostgresStore) ListUpdateRuns(limit int, status string) ([]updates.Run, error) {
	return s.ListUpdateRunsPage(limit, 0, status)
}

func (s *PostgresStore) ListUpdateRunsPage(limit, offset int, status string) ([]updates.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	status = updates.NormalizeStatus(status)
	args := make([]any, 0, 2)
	sql := `SELECT id, plan_id, plan_name, actor_id, dry_run, status, summary, error, results, created_at, updated_at, completed_at
		FROM update_runs`
	if status != "" {
		sql += " WHERE status = $1"
		args = append(args, status)
		sql += " ORDER BY updated_at DESC LIMIT $2"
		args = append(args, limit)
		if offset > 0 {
			sql += " OFFSET $3"
			args = append(args, offset)
		}
	} else {
		sql += " ORDER BY updated_at DESC LIMIT $1"
		args = append(args, limit)
		if offset > 0 {
			sql += " OFFSET $2"
			args = append(args, offset)
		}
	}

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]updates.Run, 0)
	for rows.Next() {
		run, err := scanUpdateRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) DeleteUpdateRun(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM update_runs WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ApplyUpdateResult(result updates.Result) error {
	status := updates.NormalizeStatus(result.Status)
	if status == "" {
		status = updates.StatusFailed
	}
	completedAt := result.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}

	resultsPayload, err := marshalUpdateRunResults(result.Results)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(context.Background(),
		`UPDATE update_runs
		 SET status = $2,
		     summary = $3,
		     error = $4,
		     results = $5::jsonb,
		     updated_at = $6,
		     completed_at = $6
		 WHERE id = $1`,
		result.RunID,
		status,
		strings.TrimSpace(result.Summary),
		strings.TrimSpace(result.Error),
		resultsPayload,
		completedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("update run not found")
	}
	return nil
}

func (s *PostgresStore) UpdateRunsWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT COALESCE(MAX(updated_at), to_timestamp(0)) FROM update_runs`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}
