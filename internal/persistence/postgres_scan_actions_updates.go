package persistence

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/updates"
)

type actionRunScanner interface {
	Scan(dest ...any) error
}

func scanActionRun(row actionRunScanner) (actions.Run, error) {
	run := actions.Run{}
	var target *string
	var command *string
	var connectorID *string
	var actionID *string
	var params []byte
	var completedAt *time.Time
	if err := row.Scan(
		&run.ID,
		&run.Type,
		&run.ActorID,
		&target,
		&command,
		&connectorID,
		&actionID,
		&params,
		&run.DryRun,
		&run.Status,
		&run.Output,
		&run.Error,
		&run.CreatedAt,
		&run.UpdatedAt,
		&completedAt,
	); err != nil {
		return actions.Run{}, err
	}

	if target != nil {
		run.Target = *target
	}
	if command != nil {
		run.Command = *command
	}
	if connectorID != nil {
		run.ConnectorID = *connectorID
	}
	if actionID != nil {
		run.ActionID = *actionID
	}
	if completedAt != nil {
		t := completedAt.UTC()
		run.CompletedAt = &t
	}

	if len(params) > 0 {
		parsed := map[string]string{}
		if err := json.Unmarshal(params, &parsed); err == nil {
			run.Params = parsed
		}
	}
	run.Params = cloneMetadata(run.Params)
	return run, nil
}

func (s *PostgresStore) listActionRunSteps(runID string) ([]actions.RunStep, error) {
	stepsByRunID, err := s.listActionRunStepsByRunIDs([]string{runID})
	if err != nil {
		return nil, err
	}
	steps := stepsByRunID[strings.TrimSpace(runID)]
	if steps == nil {
		return []actions.RunStep{}, nil
	}
	return steps, nil
}

func (s *PostgresStore) listActionRunStepsByRunIDs(runIDs []string) (map[string][]actions.RunStep, error) {
	cleanedRunIDs := make([]string, 0, len(runIDs))
	seen := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		normalized := strings.TrimSpace(runID)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cleanedRunIDs = append(cleanedRunIDs, normalized)
	}
	if len(cleanedRunIDs) == 0 {
		return map[string][]actions.RunStep{}, nil
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, run_id, name, status, output, error, created_at, updated_at
		 FROM action_run_steps
		 WHERE run_id = ANY($1)
		 ORDER BY run_id ASC, created_at ASC`,
		cleanedRunIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stepsByRunID := make(map[string][]actions.RunStep, len(cleanedRunIDs))
	for rows.Next() {
		step := actions.RunStep{}
		if err := rows.Scan(
			&step.ID,
			&step.RunID,
			&step.Name,
			&step.Status,
			&step.Output,
			&step.Error,
			&step.CreatedAt,
			&step.UpdatedAt,
		); err != nil {
			return nil, err
		}
		runID := strings.TrimSpace(step.RunID)
		stepsByRunID[runID] = append(stepsByRunID[runID], step)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	for _, runID := range cleanedRunIDs {
		if _, ok := stepsByRunID[runID]; !ok {
			stepsByRunID[runID] = []actions.RunStep{}
		}
	}
	return stepsByRunID, nil
}

type updatePlanScanner interface {
	Scan(dest ...any) error
}

func scanUpdatePlan(row updatePlanScanner) (updates.Plan, error) {
	plan := updates.Plan{}
	var targets []byte
	var scopes []byte
	if err := row.Scan(
		&plan.ID,
		&plan.Name,
		&plan.Description,
		&targets,
		&scopes,
		&plan.DefaultDryRun,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	); err != nil {
		return updates.Plan{}, err
	}

	plan.Targets = unmarshalStringSlice(targets)
	plan.Scopes = unmarshalStringSlice(scopes)
	return plan, nil
}

type updateRunScanner interface {
	Scan(dest ...any) error
}

func scanUpdateRun(row updateRunScanner) (updates.Run, error) {
	run := updates.Run{}
	var results []byte
	var completedAt *time.Time
	if err := row.Scan(
		&run.ID,
		&run.PlanID,
		&run.PlanName,
		&run.ActorID,
		&run.DryRun,
		&run.Status,
		&run.Summary,
		&run.Error,
		&results,
		&run.CreatedAt,
		&run.UpdatedAt,
		&completedAt,
	); err != nil {
		return updates.Run{}, err
	}

	if completedAt != nil {
		t := completedAt.UTC()
		run.CompletedAt = &t
	}
	run.Results = unmarshalUpdateRunResults(results)
	return run, nil
}
