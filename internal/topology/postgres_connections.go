package topology

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// ---------------------------------------------------------------------------
// Connections
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateConnection(c Connection) (Connection, error) {
	ctx := context.Background()

	// First, try to re-activate a previously soft-deleted connection with the same key.
	var result Connection
	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`UPDATE topology_connections
		 SET deleted = false, user_defined = $5, label = $6
		 WHERE topology_id = $1
		   AND source_asset_id = $2
		   AND target_asset_id = $3
		   AND relationship = $4
		   AND deleted = true
		 RETURNING id, topology_id, source_asset_id, target_asset_id, relationship, user_defined, label, deleted`,
		c.TopologyID, c.SourceAssetID, c.TargetAssetID, c.Relationship, c.UserDefined, c.Label,
	).Scan(&result.ID, &result.TopologyID, &result.SourceAssetID, &result.TargetAssetID,
		&result.Relationship, &result.UserDefined, &result.Label, &result.Deleted)

	if err == nil {
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, err
	}

	// No soft-deleted match — insert new.
	err = s.pool.QueryRow(ctx,
		`INSERT INTO topology_connections (topology_id, source_asset_id, target_asset_id, relationship, user_defined, label)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, topology_id, source_asset_id, target_asset_id, relationship, user_defined, label, deleted`,
		c.TopologyID, c.SourceAssetID, c.TargetAssetID, c.Relationship, c.UserDefined, c.Label,
	).Scan(&result.ID, &result.TopologyID, &result.SourceAssetID, &result.TargetAssetID,
		&result.Relationship, &result.UserDefined, &result.Label, &result.Deleted)
	if err != nil {
		return Connection{}, err
	}
	_ = createdAt // not stored in the Connection struct

	return result, nil
}

func (s *PostgresStore) UpdateConnection(id string, relationship, label string) error {
	ctx := context.Background()

	tag, err := s.pool.Exec(ctx,
		`UPDATE topology_connections
		 SET relationship = COALESCE(NULLIF($2, ''), relationship),
		     label = $3
		 WHERE id = $1 AND deleted = false`,
		id, relationship, label,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteConnection(id string) error {
	ctx := context.Background()

	tag, err := s.pool.Exec(ctx,
		`UPDATE topology_connections SET deleted = true WHERE id = $1 AND deleted = false`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListConnections(topologyID string) ([]Connection, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx,
		`SELECT id, topology_id, source_asset_id, target_asset_id, relationship, user_defined, label, deleted
		 FROM topology_connections
		 WHERE topology_id = $1
		 ORDER BY created_at ASC`,
		topologyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conns := make([]Connection, 0)
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.TopologyID, &c.SourceAssetID, &c.TargetAssetID,
			&c.Relationship, &c.UserDefined, &c.Label, &c.Deleted); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}
