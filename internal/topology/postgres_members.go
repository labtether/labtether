package topology

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

func (s *PostgresStore) SetMembers(zoneID string, members []ZoneMember) error {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Collect asset IDs for the new member set.
	assetIDs := make([]string, len(members))
	for i, m := range members {
		assetIDs[i] = m.AssetID
	}

	if len(assetIDs) > 0 {
		// Remove any existing memberships for these assets (enforces single-zone constraint).
		_, err = tx.Exec(ctx,
			`DELETE FROM zone_members WHERE asset_id = ANY($1::text[])`,
			assetIDs,
		)
		if err != nil {
			return err
		}
	}

	// Clear old members of this zone.
	_, err = tx.Exec(ctx,
		`DELETE FROM zone_members WHERE zone_id = $1`,
		zoneID,
	)
	if err != nil {
		return err
	}

	// Insert new member set.
	for _, m := range members {
		posJSON, _ := json.Marshal(m.Position)
		_, err = tx.Exec(ctx,
			`INSERT INTO zone_members (zone_id, asset_id, position, sort_order)
			 VALUES ($1, $2, $3::jsonb, $4)`,
			zoneID, m.AssetID, posJSON, m.SortOrder,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) RemoveMember(assetID string) error {
	ctx := context.Background()

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM zone_members WHERE asset_id = $1`,
		assetID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListMembers(topologyID string) ([]ZoneMember, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx,
		`SELECT zm.zone_id, zm.asset_id, zm.position, zm.sort_order
		 FROM zone_members zm
		 JOIN topology_zones tz ON tz.id = zm.zone_id
		 WHERE tz.topology_id = $1
		 ORDER BY zm.sort_order ASC`,
		topologyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]ZoneMember, 0)
	for rows.Next() {
		m, scanErr := scanMember(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func scanMember(row rowScanner) (ZoneMember, error) {
	var m ZoneMember
	var posBytes []byte

	if err := row.Scan(&m.ZoneID, &m.AssetID, &posBytes, &m.SortOrder); err != nil {
		return ZoneMember{}, err
	}
	pos, posErr := unmarshalPosition(posBytes)
	if posErr != nil {
		return ZoneMember{}, posErr
	}
	m.Position = pos
	return m, nil
}
