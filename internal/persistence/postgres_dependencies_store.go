package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

func (s *PostgresStore) CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error) {
	now := time.Now().UTC()
	source := strings.TrimSpace(req.SourceAssetID)
	target := strings.TrimSpace(req.TargetAssetID)
	if source == target {
		return dependencies.Dependency{}, dependencies.ErrSelfReference
	}

	relType := dependencies.NormalizeRelationshipType(req.RelationshipType)
	if relType == "" {
		return dependencies.Dependency{}, errors.New("invalid relationship_type")
	}
	direction := dependencies.NormalizeDirection(req.Direction)
	if direction == "" {
		direction = dependencies.DirectionDownstream
	}
	criticality := dependencies.NormalizeCriticality(req.Criticality)
	if criticality == "" {
		criticality = dependencies.CriticalityMedium
	}

	metadataPayload, err := marshalStringMap(req.Metadata)
	if err != nil {
		return dependencies.Dependency{}, err
	}

	dep, err := scanDependency(s.pool.QueryRow(context.Background(),
		`INSERT INTO asset_dependencies (
			id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $8)
		RETURNING id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, created_at, updated_at`,
		idgen.New("dep"),
		source,
		target,
		relType,
		direction,
		criticality,
		metadataPayload,
		now,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			return dependencies.Dependency{}, dependencies.ErrDuplicateDependency
		}
		return dependencies.Dependency{}, err
	}
	return dep, nil
}

func (s *PostgresStore) ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	assetID = strings.TrimSpace(assetID)

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, created_at, updated_at
		 FROM asset_dependencies
		 WHERE source_asset_id = $1 OR target_asset_id = $1
		 ORDER BY created_at DESC LIMIT $2`,
		assetID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dependencies.Dependency, 0)
	for rows.Next() {
		dep, scanErr := scanDependency(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, dep)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListAssetDependenciesBatch(assetIDs []string, limit int) ([]dependencies.Dependency, error) {
	if limit <= 0 {
		limit = 5000
	}
	if limit > 50000 {
		limit = 50000
	}

	uniqueAssetIDs := make([]string, 0, len(assetIDs))
	seen := make(map[string]struct{}, len(assetIDs))
	for _, raw := range assetIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueAssetIDs = append(uniqueAssetIDs, id)
	}
	if len(uniqueAssetIDs) == 0 {
		return []dependencies.Dependency{}, nil
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, created_at, updated_at
		 FROM asset_dependencies
		 WHERE source_asset_id = ANY($1::text[]) OR target_asset_id = ANY($1::text[])
		 ORDER BY created_at DESC
		 LIMIT $2`,
		uniqueAssetIDs, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dependencies.Dependency, 0)
	for rows.Next() {
		dep, scanErr := scanDependency(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, dep)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetAssetDependency(id string) (dependencies.Dependency, bool, error) {
	dep, err := scanDependency(s.pool.QueryRow(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, created_at, updated_at
		 FROM asset_dependencies WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dependencies.Dependency{}, false, nil
		}
		return dependencies.Dependency{}, false, err
	}
	return dep, true, nil
}

func (s *PostgresStore) DeleteAssetDependency(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM asset_dependencies WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return dependencies.ErrDependencyNotFound
	}
	return nil
}

