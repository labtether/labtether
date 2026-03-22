package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/groupprofiles"
	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type groupProfileScanner interface {
	Scan(dest ...any) error
}

func scanGroupProfile(row groupProfileScanner) (groupprofiles.Profile, error) {
	p := groupprofiles.Profile{}
	var config []byte
	if err := row.Scan(
		&p.ID,
		&p.Name,
		&p.Description,
		&config,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		return groupprofiles.Profile{}, err
	}
	p.Config = unmarshalAnyMap(config)
	p.CreatedAt = p.CreatedAt.UTC()
	p.UpdatedAt = p.UpdatedAt.UTC()
	return p, nil
}

type groupProfileAssignmentScanner interface {
	Scan(dest ...any) error
}

func scanGroupProfileAssignment(row groupProfileAssignmentScanner) (groupprofiles.Assignment, error) {
	a := groupprofiles.Assignment{}
	var assignedBy *string
	if err := row.Scan(
		&a.ID,
		&a.GroupID,
		&a.ProfileID,
		&assignedBy,
		&a.AssignedAt,
	); err != nil {
		return groupprofiles.Assignment{}, err
	}
	if assignedBy != nil {
		a.AssignedBy = *assignedBy
	}
	a.AssignedAt = a.AssignedAt.UTC()
	return a, nil
}

type driftCheckScanner interface {
	Scan(dest ...any) error
}

func scanDriftCheck(row driftCheckScanner) (groupprofiles.DriftCheck, error) {
	d := groupprofiles.DriftCheck{}
	var driftDetails []byte
	if err := row.Scan(
		&d.ID,
		&d.GroupID,
		&d.ProfileID,
		&d.Status,
		&driftDetails,
		&d.CheckedAt,
	); err != nil {
		return groupprofiles.DriftCheck{}, err
	}
	d.DriftDetails = unmarshalAnyMap(driftDetails)
	d.CheckedAt = d.CheckedAt.UTC()
	return d, nil
}

// --- columns ---

const groupProfileColumns = `id, name, description, config, created_at, updated_at`
const groupProfileAssignmentColumns = `id, group_id, profile_id, assigned_by, assigned_at`
const driftCheckColumns = `id, group_id, profile_id, status, drift_details, checked_at`

// --- store methods ---

func (s *PostgresStore) CreateGroupProfile(req groupprofiles.CreateProfileRequest) (groupprofiles.Profile, error) {
	now := time.Now().UTC()

	configPayload, err := marshalAnyMap(req.Config)
	if err != nil {
		return groupprofiles.Profile{}, err
	}

	return scanGroupProfile(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO group_profiles (%s)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $5)
		 RETURNING %s`, groupProfileColumns, groupProfileColumns),
		idgen.New("sp"),
		strings.TrimSpace(req.Name),
		strings.TrimSpace(req.Description),
		configPayload,
		now,
	))
}

func (s *PostgresStore) GetGroupProfile(id string) (groupprofiles.Profile, bool, error) {
	profile, err := scanGroupProfile(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_profiles WHERE id = $1`, groupProfileColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groupprofiles.Profile{}, false, nil
		}
		return groupprofiles.Profile{}, false, err
	}
	return profile, true, nil
}

func (s *PostgresStore) ListGroupProfiles(limit int) ([]groupprofiles.Profile, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_profiles ORDER BY updated_at DESC LIMIT $1`, groupProfileColumns),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupprofiles.Profile, 0, limit)
	for rows.Next() {
		p, scanErr := scanGroupProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateGroupProfile(id string, req groupprofiles.UpdateProfileRequest) (groupprofiles.Profile, error) {
	existing, ok, err := s.GetGroupProfile(id)
	if err != nil {
		return groupprofiles.Profile{}, err
	}
	if !ok {
		return groupprofiles.Profile{}, groupprofiles.ErrProfileNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Config != nil {
		existing.Config = cloneAnyMap(*req.Config)
	}

	configPayload, err := marshalAnyMap(existing.Config)
	if err != nil {
		return groupprofiles.Profile{}, err
	}

	now := time.Now().UTC()
	return scanGroupProfile(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`UPDATE group_profiles
		 SET name = $2, description = $3, config = $4::jsonb, updated_at = $5
		 WHERE id = $1
		 RETURNING %s`, groupProfileColumns),
		existing.ID,
		existing.Name,
		existing.Description,
		configPayload,
		now,
	))
}

func (s *PostgresStore) DeleteGroupProfile(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM group_profiles WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return groupprofiles.ErrProfileNotFound
	}
	return nil
}

func (s *PostgresStore) AssignGroupProfile(groupID, profileID, assignedBy string) (groupprofiles.Assignment, error) {
	now := time.Now().UTC()
	groupID = strings.TrimSpace(groupID)
	profileID = strings.TrimSpace(profileID)
	if assignedBy == "" {
		assignedBy = "owner"
	}

	return scanGroupProfileAssignment(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO group_profile_assignments (%s)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (group_id) DO UPDATE
		 SET profile_id = EXCLUDED.profile_id,
		     assigned_by = EXCLUDED.assigned_by,
		     assigned_at = EXCLUDED.assigned_at
		 RETURNING %s`, groupProfileAssignmentColumns, groupProfileAssignmentColumns),
		idgen.New("spa"),
		groupID,
		profileID,
		nullIfBlank(assignedBy),
		now,
	))
}

func (s *PostgresStore) GetGroupProfileAssignment(groupID string) (groupprofiles.Assignment, bool, error) {
	a, err := scanGroupProfileAssignment(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_profile_assignments WHERE group_id = $1`, groupProfileAssignmentColumns),
		strings.TrimSpace(groupID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groupprofiles.Assignment{}, false, nil
		}
		return groupprofiles.Assignment{}, false, err
	}
	return a, true, nil
}

func (s *PostgresStore) RemoveGroupProfileAssignment(groupID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM group_profile_assignments WHERE group_id = $1`,
		strings.TrimSpace(groupID),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return groupprofiles.ErrProfileNotFound
	}
	return nil
}

func (s *PostgresStore) RecordDriftCheck(check groupprofiles.DriftCheck) (groupprofiles.DriftCheck, error) {
	checkedAt := check.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}

	status := groupprofiles.NormalizeDriftStatus(check.Status)
	if status == "" {
		status = groupprofiles.DriftStatusCompliant
	}

	driftPayload, err := marshalAnyMap(check.DriftDetails)
	if err != nil {
		return groupprofiles.DriftCheck{}, err
	}

	return scanDriftCheck(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO group_profile_drift_checks (%s)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		 RETURNING %s`, driftCheckColumns, driftCheckColumns),
		idgen.New("spd"),
		strings.TrimSpace(check.GroupID),
		strings.TrimSpace(check.ProfileID),
		status,
		driftPayload,
		checkedAt.UTC(),
	))
}

func (s *PostgresStore) ListDriftChecks(groupID string, limit int) ([]groupprofiles.DriftCheck, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_profile_drift_checks
		 WHERE group_id = $1 ORDER BY checked_at DESC LIMIT $2`, driftCheckColumns),
		strings.TrimSpace(groupID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupprofiles.DriftCheck, 0, limit)
	for rows.Next() {
		d, scanErr := scanDriftCheck(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
