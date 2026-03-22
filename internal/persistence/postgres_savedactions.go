package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/savedactions"
)

func (s *PostgresStore) CreateSavedAction(ctx context.Context, action savedactions.SavedAction) error {
	stepsJSON, err := json.Marshal(action.Steps)
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}
	if action.Steps == nil {
		stepsJSON = []byte("[]")
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO saved_actions (id, name, description, steps, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		action.ID, action.Name, action.Description, stepsJSON, action.CreatedBy, action.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetSavedAction(ctx context.Context, id string) (savedactions.SavedAction, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, steps, created_by, created_at
		 FROM saved_actions WHERE id = $1`, id)
	return scanSavedActionRow(row)
}

func (s *PostgresStore) ListSavedActions(ctx context.Context) ([]savedactions.SavedAction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, steps, created_by, created_at
		 FROM saved_actions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []savedactions.SavedAction
	for rows.Next() {
		var a savedactions.SavedAction
		var stepsJSON []byte
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &stepsJSON, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		if stepsJSON != nil {
			if err := json.Unmarshal(stepsJSON, &a.Steps); err != nil {
				return nil, fmt.Errorf("corrupt steps JSON for saved action %s: %w", a.ID, err)
			}
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// DeleteSavedAction removes a saved action by ID. Returns nil even if the
// action does not exist (idempotent delete).
func (s *PostgresStore) DeleteSavedAction(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM saved_actions WHERE id = $1`, id)
	return err
}

func scanSavedActionRow(row pgx.Row) (savedactions.SavedAction, bool, error) {
	var a savedactions.SavedAction
	var stepsJSON []byte
	err := row.Scan(&a.ID, &a.Name, &a.Description, &stepsJSON, &a.CreatedBy, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return savedactions.SavedAction{}, false, nil
		}
		return savedactions.SavedAction{}, false, err
	}
	if stepsJSON != nil {
		if err := json.Unmarshal(stepsJSON, &a.Steps); err != nil {
			return savedactions.SavedAction{}, false, fmt.Errorf("corrupt steps JSON for saved action %s: %w", a.ID, err)
		}
	}
	return a, true, nil
}
