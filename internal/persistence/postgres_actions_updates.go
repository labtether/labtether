package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/updates"
)

func (s *PostgresStore) CreateActionRun(req actions.ExecuteRequest) (actions.Run, error) {
	now := time.Now().UTC()
	runType := actions.NormalizeRunType(req.Type)
	if runType == "" {
		if strings.TrimSpace(req.ConnectorID) != "" || strings.TrimSpace(req.ActionID) != "" {
			runType = actions.RunTypeConnectorAction
		} else {
			runType = actions.RunTypeCommand
		}
	}

	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = "owner"
	}

	paramsPayload, err := marshalStringMap(req.Params)
	if err != nil {
		return actions.Run{}, err
	}

	run := actions.Run{
		ID:          idgen.New("actrun"),
		Type:        runType,
		ActorID:     actorID,
		Target:      strings.TrimSpace(req.Target),
		Command:     strings.TrimSpace(req.Command),
		ConnectorID: strings.TrimSpace(req.ConnectorID),
		ActionID:    strings.TrimSpace(req.ActionID),
		Params:      cloneMetadata(req.Params),
		DryRun:      req.DryRun,
		Status:      actions.StatusQueued,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO action_runs (id, type, actor_id, target, command, connector_id, action_id, params, dry_run, status, output, error, created_at, updated_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, '', '', $11, $11, NULL)`,
		run.ID,
		run.Type,
		run.ActorID,
		nullIfBlank(run.Target),
		nullIfBlank(run.Command),
		nullIfBlank(run.ConnectorID),
		nullIfBlank(run.ActionID),
		paramsPayload,
		run.DryRun,
		run.Status,
		run.CreatedAt,
	)
	if err != nil {
		return actions.Run{}, err
	}

	return run, nil
}

func (s *PostgresStore) GetActionRun(id string) (actions.Run, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, type, actor_id, target, command, connector_id, action_id, params, dry_run, status, output, error, created_at, updated_at, completed_at
		 FROM action_runs
		 WHERE id = $1`,
		id,
	)

	run, err := scanActionRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return actions.Run{}, false, nil
		}
		return actions.Run{}, false, err
	}

	steps, err := s.listActionRunSteps(run.ID)
	if err != nil {
		return actions.Run{}, false, err
	}
	run.Steps = steps
	return run, true, nil
}

func (s *PostgresStore) ListActionRuns(limit, offset int, runType, status string) ([]actions.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	next := 1

	normalizedType := actions.NormalizeRunType(runType)
	if normalizedType != "" {
		where = append(where, fmt.Sprintf("type = $%d", next))
		args = append(args, normalizedType)
		next++
	}

	normalizedStatus := actions.NormalizeStatus(status)
	if normalizedStatus != "" {
		where = append(where, fmt.Sprintf("status = $%d", next))
		args = append(args, normalizedStatus)
		next++
	}

	sql := `SELECT id, type, actor_id, target, command, connector_id, action_id, params, dry_run, status, output, error, created_at, updated_at, completed_at
		FROM action_runs`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", next)
	args = append(args, limit)
	next++
	if offset > 0 {
		args = append(args, offset)
		sql += fmt.Sprintf(" OFFSET $%d", next)
	}

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]actions.Run, 0, limit)
	for rows.Next() {
		run, err := scanActionRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	runIDs := make([]string, 0, len(out))
	for _, run := range out {
		runIDs = append(runIDs, run.ID)
	}
	stepsByRunID, err := s.listActionRunStepsByRunIDs(runIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		steps := stepsByRunID[strings.TrimSpace(out[i].ID)]
		if steps == nil {
			steps = []actions.RunStep{}
		}
		out[i].Steps = steps
	}

	return out, nil
}

func (s *PostgresStore) DeleteActionRun(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM action_runs WHERE id = $1`,
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

func (s *PostgresStore) ApplyActionResult(result actions.Result) error {
	status := actions.NormalizeStatus(result.Status)
	if status == "" {
		status = actions.StatusFailed
	}

	completedAt := result.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	tag, err := tx.Exec(context.Background(),
		`UPDATE action_runs
		 SET status = $2,
		     output = $3,
		     error = $4,
		     updated_at = $5,
		     completed_at = $5
		 WHERE id = $1`,
		result.RunID,
		status,
		strings.TrimSpace(result.Output),
		strings.TrimSpace(result.Error),
		completedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("action run not found")
	}

	if _, err := tx.Exec(context.Background(), `DELETE FROM action_run_steps WHERE run_id = $1`, result.RunID); err != nil {
		return err
	}

	for _, step := range result.Steps {
		stepStatus := actions.NormalizeStatus(step.Status)
		if stepStatus == "" {
			stepStatus = actions.StatusFailed
		}
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO action_run_steps (id, run_id, name, status, output, error, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
			idgen.New("actstep"),
			result.RunID,
			strings.TrimSpace(step.Name),
			stepStatus,
			strings.TrimSpace(step.Output),
			strings.TrimSpace(step.Error),
			completedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) CreateUpdatePlan(req updates.CreatePlanRequest) (updates.Plan, error) {
	now := time.Now().UTC()
	defaultDryRun := true
	if req.DefaultDryRun != nil {
		defaultDryRun = *req.DefaultDryRun
	}

	targets := sanitizeStringSlice(req.Targets)
	scopes := sanitizeStringSlice(req.Scopes)
	if len(scopes) == 0 {
		scopes = append([]string(nil), updates.DefaultScopes...)
	}

	targetPayload, err := marshalStringSlice(targets)
	if err != nil {
		return updates.Plan{}, err
	}
	scopePayload, err := marshalStringSlice(scopes)
	if err != nil {
		return updates.Plan{}, err
	}

	plan := updates.Plan{
		ID:            idgen.New("upln"),
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Targets:       targets,
		Scopes:        scopes,
		DefaultDryRun: defaultDryRun,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO update_plans (id, name, description, targets, scopes, default_dry_run, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6, $7, $7)`,
		plan.ID,
		plan.Name,
		plan.Description,
		targetPayload,
		scopePayload,
		plan.DefaultDryRun,
		plan.CreatedAt,
	)
	if err != nil {
		return updates.Plan{}, err
	}

	return plan, nil
}

func (s *PostgresStore) ListUpdatePlans(limit int) ([]updates.Plan, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, name, description, targets, scopes, default_dry_run, created_at, updated_at
		 FROM update_plans
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]updates.Plan, 0, limit)
	for rows.Next() {
		plan, err := scanUpdatePlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, plan)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) GetUpdatePlan(id string) (updates.Plan, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, description, targets, scopes, default_dry_run, created_at, updated_at
		 FROM update_plans
		 WHERE id = $1`,
		id,
	)

	plan, err := scanUpdatePlan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return updates.Plan{}, false, nil
		}
		return updates.Plan{}, false, err
	}
	return plan, true, nil
}

func (s *PostgresStore) DeleteUpdatePlan(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM update_plans WHERE id = $1`,
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
	} else {
		sql += " ORDER BY updated_at DESC LIMIT $1"
		args = append(args, limit)
	}

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]updates.Run, 0, limit)
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

func (s *PostgresStore) ActionRunsWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT COALESCE(MAX(updated_at), to_timestamp(0)) FROM action_runs`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
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

func (s *PostgresStore) UpdatePlansWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT COALESCE(MAX(updated_at), to_timestamp(0)) FROM update_plans`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}
