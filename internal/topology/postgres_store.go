package topology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a topology entity is not found.
var ErrNotFound = errors.New("topology: not found")

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore sharing the given connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// compile-time interface check
var _ Store = (*PostgresStore)(nil)

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetOrCreateLayout() (Layout, error) {
	ctx := context.Background()

	var l Layout
	var viewportBytes []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, viewport, created_at, updated_at
		 FROM topology_layouts
		 ORDER BY created_at ASC
		 LIMIT 1`,
	).Scan(&l.ID, &l.Name, &viewportBytes, &l.CreatedAt, &l.UpdatedAt)

	if err == nil {
		vp, vpErr := unmarshalViewport(viewportBytes)
		if vpErr != nil {
			return Layout{}, vpErr
		}
		l.Viewport = vp
		l.CreatedAt = l.CreatedAt.UTC()
		l.UpdatedAt = l.UpdatedAt.UTC()
		return l, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Layout{}, err
	}

	// No layout exists — insert default.
	defaultViewport := Viewport{X: 0, Y: 0, Zoom: 1}
	vpJSON, _ := json.Marshal(defaultViewport)

	err = s.pool.QueryRow(ctx,
		`INSERT INTO topology_layouts (name, viewport)
		 VALUES ($1, $2::jsonb)
		 RETURNING id, name, viewport, created_at, updated_at`,
		"My Homelab", vpJSON,
	).Scan(&l.ID, &l.Name, &viewportBytes, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return Layout{}, err
	}
	vp, vpErr := unmarshalViewport(viewportBytes)
	if vpErr != nil {
		return Layout{}, vpErr
	}
	l.Viewport = vp
	l.CreatedAt = l.CreatedAt.UTC()
	l.UpdatedAt = l.UpdatedAt.UTC()
	return l, nil
}

func (s *PostgresStore) UpdateViewport(viewport Viewport) error {
	ctx := context.Background()
	vpJSON, err := json.Marshal(viewport)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE topology_layouts
		 SET viewport = $1::jsonb, updated_at = $2
		 WHERE id = (SELECT id FROM topology_layouts ORDER BY created_at ASC LIMIT 1)`,
		vpJSON, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Zones
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateZone(z Zone) (Zone, error) {
	ctx := context.Background()

	posJSON, _ := json.Marshal(z.Position)
	sizeJSON, _ := json.Marshal(z.Size)

	var parentZoneID *string
	if z.ParentZoneID != "" {
		parentZoneID = &z.ParentZoneID
	}

	var result Zone
	var posBytes, sizeBytes []byte
	var parentID *string

	err := s.pool.QueryRow(ctx,
		`INSERT INTO topology_zones (topology_id, parent_zone_id, label, color, icon, position, size, collapsed, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9)
		 RETURNING id, topology_id, parent_zone_id, label, color, icon, position, size, collapsed, sort_order`,
		z.TopologyID, parentZoneID, z.Label, z.Color, z.Icon, posJSON, sizeJSON, z.Collapsed, z.SortOrder,
	).Scan(&result.ID, &result.TopologyID, &parentID, &result.Label, &result.Color, &result.Icon,
		&posBytes, &sizeBytes, &result.Collapsed, &result.SortOrder)
	if err != nil {
		return Zone{}, err
	}

	if parentID != nil {
		result.ParentZoneID = *parentID
	}
	pos, posErr := unmarshalPosition(posBytes)
	if posErr != nil {
		return Zone{}, posErr
	}
	result.Position = pos
	sz, szErr := unmarshalSize(sizeBytes)
	if szErr != nil {
		return Zone{}, szErr
	}
	result.Size = sz
	return result, nil
}

func (s *PostgresStore) UpdateZone(z Zone) error {
	ctx := context.Background()

	posJSON, _ := json.Marshal(z.Position)
	sizeJSON, _ := json.Marshal(z.Size)

	var parentZoneID *string
	if z.ParentZoneID != "" {
		parentZoneID = &z.ParentZoneID
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE topology_zones
		 SET parent_zone_id = $2, label = $3, color = $4, icon = $5,
		     position = $6::jsonb, size = $7::jsonb, collapsed = $8, sort_order = $9,
		     updated_at = $10
		 WHERE id = $1`,
		z.ID, parentZoneID, z.Label, z.Color, z.Icon, posJSON, sizeJSON, z.Collapsed, z.SortOrder,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteZone(id string) error {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Get the zone's parent so we can re-parent children.
	var parentID *string
	err = tx.QueryRow(ctx,
		`SELECT parent_zone_id FROM topology_zones WHERE id = $1`,
		id,
	).Scan(&parentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Re-parent child zones to the deleted zone's parent.
	_, err = tx.Exec(ctx,
		`UPDATE topology_zones SET parent_zone_id = $2 WHERE parent_zone_id = $1`,
		id, parentID,
	)
	if err != nil {
		return err
	}

	// Delete the zone (CASCADE handles zone_members).
	tag, err := tx.Exec(ctx,
		`DELETE FROM topology_zones WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) ListZones(topologyID string) ([]Zone, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx,
		`SELECT id, topology_id, parent_zone_id, label, color, icon, position, size, collapsed, sort_order
		 FROM topology_zones
		 WHERE topology_id = $1
		 ORDER BY sort_order ASC, label ASC`,
		topologyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	zones := make([]Zone, 0)
	for rows.Next() {
		z, scanErr := scanZone(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

func (s *PostgresStore) ReorderZones(updates []ZoneReorder) error {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	for _, u := range updates {
		var parentZoneID *string
		if u.ParentZoneID != "" {
			parentZoneID = &u.ParentZoneID
		}
		_, err := tx.Exec(ctx,
			`UPDATE topology_zones SET parent_zone_id = $2, sort_order = $3, updated_at = $4 WHERE id = $1`,
			u.ZoneID, parentZoneID, u.SortOrder, now,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

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

// ---------------------------------------------------------------------------
// Dismissed
// ---------------------------------------------------------------------------

func (s *PostgresStore) DismissAsset(topologyID, assetID string) error {
	ctx := context.Background()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO dismissed_assets (topology_id, asset_id)
		 VALUES ($1, $2)
		 ON CONFLICT (topology_id, asset_id) DO NOTHING`,
		topologyID, assetID,
	)
	return err
}

func (s *PostgresStore) UndismissAsset(topologyID, assetID string) error {
	ctx := context.Background()

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM dismissed_assets WHERE topology_id = $1 AND asset_id = $2`,
		topologyID, assetID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListDismissed(topologyID string) ([]string, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx,
		`SELECT asset_id FROM dismissed_assets WHERE topology_id = $1 ORDER BY dismissed_at ASC`,
		topologyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func (s *PostgresStore) ClearTopology(topologyID string) error {
	ctx := context.Background()

	queries := []string{
		`DELETE FROM zone_members WHERE zone_id IN (SELECT id FROM topology_zones WHERE topology_id = $1)`,
		`DELETE FROM topology_zones WHERE topology_id = $1`,
		`DELETE FROM topology_connections WHERE topology_id = $1`,
		`DELETE FROM dismissed_assets WHERE topology_id = $1`,
	}
	for _, q := range queries {
		if _, err := s.pool.Exec(ctx, q, topologyID); err != nil {
			return fmt.Errorf("clear topology: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanZone(row rowScanner) (Zone, error) {
	var z Zone
	var parentID *string
	var posBytes, sizeBytes []byte

	if err := row.Scan(&z.ID, &z.TopologyID, &parentID, &z.Label, &z.Color, &z.Icon,
		&posBytes, &sizeBytes, &z.Collapsed, &z.SortOrder); err != nil {
		return Zone{}, err
	}
	if parentID != nil {
		z.ParentZoneID = *parentID
	}
	pos, posErr := unmarshalPosition(posBytes)
	if posErr != nil {
		return Zone{}, posErr
	}
	z.Position = pos
	sz, szErr := unmarshalSize(sizeBytes)
	if szErr != nil {
		return Zone{}, szErr
	}
	z.Size = sz
	return z, nil
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

// ---------------------------------------------------------------------------
// JSONB helpers
// ---------------------------------------------------------------------------

func unmarshalViewport(data []byte) (Viewport, error) {
	var v Viewport
	if len(data) > 0 {
		if err := json.Unmarshal(data, &v); err != nil {
			return v, fmt.Errorf("unmarshal viewport: %w", err)
		}
	}
	return v, nil
}

func unmarshalPosition(data []byte) (Position, error) {
	var p Position
	if len(data) > 0 {
		if err := json.Unmarshal(data, &p); err != nil {
			return p, fmt.Errorf("unmarshal position: %w", err)
		}
	}
	return p, nil
}

func unmarshalSize(data []byte) (Size, error) {
	var sz Size
	if len(data) > 0 {
		if err := json.Unmarshal(data, &sz); err != nil {
			return sz, fmt.Errorf("unmarshal size: %w", err)
		}
	}
	return sz, nil
}
