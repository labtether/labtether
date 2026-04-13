package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/idgen"
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

func (s *PostgresStore) Descendants(rootAssetID string, maxDepth int) ([]edges.TreeNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE tree AS (
			SELECT target_asset_id AS id, 1 AS depth, ARRAY[source_asset_id] AS visited
			FROM asset_edges
			WHERE source_asset_id = $1
			  AND relationship_type IN ('contains', 'runs_on', 'hosted_on')
			  AND origin NOT IN ('suggested', 'dismissed')
			UNION ALL
			SELECT e.target_asset_id, t.depth + 1, t.visited || e.source_asset_id
			FROM asset_edges e
			JOIN tree t ON e.source_asset_id = t.id
			WHERE e.relationship_type IN ('contains', 'runs_on', 'hosted_on')
			  AND e.origin NOT IN ('suggested', 'dismissed')
			  AND t.depth < $2
			  AND NOT (e.target_asset_id = ANY(t.visited))
		)
		SELECT id, depth FROM tree`,
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
		`WITH RECURSIVE ancestors AS (
			SELECT source_asset_id AS id, 1 AS depth, ARRAY[target_asset_id] AS visited
			FROM asset_edges
			WHERE target_asset_id = $1
			  AND relationship_type IN ('contains', 'runs_on', 'hosted_on')
			  AND origin NOT IN ('suggested', 'dismissed')
			UNION ALL
			SELECT e.source_asset_id, a.depth + 1, a.visited || e.target_asset_id
			FROM asset_edges e
			JOIN ancestors a ON e.target_asset_id = a.id
			WHERE e.relationship_type IN ('contains', 'runs_on', 'hosted_on')
			  AND e.origin NOT IN ('suggested', 'dismissed')
			  AND a.depth < $2
			  AND NOT (e.source_asset_id = ANY(a.visited))
		)
		SELECT id, depth FROM ancestors ORDER BY depth DESC`,
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
// Composite CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateComposite(req edges.CreateCompositeRequest) (edges.Composite, error) {
	primaryID := strings.TrimSpace(req.PrimaryAssetID)
	if primaryID == "" {
		return edges.Composite{}, errors.New("primary_asset_id is required")
	}
	if len(req.FacetAssetIDs) == 0 {
		return edges.Composite{}, errors.New("at least one facet asset ID is required")
	}

	now := time.Now().UTC()

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return edges.Composite{}, err
	}
	defer tx.Rollback(context.Background())

	// Insert primary member
	_, err = tx.Exec(context.Background(),
		`INSERT INTO asset_composites (composite_id, member_asset_id, role, created_at)
		 VALUES ($1, $2, 'primary', $3)`,
		primaryID, primaryID, now,
	)
	if err != nil {
		return edges.Composite{}, err
	}

	// Insert facet members
	for _, facetID := range req.FacetAssetIDs {
		fid := strings.TrimSpace(facetID)
		if fid == "" || fid == primaryID {
			continue
		}
		_, err = tx.Exec(context.Background(),
			`INSERT INTO asset_composites (composite_id, member_asset_id, role, created_at)
			 VALUES ($1, $2, 'facet', $3)`,
			primaryID, fid, now,
		)
		if err != nil {
			return edges.Composite{}, err
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return edges.Composite{}, err
	}

	return s.getCompositeByID(primaryID)
}

func (s *PostgresStore) GetComposite(compositeID string) (edges.Composite, bool, error) {
	compositeID = strings.TrimSpace(compositeID)
	comp, err := s.getCompositeByID(compositeID)
	if err != nil {
		return edges.Composite{}, false, err
	}
	if len(comp.Members) == 0 {
		return edges.Composite{}, false, nil
	}
	return comp, true, nil
}

func (s *PostgresStore) getCompositeByID(compositeID string) (edges.Composite, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT composite_id, member_asset_id, role, created_at
		 FROM asset_composites
		 WHERE composite_id = $1
		 ORDER BY role ASC, created_at ASC`,
		compositeID,
	)
	if err != nil {
		return edges.Composite{}, err
	}
	defer rows.Close()

	comp := edges.Composite{CompositeID: compositeID}
	for rows.Next() {
		var cid string
		var m edges.CompositeMember
		if err := rows.Scan(&cid, &m.AssetID, &m.Role, &m.CreatedAt); err != nil {
			return edges.Composite{}, err
		}
		m.CreatedAt = m.CreatedAt.UTC()
		comp.Members = append(comp.Members, m)
	}
	return comp, rows.Err()
}