func (s *PostgresStore) BlastRadius(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	// Cycle detection: track visited asset IDs in a path array.
	// NOT (ad.target_asset_id = ANY(d.visited)) prevents re-visiting nodes
	// that are already in the current traversal path, breaking circular references.
	// The depth < $2 guard provides a hard stop even for very wide non-cyclic graphs.
	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE downstream AS (
			SELECT
				target_asset_id AS asset_id,
				relationship_type,
				criticality,
				1 AS depth,
				ARRAY[source_asset_id, target_asset_id] AS visited
			FROM asset_dependencies
			WHERE source_asset_id = $1 AND direction IN ('downstream', 'bidirectional')
			UNION ALL
			SELECT
				ad.target_asset_id,
				ad.relationship_type,
				ad.criticality,
				d.depth + 1,
				d.visited || ad.target_asset_id
			FROM asset_dependencies ad
			JOIN downstream d ON d.asset_id = ad.source_asset_id
			WHERE d.depth < $2
			  AND ad.direction IN ('downstream', 'bidirectional')
			  AND NOT (ad.target_asset_id = ANY(d.visited))
		)
		SELECT DISTINCT d.asset_id, COALESCE(a.name, ''), d.depth, d.relationship_type, d.criticality
		FROM downstream d LEFT JOIN assets a ON a.id = d.asset_id
		ORDER BY d.depth, d.criticality`,
		strings.TrimSpace(assetID), maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dependencies.ImpactNode, 0, 32)
	for rows.Next() {
		var node dependencies.ImpactNode
		if err := rows.Scan(&node.AssetID, &node.AssetName, &node.Depth, &node.RelationshipType, &node.Criticality); err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpstreamCauses(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	// Cycle detection: track visited asset IDs in a path array.
	// NOT (ad.source_asset_id = ANY(u.visited)) prevents re-visiting nodes
	// that are already in the current traversal path, breaking circular references.
	// The depth < $2 guard provides a hard stop even for very wide non-cyclic graphs.
	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE upstream AS (
			SELECT
				source_asset_id AS asset_id,
				relationship_type,
				criticality,
				1 AS depth,
				ARRAY[target_asset_id, source_asset_id] AS visited
			FROM asset_dependencies
			WHERE target_asset_id = $1 AND direction IN ('upstream', 'bidirectional')
			UNION ALL
			SELECT
				ad.source_asset_id,
				ad.relationship_type,
				ad.criticality,
				u.depth + 1,
				u.visited || ad.source_asset_id
			FROM asset_dependencies ad
			JOIN upstream u ON u.asset_id = ad.target_asset_id
			WHERE u.depth < $2
			  AND ad.direction IN ('upstream', 'bidirectional')
			  AND NOT (ad.source_asset_id = ANY(u.visited))
		)
		SELECT DISTINCT u.asset_id, COALESCE(a.name, ''), u.depth, u.relationship_type, u.criticality
		FROM upstream u LEFT JOIN assets a ON a.id = u.asset_id
		ORDER BY u.depth, u.criticality`,
		strings.TrimSpace(assetID), maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]dependencies.ImpactNode, 0, 32)
	for rows.Next() {
		var node dependencies.ImpactNode
		if err := rows.Scan(&node.AssetID, &node.AssetName, &node.Depth, &node.RelationshipType, &node.Criticality); err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s *PostgresStore) LinkIncidentAsset(incidentID string, req incidents.LinkAssetRequest) (incidents.IncidentAsset, error) {
	incidentID = strings.TrimSpace(incidentID)
	assetID := strings.TrimSpace(req.AssetID)
	role := incidents.NormalizeAssetRole(req.Role)
	if role == "" {
		return incidents.IncidentAsset{}, errors.New("invalid asset role")
	}

	var incidentExists bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`,
		incidentID,
	).Scan(&incidentExists); err != nil {
		return incidents.IncidentAsset{}, err
	}
	if !incidentExists {
		return incidents.IncidentAsset{}, incidents.ErrIncidentNotFound
	}

	now := time.Now().UTC()
	ia, err := scanIncidentAsset(s.pool.QueryRow(context.Background(),
		`INSERT INTO incident_assets (id, incident_id, asset_id, role, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, incident_id, asset_id, role, created_at`,
		idgen.New("ia"),
		incidentID,
		assetID,
		role,
		now,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			return incidents.IncidentAsset{}, incidents.ErrIncidentAssetConflict
		}
		return incidents.IncidentAsset{}, err
	}
	return ia, nil
}

func (s *PostgresStore) ListIncidentAssets(incidentID string, limit int) ([]incidents.IncidentAsset, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	incidentID = strings.TrimSpace(incidentID)

	var incidentExists bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`,
		incidentID,
	).Scan(&incidentExists); err != nil {
		return nil, err
	}
	if !incidentExists {
		return nil, incidents.ErrIncidentNotFound
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, incident_id, asset_id, role, created_at
		 FROM incident_assets
		 WHERE incident_id = $1
		 ORDER BY created_at DESC LIMIT $2`,
		incidentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]incidents.IncidentAsset, 0)
	for rows.Next() {
		ia, scanErr := scanIncidentAsset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, ia)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UnlinkIncidentAsset(incidentID, linkID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM incident_assets WHERE id = $1 AND incident_id = $2`,
		strings.TrimSpace(linkID),
		strings.TrimSpace(incidentID),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
