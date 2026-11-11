package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/idgen"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Edge CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateEdge(req edges.CreateEdgeRequest) (edges.Edge, error) {
	now := time.Now().UTC()
	source := strings.TrimSpace(req.SourceAssetID)
	target := strings.TrimSpace(req.TargetAssetID)
	if source == target {
		return edges.Edge{}, errors.New("source and target asset IDs must differ")
	}

	relType := strings.TrimSpace(req.RelationshipType)
	if relType == "" {
		return edges.Edge{}, errors.New("invalid relationship_type")
	}
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = edges.DirDownstream
	}
	criticality := strings.TrimSpace(req.Criticality)
	if criticality == "" {
		criticality = edges.CritMedium
	}
	origin := edges.NormalizeOrigin(req.Origin)
	confidence := req.Confidence
	if confidence <= 0 {
		confidence = 1.0
	}

	metadataPayload, err := marshalStringMap(req.Metadata)
	if err != nil {
		return edges.Edge{}, err
	}
	signalsPayload, err := marshalAnyMap(req.MatchSignals)
	if err != nil {
		return edges.Edge{}, err
	}

	edge, err := scanEdge(s.pool.QueryRow(context.Background(),
		`INSERT INTO asset_edges (
			id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10::jsonb, $11, $11)
		RETURNING id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at`,
		idgen.New("edge"),
		source,
		target,
		relType,
		direction,
		criticality,
		metadataPayload,
		origin,
		confidence,
		signalsPayload,
		now,
	))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") {
			return edges.Edge{}, errors.New("duplicate edge")
		}
		return edges.Edge{}, err
	}
	return edge, nil
}

func (s *PostgresStore) GetEdge(id string) (edges.Edge, bool, error) {
	edge, err := scanEdge(s.pool.QueryRow(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at
		 FROM asset_edges WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return edges.Edge{}, false, nil
		}
		return edges.Edge{}, false, err
	}
	return edge, true, nil
}

