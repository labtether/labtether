package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// LatestReliabilityMetricSnapshots returns the newest materialized score for
// each of the first maxGroups groups in stable ID order. The lateral lookup is
// bounded to one history row per selected group and runs as one database query.
func (s *PostgresStore) LatestReliabilityMetricSnapshots(ctx context.Context, at time.Time, maxGroups int) ([]ReliabilityMetricSnapshot, error) {
	if ctx == nil {
		return nil, fmt.Errorf("reliability metric snapshot context is required")
	}
	if maxGroups <= 0 || maxGroups > telemetry.MaxSiteReliabilityMetricSeries {
		return nil, ErrReliabilityMetricSnapshotLimitExceeded
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	rows, err := s.pool.Query(ctx, `
		WITH limited_groups AS MATERIALIZED (
			SELECT id, name
			  FROM groups
			 ORDER BY id
			 LIMIT $1
		)
		SELECT selected.id, selected.name, latest.score
		  FROM limited_groups AS selected
		  JOIN LATERAL (
			SELECT score
			  FROM group_reliability_history
			 WHERE group_id = selected.id
			   AND computed_at <= $2
			   AND computed_at > $2 - INTERVAL '24 hours'
			 ORDER BY computed_at DESC, id DESC
			 LIMIT 1
		  ) AS latest ON TRUE
		 ORDER BY selected.id`, maxGroups, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ReliabilityMetricSnapshot, 0, maxGroups)
	for rows.Next() {
		var snapshot ReliabilityMetricSnapshot
		if err := rows.Scan(&snapshot.GroupID, &snapshot.GroupName, &snapshot.Score); err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
