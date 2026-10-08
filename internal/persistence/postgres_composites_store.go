package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/edges"
	"strings"
	"time"
)

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
