package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type maintenanceOverrideScanner interface {
	Scan(dest ...any) error
}

func scanMaintenanceOverride(row maintenanceOverrideScanner) (groupmaintenance.MaintenanceOverride, error) {
	o := groupmaintenance.MaintenanceOverride{}
	var referenceID *string
	var approvedBy *string
	if err := row.Scan(
		&o.ID,
		&o.MaintenanceWindowID,
		&o.OverrideType,
		&o.Reason,
		&referenceID,
		&approvedBy,
		&o.CreatedAt,
	); err != nil {
		return groupmaintenance.MaintenanceOverride{}, err
	}
	if referenceID != nil {
		o.ReferenceID = *referenceID
	}
	if approvedBy != nil {
		o.ApprovedBy = *approvedBy
	}
	o.CreatedAt = o.CreatedAt.UTC()
	return o, nil
}

// --- columns ---

const maintenanceOverrideColumns = `id, maintenance_window_id, override_type, reason, reference_id, approved_by, created_at`

// --- store methods ---

func (s *PostgresStore) CreateMaintenanceOverride(req groupmaintenance.CreateOverrideRequest) (groupmaintenance.MaintenanceOverride, error) {
	now := time.Now().UTC()

	overrideType := strings.TrimSpace(req.OverrideType)
	if overrideType == "" {
		overrideType = groupmaintenance.OverrideTypeAction
	}

	return scanMaintenanceOverride(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO maintenance_overrides (%s)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING %s`, maintenanceOverrideColumns, maintenanceOverrideColumns),
		idgen.New("mov"),
		strings.TrimSpace(req.MaintenanceWindowID),
		overrideType,
		strings.TrimSpace(req.Reason),
		nullIfBlank(req.ReferenceID),
		nullIfBlank(req.ApprovedBy),
		now,
	))
}

func (s *PostgresStore) ListMaintenanceOverrides(windowID string, limit int) ([]groupmaintenance.MaintenanceOverride, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM maintenance_overrides
		 WHERE maintenance_window_id = $1 ORDER BY created_at DESC LIMIT $2`, maintenanceOverrideColumns),
		strings.TrimSpace(windowID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupmaintenance.MaintenanceOverride, 0)
	for rows.Next() {
		o, scanErr := scanMaintenanceOverride(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
