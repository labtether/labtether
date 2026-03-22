package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/idgen"
)

// --- scan helpers ---

type reliabilityRecordScanner interface {
	Scan(dest ...any) error
}

func scanReliabilityRecord(row reliabilityRecordScanner) (ReliabilityRecord, error) {
	r := ReliabilityRecord{}
	var factors []byte
	if err := row.Scan(
		&r.ID,
		&r.GroupID,
		&r.Score,
		&r.Grade,
		&factors,
		&r.WindowHours,
		&r.ComputedAt,
	); err != nil {
		return ReliabilityRecord{}, err
	}
	r.Factors = unmarshalAnyMap(factors)
	r.ComputedAt = r.ComputedAt.UTC()
	return r, nil
}

// --- columns ---

const reliabilityRecordColumns = `id, group_id, score, grade, factors, window_hours, computed_at`

// --- store methods ---

func (s *PostgresStore) InsertReliabilityRecord(groupID string, score int, grade string, factors map[string]any, windowHours int) error {
	now := time.Now().UTC()

	factorsPayload, err := marshalAnyMap(factors)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO group_reliability_history (id, group_id, score, grade, factors, window_hours, computed_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
		idgen.New("rh"),
		strings.TrimSpace(groupID),
		score,
		strings.TrimSpace(grade),
		factorsPayload,
		windowHours,
		now,
	)
	return err
}

func (s *PostgresStore) ListReliabilityHistory(groupID string, days int) ([]ReliabilityRecord, error) {
	if days <= 0 {
		days = 7
	}
	if days > 365 {
		days = 365
	}

	rows, err := s.pool.Query(context.Background(),
		fmt.Sprintf(`SELECT %s FROM group_reliability_history
		 WHERE group_id = $1 AND computed_at > now() - make_interval(days => $2)
		 ORDER BY computed_at DESC`, reliabilityRecordColumns),
		strings.TrimSpace(groupID),
		days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ReliabilityRecord, 0, 64)
	for rows.Next() {
		r, scanErr := scanReliabilityRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) PruneReliabilityHistory(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		olderThanDays = 90
	}

	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM group_reliability_history
		 WHERE computed_at < now() - make_interval(days => $1)`,
		olderThanDays,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
