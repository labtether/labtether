package persistence

import (
	"context"

	"github.com/labtether/labtether/internal/model"
)

func (s *PostgresStore) GetResourceCapabilitySetsContext(ctx context.Context, ids []string) ([]model.CapabilitySet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := alertCapabilityBatchIDs(ids)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	// The unique subject key and bounded input IDs bound this result without a
	// LIMIT. Each query has one PostgreSQL statement snapshot.
	rows, err := s.pool.Query(ctx, `SELECT subject_type, subject_id,
		provider_instance_id, capabilities, updated_at
		FROM canonical_capability_sets
		WHERE subject_type = 'resource' AND subject_id = ANY($1::text[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.CapabilitySet, 0, len(ids))
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		set, _, err := scanCapabilitySet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, set)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
