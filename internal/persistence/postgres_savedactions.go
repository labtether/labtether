package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

func (s *PostgresStore) GetSavedAction(ctx context.Context, actorID, id string) (savedactions.SavedAction, bool, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, steps, created_by, created_at
		 FROM saved_actions WHERE created_by = $1 AND id = $2`,
		actorID,
		strings.TrimSpace(id),
	)
	return scanSavedActionRow(row)
}

func (s *PostgresStore) ListSavedActions(ctx context.Context, actorID string, limit, offset int) ([]savedactions.SavedAction, int, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM saved_actions WHERE created_by = $1`, actorID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, steps, created_by, created_at
		 FROM saved_actions
		 WHERE created_by = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		actorID,
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []savedactions.SavedAction
	for rows.Next() {
		var a savedactions.SavedAction
		var stepsJSON []byte
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &stepsJSON, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		if stepsJSON != nil {
			if err := json.Unmarshal(stepsJSON, &a.Steps); err != nil {
				return nil, 0, fmt.Errorf("corrupt steps JSON for saved action %s: %w", a.ID, err)
			}
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (s *PostgresStore) DeleteSavedAction(ctx context.Context, actorID, id string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "system"
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM saved_actions WHERE created_by = $1 AND id = $2`,
		actorID,
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