func (s *PostgresStore) UpdateEdge(id string, relType, criticality string) error {
	relType = strings.TrimSpace(relType)
	criticality = strings.TrimSpace(criticality)
	if relType == "" && criticality == "" {
		return errors.New("nothing to update")
	}

	now := time.Now().UTC()
	idTrimmed := strings.TrimSpace(id)

	// Use COALESCE with NULLIF to conditionally update only non-empty values.
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE asset_edges
		 SET relationship_type = COALESCE(NULLIF($2, ''), relationship_type),
		     criticality = COALESCE(NULLIF($3, ''), criticality),
		     updated_at = $4
		 WHERE id = $1`,
		idTrimmed, relType, criticality, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteEdge(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM asset_edges WHERE id = $1`,
		strings.TrimSpace(id),
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
// Edge queries
// ---------------------------------------------------------------------------

func (s *PostgresStore) ListEdgesByAsset(assetID string, limit int) ([]edges.Edge, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	assetID = strings.TrimSpace(assetID)

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at
		 FROM asset_edges
		 WHERE source_asset_id = $1 OR target_asset_id = $1
		 ORDER BY created_at DESC LIMIT $2`,
		assetID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]edges.Edge, 0)
	for rows.Next() {
		e, scanErr := scanEdge(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListEdgesBatch(assetIDs []string, limit int) ([]edges.Edge, error) {
	if limit <= 0 {
		limit = 5000
	}
	if limit > 50000 {
		limit = 50000
	}

	uniqueAssetIDs := dedupeStrings(assetIDs)
	if len(uniqueAssetIDs) == 0 {
		return []edges.Edge{}, nil
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at
		 FROM asset_edges
		 WHERE source_asset_id = ANY($1::text[]) OR target_asset_id = ANY($1::text[])
		 ORDER BY created_at DESC
		 LIMIT $2`,
		uniqueAssetIDs, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]edges.Edge, 0)
	for rows.Next() {
		e, scanErr := scanEdge(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Graph traversal
// ---------------------------------------------------------------------------

// containmentLinksSQL lists confirmed containment links as parent and child.
// "contains" points from the parent to the child; "runs_on" and "hosted_on"
// point from the child to its host (a container runs on its Docker host).
// Reading them all as parent-to-child put a host under its own containers.
const containmentLinksSQL = `
			SELECT source_asset_id AS parent_id, target_asset_id AS child_id
			FROM asset_edges
			WHERE relationship_type = 'contains' AND origin NOT IN ('suggested', 'dismissed')
			UNION
			SELECT target_asset_id, source_asset_id
			FROM asset_edges
			WHERE relationship_type IN ('runs_on', 'hosted_on') AND origin NOT IN ('suggested', 'dismissed')`

func (s *PostgresStore) Descendants(rootAssetID string, maxDepth int) ([]edges.TreeNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE links AS (`+containmentLinksSQL+`),
		tree AS (
			SELECT child_id AS id, 1 AS depth, ARRAY[parent_id, child_id] AS visited
			FROM links
			WHERE parent_id = $1
			UNION ALL
			SELECT l.child_id, t.depth + 1, t.visited || l.child_id
			FROM links l
			JOIN tree t ON l.parent_id = t.id
			WHERE t.depth < $2
			  AND NOT (l.child_id = ANY(t.visited))
		)
		SELECT id, MIN(depth) AS depth FROM tree GROUP BY id ORDER BY depth, id`,
		strings.TrimSpace(rootAssetID), maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]edges.TreeNode, 0, 32)
	for rows.Next() {
		var node edges.TreeNode
		if err := rows.Scan(&node.AssetID, &node.Depth); err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s *PostgresStore) Ancestors(assetID string, maxDepth int) ([]edges.TreeNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE links AS (`+containmentLinksSQL+`),
		ancestors AS (
			SELECT parent_id AS id, 1 AS depth, ARRAY[child_id, parent_id] AS visited
			FROM links
			WHERE child_id = $1
			UNION ALL
			SELECT l.parent_id, a.depth + 1, a.visited || l.parent_id
			FROM links l
			JOIN ancestors a ON l.child_id = a.id
			WHERE a.depth < $2
			  AND NOT (l.parent_id = ANY(a.visited))
		)
		SELECT id, MAX(depth) AS depth FROM ancestors GROUP BY id ORDER BY depth DESC, id`,
		strings.TrimSpace(assetID), maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]edges.TreeNode, 0, 32)
	for rows.Next() {
		var node edges.TreeNode
		if err := rows.Scan(&node.AssetID, &node.Depth); err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Proposals
// ---------------------------------------------------------------------------

func (s *PostgresStore) ListProposals() ([]edges.Edge, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, source_asset_id, target_asset_id, relationship_type,
			direction, criticality, metadata, origin, confidence,
			match_signals, created_at, updated_at
		 FROM asset_edges
		 WHERE origin = 'suggested'
		 ORDER BY confidence DESC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]edges.Edge, 0, 32)
	for rows.Next() {
		e, scanErr := scanEdge(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) AcceptProposal(edgeID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE asset_edges SET origin = 'manual', updated_at = $2 WHERE id = $1 AND origin = 'suggested'`,
		strings.TrimSpace(edgeID), time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DismissProposal(edgeID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE asset_edges SET origin = 'dismissed', updated_at = $2 WHERE id = $1 AND origin = 'suggested'`,
		strings.TrimSpace(edgeID), time.Now().UTC(),
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
// scan helpers
// ---------------------------------------------------------------------------

type edgeScanner interface {
	Scan(dest ...any) error
}

func scanEdge(row edgeScanner) (edges.Edge, error) {
	var e edges.Edge
	var metadata []byte
	var matchSignals []byte
	if err := row.Scan(
		&e.ID,
		&e.SourceAssetID,
		&e.TargetAssetID,
		&e.RelationshipType,
		&e.Direction,
		&e.Criticality,
		&metadata,
		&e.Origin,
		&e.Confidence,
		&matchSignals,
		&e.CreatedAt,
		&e.UpdatedAt,
	); err != nil {
		return edges.Edge{}, err
	}
	e.Metadata = unmarshalStringMap(metadata)
	e.MatchSignals = unmarshalAnyMap(matchSignals)
	e.CreatedAt = e.CreatedAt.UTC()
	e.UpdatedAt = e.UpdatedAt.UTC()
	return e, nil
}

// dedupeStrings returns a deduplicated, trimmed copy of the input slice.
func dedupeStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