func (s *PostgresStore) ChangePrimary(compositeID, newPrimaryAssetID string) error {
	compositeID = strings.TrimSpace(compositeID)
	newPrimaryAssetID = strings.TrimSpace(newPrimaryAssetID)

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// Verify the composite exists and the new primary is a member
	var memberExists bool
	err = tx.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM asset_composites WHERE composite_id = $1 AND member_asset_id = $2)`,
		compositeID, newPrimaryAssetID,
	).Scan(&memberExists)
	if err != nil {
		return err
	}
	if !memberExists {
		return ErrNotFound
	}

	// Demote old primary to facet
	_, err = tx.Exec(context.Background(),
		`UPDATE asset_composites SET role = 'facet' WHERE composite_id = $1 AND role = 'primary'`,
		compositeID,
	)
	if err != nil {
		return err
	}

	// Promote new primary
	_, err = tx.Exec(context.Background(),
		`UPDATE asset_composites SET role = 'primary' WHERE composite_id = $1 AND member_asset_id = $2`,
		compositeID, newPrimaryAssetID,
	)
	if err != nil {
		return err
	}

	// Update composite_id to new primary's asset ID for all members
	_, err = tx.Exec(context.Background(),
		`UPDATE asset_composites SET composite_id = $2 WHERE composite_id = $1`,
		compositeID, newPrimaryAssetID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) DetachMember(compositeID, memberAssetID string) error {
	compositeID = strings.TrimSpace(compositeID)
	memberAssetID = strings.TrimSpace(memberAssetID)

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// Delete the member
	tag, err := tx.Exec(context.Background(),
		`DELETE FROM asset_composites WHERE composite_id = $1 AND member_asset_id = $2`,
		compositeID, memberAssetID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	// Check remaining member count
	var remaining int
	err = tx.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM asset_composites WHERE composite_id = $1`,
		compositeID,
	).Scan(&remaining)
	if err != nil {
		return err
	}

	// If only one member remains, dissolve the composite
	if remaining <= 1 {
		_, err = tx.Exec(context.Background(),
			`DELETE FROM asset_composites WHERE composite_id = $1`,
			compositeID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) ListCompositesByAssets(assetIDs []string) ([]edges.Composite, error) {
	uniqueIDs := dedupeStrings(assetIDs)
	if len(uniqueIDs) == 0 {
		return []edges.Composite{}, nil
	}

	// Find all composite_ids that contain at least one of the given assets
	rows, err := s.pool.Query(context.Background(),
		`SELECT DISTINCT c.composite_id, c.member_asset_id, c.role, c.created_at
		 FROM asset_composites c
		 WHERE c.composite_id IN (
			SELECT composite_id FROM asset_composites WHERE member_asset_id = ANY($1::text[])
		 )
		 ORDER BY c.composite_id, c.role ASC, c.created_at ASC`,
		uniqueIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	compositeMap := make(map[string]*edges.Composite)
	var order []string
	for rows.Next() {
		var cid string
		var m edges.CompositeMember
		if err := rows.Scan(&cid, &m.AssetID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt = m.CreatedAt.UTC()
		if _, exists := compositeMap[cid]; !exists {
			compositeMap[cid] = &edges.Composite{CompositeID: cid}
			order = append(order, cid)
		}
		compositeMap[cid].Members = append(compositeMap[cid].Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]edges.Composite, 0, len(order))
	for _, cid := range order {
		out = append(out, *compositeMap[cid])
	}
	return out, nil
}

func (s *PostgresStore) ResolveCompositeID(assetID string) (string, bool, error) {
	var compositeID string
	err := s.pool.QueryRow(context.Background(),
		`SELECT composite_id FROM asset_composites WHERE member_asset_id = $1`,
		strings.TrimSpace(assetID),
	).Scan(&compositeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return compositeID, true, nil
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
