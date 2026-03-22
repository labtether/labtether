package persistence

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

func (s *PostgresStore) UpsertPresence(p AgentPresence) error {
	ctx := context.Background()
	metaJSON, _ := json.Marshal(p.Metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_presence (asset_id, transport, connected_at, last_heartbeat_at, session_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (asset_id) DO UPDATE SET
			transport = EXCLUDED.transport,
			connected_at = EXCLUDED.connected_at,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			session_id = EXCLUDED.session_id,
			metadata = EXCLUDED.metadata`,
		p.AssetID, p.Transport, p.ConnectedAt, p.LastHeartbeatAt, p.SessionID, metaJSON,
	)
	return err
}

func (s *PostgresStore) UpdateHeartbeat(assetID string, at time.Time) error {
	_, err := s.UpdateHeartbeatForSession(assetID, "", at)
	return err
}

func (s *PostgresStore) UpdateHeartbeatForSession(assetID, sessionID string, at time.Time) (bool, error) {
	ctx := context.Background()
	if sessionID == "" {
		tag, err := s.pool.Exec(ctx,
			`UPDATE agent_presence SET last_heartbeat_at = $2 WHERE asset_id = $1`,
			assetID, at,
		)
		return tag.RowsAffected() > 0, err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_presence SET last_heartbeat_at = $3 WHERE asset_id = $1 AND session_id = $2`,
		assetID, sessionID, at,
	)
	return tag.RowsAffected() > 0, err
}

func (s *PostgresStore) UpdatePresenceMetadata(assetID string, metadata map[string]any) error {
	_, err := s.UpdatePresenceMetadataForSession(assetID, "", metadata)
	return err
}

func (s *PostgresStore) UpdatePresenceMetadataForSession(assetID, sessionID string, metadata map[string]any) (bool, error) {
	ctx := context.Background()
	metaJSON, _ := json.Marshal(metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	if sessionID == "" {
		tag, err := s.pool.Exec(ctx,
			`UPDATE agent_presence SET metadata = $2 WHERE asset_id = $1`,
			assetID, metaJSON,
		)
		return tag.RowsAffected() > 0, err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_presence SET metadata = $3 WHERE asset_id = $1 AND session_id = $2`,
		assetID, sessionID, metaJSON,
	)
	return tag.RowsAffected() > 0, err
}

func (s *PostgresStore) DeletePresence(assetID string) error {
	_, err := s.DeletePresenceForSession(assetID, "")
	return err
}

func (s *PostgresStore) DeletePresenceForSession(assetID, sessionID string) (bool, error) {
	ctx := context.Background()
	if sessionID == "" {
		tag, err := s.pool.Exec(ctx,
			`DELETE FROM agent_presence WHERE asset_id = $1`,
			assetID,
		)
		return tag.RowsAffected() > 0, err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM agent_presence WHERE asset_id = $1 AND session_id = $2`,
		assetID, sessionID,
	)
	return tag.RowsAffected() > 0, err
}

func (s *PostgresStore) ListPresence() ([]AgentPresence, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx,
		`SELECT asset_id, transport, connected_at, last_heartbeat_at, session_id, metadata
		 FROM agent_presence ORDER BY connected_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AgentPresence
	for rows.Next() {
		var p AgentPresence
		var metaJSON []byte
		if err := rows.Scan(&p.AssetID, &p.Transport, &p.ConnectedAt, &p.LastHeartbeatAt, &p.SessionID, &metaJSON); err != nil {
			log.Printf("persistence: presence scan error: %v", err)
			continue
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &p.Metadata)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetStalePresence(olderThan time.Time) ([]AgentPresence, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx,
		`SELECT asset_id, transport, connected_at, last_heartbeat_at, session_id, metadata
		 FROM agent_presence WHERE last_heartbeat_at < $1`,
		olderThan,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AgentPresence
	for rows.Next() {
		var p AgentPresence
		var metaJSON []byte
		if err := rows.Scan(&p.AssetID, &p.Transport, &p.ConnectedAt, &p.LastHeartbeatAt, &p.SessionID, &metaJSON); err != nil {
			log.Printf("persistence: stale presence scan error: %v", err)
			continue
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &p.Metadata)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateAssetTransportType(assetID, transportType string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`UPDATE assets SET transport_type = $2 WHERE id = $1`,
		assetID, transportType,
	)
	return err
}
