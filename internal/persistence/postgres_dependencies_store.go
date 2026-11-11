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

// impactLinksSQL lists confirmed links as cause and affected device: when the
// cause goes down, the affected device is hit. runs_on, hosted_on and
// depends_on point from the device that needs the other (a container runs on
// its host, a service depends on its database), so impact flows from target to
// source; contains, provides_to and connected_to keep their source-to-target
// reading. A bidirectional link carries impact both ways. Suggested and
// dismissed links are not confirmed and carry none.
const impactLinksSQL = `
			SELECT target_asset_id AS cause_id, source_asset_id AS affected_id, relationship_type, criticality
			FROM asset_edges
			WHERE relationship_type IN ('runs_on', 'hosted_on', 'depends_on')
			  AND origin NOT IN ('suggested', 'dismissed')
			UNION ALL
			SELECT source_asset_id, target_asset_id, relationship_type, criticality
			FROM asset_edges
			WHERE relationship_type NOT IN ('runs_on', 'hosted_on', 'depends_on')
			  AND origin NOT IN ('suggested', 'dismissed')
			UNION ALL
			SELECT source_asset_id, target_asset_id, relationship_type, criticality
			FROM asset_edges
			WHERE relationship_type IN ('runs_on', 'hosted_on', 'depends_on') AND direction = 'bidirectional'
			  AND origin NOT IN ('suggested', 'dismissed')
			UNION ALL
			SELECT target_asset_id, source_asset_id, relationship_type, criticality
			FROM asset_edges
			WHERE relationship_type NOT IN ('runs_on', 'hosted_on', 'depends_on') AND direction = 'bidirectional'
			  AND origin NOT IN ('suggested', 'dismissed')`

// BlastRadius lists the devices hit when assetID goes down, each once at its
// nearest depth.
func (s *PostgresStore) BlastRadius(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return s.impactWalk(`WITH RECURSIVE links AS (`+impactLinksSQL+`),
		walk AS (
			SELECT affected_id AS asset_id, relationship_type, criticality, 1 AS depth, ARRAY[cause_id, affected_id] AS visited
			FROM links WHERE cause_id = $1
			UNION ALL
			SELECT l.affected_id, l.relationship_type, l.criticality, w.depth + 1, w.visited || l.affected_id
			FROM links l JOIN walk w ON l.cause_id = w.asset_id
			WHERE w.depth < $2 AND NOT (l.affected_id = ANY(w.visited))
		)`+impactNearestSQL, assetID, maxDepth)
}

// UpstreamCauses lists the devices whose failure hits assetID, each once at
// its nearest depth.
func (s *PostgresStore) UpstreamCauses(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return s.impactWalk(`WITH RECURSIVE links AS (`+impactLinksSQL+`),
		walk AS (
			SELECT cause_id AS asset_id, relationship_type, criticality, 1 AS depth, ARRAY[affected_id, cause_id] AS visited
			FROM links WHERE affected_id = $1
			UNION ALL
			SELECT l.cause_id, l.relationship_type, l.criticality, w.depth + 1, w.visited || l.cause_id
			FROM links l JOIN walk w ON l.affected_id = w.asset_id
			WHERE w.depth < $2 AND NOT (l.cause_id = ANY(w.visited))
		)`+impactNearestSQL, assetID, maxDepth)
}

// impactNearestSQL keeps each device once, at the depth it is first reached.
const impactNearestSQL = `,
		nearest AS (
			SELECT DISTINCT ON (asset_id) asset_id, relationship_type, criticality, depth
			FROM walk ORDER BY asset_id, depth, relationship_type
		)
		SELECT n.asset_id, COALESCE(a.name, ''), n.depth, n.relationship_type, n.criticality
		FROM nearest n LEFT JOIN assets a ON a.id = n.asset_id
		ORDER BY n.depth, n.criticality, n.asset_id`

func (s *PostgresStore) impactWalk(query, assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}
	rows, err := s.pool.Query(context.Background(), query, strings.TrimSpace(assetID), maxDepth)
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
