package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/idgen"
)

const groupMaintenanceWindowColumns = `id, group_id, name, start_at, end_at, suppress_alerts, block_actions, block_updates, created_at, updated_at`

type groupMaintenanceWindowScanner interface {
	Scan(dest ...any) error
}

func scanGroupMaintenanceWindow(row groupMaintenanceWindowScanner) (groupmaintenance.MaintenanceWindow, error) {
	window := groupmaintenance.MaintenanceWindow{}
	if err := row.Scan(
		&window.ID,
		&window.GroupID,
		&window.Name,
		&window.StartAt,
		&window.EndAt,
		&window.SuppressAlerts,
		&window.BlockActions,
		&window.BlockUpdates,
		&window.CreatedAt,
		&window.UpdatedAt,
	); err != nil {
		return groupmaintenance.MaintenanceWindow{}, err
	}
	window.StartAt = window.StartAt.UTC()
	window.EndAt = window.EndAt.UTC()
	window.CreatedAt = window.CreatedAt.UTC()
	window.UpdatedAt = window.UpdatedAt.UTC()
	return window, nil
}

func (s *PostgresStore) groupExists(ctx context.Context, groupID string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)`, strings.TrimSpace(groupID)).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *PostgresStore) CreateGroupMaintenanceWindow(groupID string, req groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	ctx := context.Background()
	groupID = strings.TrimSpace(groupID)
	if exists, err := s.groupExists(ctx, groupID); err != nil {
		return groupmaintenance.MaintenanceWindow{}, err
	} else if !exists {
		return groupmaintenance.MaintenanceWindow{}, groupmaintenance.ErrGroupNotFound
	}

	now := time.Now().UTC()
	return scanGroupMaintenanceWindow(s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO group_maintenance_windows (%s)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		 RETURNING %s`, groupMaintenanceWindowColumns, groupMaintenanceWindowColumns),
		idgen.New("gmw"),
		groupID,
		strings.TrimSpace(req.Name),
		req.StartAt.UTC(),
		req.EndAt.UTC(),
		req.SuppressAlerts,
		req.BlockActions,
		req.BlockUpdates,
		now,
	))
}

func (s *PostgresStore) GetGroupMaintenanceWindow(groupID, windowID string) (groupmaintenance.MaintenanceWindow, bool, error) {
	ctx := context.Background()
	window, err := scanGroupMaintenanceWindow(s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s
		 FROM group_maintenance_windows
		 WHERE group_id = $1 AND id = $2`, groupMaintenanceWindowColumns),
		strings.TrimSpace(groupID),
		strings.TrimSpace(windowID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if exists, existsErr := s.groupExists(ctx, groupID); existsErr != nil {
				return groupmaintenance.MaintenanceWindow{}, false, existsErr
			} else if !exists {
				return groupmaintenance.MaintenanceWindow{}, false, groupmaintenance.ErrGroupNotFound
			}
			return groupmaintenance.MaintenanceWindow{}, false, nil
		}
		return groupmaintenance.MaintenanceWindow{}, false, err
	}
	return window, true, nil
}

func (s *PostgresStore) ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error) {
	ctx := context.Background()
	groupID = strings.TrimSpace(groupID)
	if exists, err := s.groupExists(ctx, groupID); err != nil {
		return nil, err
	} else if !exists {
		return nil, groupmaintenance.ErrGroupNotFound
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := fmt.Sprintf(`SELECT %s
		FROM group_maintenance_windows
		WHERE group_id = $1`, groupMaintenanceWindowColumns)
	args := []any{groupID}
	if activeAt != nil && !activeAt.IsZero() {
		query += ` AND start_at <= $2 AND end_at >= $2`
		args = append(args, activeAt.UTC())
		query += ` ORDER BY start_at ASC LIMIT $3`
		args = append(args, limit)
	} else {
		query += ` ORDER BY start_at DESC LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupmaintenance.MaintenanceWindow, 0)
	for rows.Next() {
		window, scanErr := scanGroupMaintenanceWindow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, window)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	window, ok, err := s.GetGroupMaintenanceWindow(groupID, windowID)
	if err != nil {
		return groupmaintenance.MaintenanceWindow{}, err
	}
	if !ok {
		return groupmaintenance.MaintenanceWindow{}, groupmaintenance.ErrMaintenanceWindowNotFound
	}

	window.Name = strings.TrimSpace(req.Name)
	window.StartAt = req.StartAt.UTC()
	window.EndAt = req.EndAt.UTC()
	window.SuppressAlerts = req.SuppressAlerts
	window.BlockActions = req.BlockActions
	window.BlockUpdates = req.BlockUpdates

	updated, err := scanGroupMaintenanceWindow(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`UPDATE group_maintenance_windows
		 SET name = $3,
		     start_at = $4,
		     end_at = $5,
		     suppress_alerts = $6,
		     block_actions = $7,
		     block_updates = $8,
		     updated_at = $9
		 WHERE group_id = $1 AND id = $2
		 RETURNING %s`, groupMaintenanceWindowColumns),
		strings.TrimSpace(groupID),
		strings.TrimSpace(windowID),
		window.Name,
		window.StartAt,
		window.EndAt,
		window.SuppressAlerts,
		window.BlockActions,
		window.BlockUpdates,
		time.Now().UTC(),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groupmaintenance.MaintenanceWindow{}, groupmaintenance.ErrMaintenanceWindowNotFound
		}
		return groupmaintenance.MaintenanceWindow{}, err
	}
	return updated, nil
}

func (s *PostgresStore) DeleteGroupMaintenanceWindow(groupID, windowID string) error {
	ctx := context.Background()
	groupID = strings.TrimSpace(groupID)
	windowID = strings.TrimSpace(windowID)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM group_maintenance_windows WHERE group_id = $1 AND id = $2`,
		groupID,
		windowID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	if exists, existsErr := s.groupExists(ctx, groupID); existsErr != nil {
		return existsErr
	} else if !exists {
		return groupmaintenance.ErrGroupNotFound
	}
	return groupmaintenance.ErrMaintenanceWindowNotFound
}
