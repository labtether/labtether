package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type failoverPairScanner interface {
	Scan(dest ...any) error
}

func scanFailoverPair(row failoverPairScanner) (groupfailover.FailoverPair, error) {
	fp := groupfailover.FailoverPair{}
	var name *string
	var capabilities []byte
	var lastCheckedAt *time.Time
	if err := row.Scan(
		&fp.ID,
		&fp.PrimaryGroupID,
		&fp.BackupGroupID,
		&name,
		&capabilities,
		&fp.ReadinessScore,
		&lastCheckedAt,
		&fp.CreatedAt,
		&fp.UpdatedAt,
	); err != nil {
		return groupfailover.FailoverPair{}, err
	}
	if name != nil {
		fp.Name = *name
	}
	fp.RequiredCapabilities = unmarshalAnyMap(capabilities)
	if lastCheckedAt != nil {
		value := lastCheckedAt.UTC()
		fp.LastCheckedAt = &value
	}
	fp.CreatedAt = fp.CreatedAt.UTC()
	fp.UpdatedAt = fp.UpdatedAt.UTC()
	return fp, nil
}

// --- columns ---

const failoverPairColumns = `id, primary_group_id, backup_group_id, name, required_capabilities, readiness_score, last_checked_at, created_at, updated_at`

// --- store methods ---

func (s *PostgresStore) CreateFailoverPair(req groupfailover.CreatePairRequest) (groupfailover.FailoverPair, error) {
	now := time.Now().UTC()
	primaryGroupID := strings.TrimSpace(req.PrimaryGroupID)
	backupGroupID := strings.TrimSpace(req.BackupGroupID)
	if primaryGroupID == backupGroupID {
		return groupfailover.FailoverPair{}, errors.New("primary_group_id and backup_group_id must be different")
	}
	name := strings.TrimSpace(req.Name)

	capPayload, err := marshalAnyMap(req.RequiredCapabilities)
	if err != nil {
		return groupfailover.FailoverPair{}, err
	}

	fp, err := scanFailoverPair(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`INSERT INTO group_failover_pairs (%s)
		 VALUES ($1, $2, $3, $4, $5::jsonb, 0, NULL, $6, $6)
		 RETURNING %s`, failoverPairColumns, failoverPairColumns),
		idgen.New("sfp"),
		primaryGroupID,
		backupGroupID,
		name,
		capPayload,
		now,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			return groupfailover.FailoverPair{}, errors.New("failover pair already exists for this group combination")
		}
		return groupfailover.FailoverPair{}, err
	}
	return fp, nil
}

func (s *PostgresStore) GetFailoverPair(id string) (groupfailover.FailoverPair, bool, error) {
	fp, err := scanFailoverPair(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_failover_pairs WHERE id = $1`, failoverPairColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groupfailover.FailoverPair{}, false, nil
		}
		return groupfailover.FailoverPair{}, false, err
	}
	return fp, true, nil
}

func (s *PostgresStore) ListFailoverPairs(limit int) ([]groupfailover.FailoverPair, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_failover_pairs ORDER BY updated_at DESC LIMIT $1`, failoverPairColumns),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groupfailover.FailoverPair, 0)
	for rows.Next() {
		fp, scanErr := scanFailoverPair(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, fp)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateFailoverPair(id string, req groupfailover.UpdatePairRequest) (groupfailover.FailoverPair, error) {
	existing, ok, err := s.GetFailoverPair(strings.TrimSpace(id))
	if err != nil {
		return groupfailover.FailoverPair{}, err
	}
	if !ok {
		return groupfailover.FailoverPair{}, groupfailover.ErrPairNotFound
	}

	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.PrimaryGroupID != nil {
		existing.PrimaryGroupID = strings.TrimSpace(*req.PrimaryGroupID)
	}
	if req.BackupGroupID != nil {
		existing.BackupGroupID = strings.TrimSpace(*req.BackupGroupID)
	}
	if existing.PrimaryGroupID == existing.BackupGroupID {
		return groupfailover.FailoverPair{}, errors.New("primary_group_id and backup_group_id must be different")
	}
	if req.RequiredCapabilities != nil {
		existing.RequiredCapabilities = req.RequiredCapabilities
	}

	capPayload, err := marshalAnyMap(existing.RequiredCapabilities)
	if err != nil {
		return groupfailover.FailoverPair{}, err
	}
	now := time.Now().UTC()

	fp, err := scanFailoverPair(s.pool.QueryRow(context.Background(),
		fmt.Sprintf(`UPDATE group_failover_pairs
		 SET primary_group_id = $2, backup_group_id = $3, name = $4,
		     required_capabilities = $5::jsonb, updated_at = $6
		 WHERE id = $1
		 RETURNING %s`, failoverPairColumns),
		existing.ID,
		existing.PrimaryGroupID,
		existing.BackupGroupID,
		nullIfBlank(existing.Name),
		capPayload,
		now,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groupfailover.FailoverPair{}, groupfailover.ErrPairNotFound
		}
		return groupfailover.FailoverPair{}, err
	}
	return fp, nil
}

func (s *PostgresStore) DeleteFailoverPair(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM group_failover_pairs WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return groupfailover.ErrPairNotFound
	}
	return nil
}

func (s *PostgresStore) UpdateFailoverReadiness(id string, score int, checkedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE group_failover_pairs
		 SET readiness_score = $2, last_checked_at = $3, updated_at = $4
		 WHERE id = $1`,
		strings.TrimSpace(id),
		score,
		checkedAt.UTC(),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return groupfailover.ErrPairNotFound
	}
	return nil
}
