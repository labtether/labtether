package persistence

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/idgen"
)

// groupScanner is satisfied by both pgx.Row and pgx.Rows.
type groupScanner interface {
	Scan(dest ...any) error
}

// scanGroup extracts a groups.Group from a row that returns columns in the
// canonical order: id, name, slug, parent_group_id, icon, sort_order,
// timezone, location, latitude, longitude, metadata, created_at, updated_at.
func scanGroup(row groupScanner) (groups.Group, error) {
	var g groups.Group
	var parentGroupID *string
	var lat *float64
	var lon *float64
	var metadata []byte
	var jumpChain []byte

	if err := row.Scan(
		&g.ID,
		&g.Name,
		&g.Slug,
		&parentGroupID,
		&g.Icon,
		&g.SortOrder,
		&g.Timezone,
		&g.Location,
		&lat,
		&lon,
		&metadata,
		&jumpChain,
		&g.CreatedAt,
		&g.UpdatedAt,
	); err != nil {
		return groups.Group{}, err
	}

	if parentGroupID != nil {
		g.ParentGroupID = *parentGroupID
	}
	g.Latitude = cloneFloatPtr(lat)
	g.Longitude = cloneFloatPtr(lon)
	g.Metadata = unmarshalStringMap(metadata)
	if len(jumpChain) > 0 {
		g.JumpChain = jumpChain
	}
	g.CreatedAt = g.CreatedAt.UTC()
	g.UpdatedAt = g.UpdatedAt.UTC()
	return g, nil
}

// scanGroupWithDepth is like scanGroup but expects an additional trailing
// depth column (used by the recursive CTE in GetGroupTree).
func scanGroupWithDepth(row groupScanner) (groups.Group, int, error) {
	var g groups.Group
	var parentGroupID *string
	var lat *float64
	var lon *float64
	var metadata []byte
	var jumpChain []byte
	var depth int

	if err := row.Scan(
		&g.ID,
		&g.Name,
		&g.Slug,
		&parentGroupID,
		&g.Icon,
		&g.SortOrder,
		&g.Timezone,
		&g.Location,
		&lat,
		&lon,
		&metadata,
		&jumpChain,
		&g.CreatedAt,
		&g.UpdatedAt,
		&depth,
	); err != nil {
		return groups.Group{}, 0, err
	}

	if parentGroupID != nil {
		g.ParentGroupID = *parentGroupID
	}
	g.Latitude = cloneFloatPtr(lat)
	g.Longitude = cloneFloatPtr(lon)
	g.Metadata = unmarshalStringMap(metadata)
	if len(jumpChain) > 0 {
		g.JumpChain = jumpChain
	}
	g.CreatedAt = g.CreatedAt.UTC()
	g.UpdatedAt = g.UpdatedAt.UTC()
	return g, depth, nil
}

// slugRe matches characters that are not lowercase letters, digits, or hyphens.
var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// slugify produces a URL-safe slug from a human-readable name.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "group"
	}
	return s
}

const groupColumns = `id, name, slug, parent_group_id, icon, sort_order,
	timezone, location, latitude, longitude, metadata, jump_chain, created_at, updated_at`

func (s *PostgresStore) CreateGroup(req groups.CreateRequest) (groups.Group, error) {
	now := time.Now().UTC()

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Name)
	}

	metadataPayload, err := marshalStringMap(req.Metadata)
	if err != nil {
		return groups.Group{}, err
	}

	var jumpChainPayload []byte
	if len(req.JumpChain) > 0 {
		jumpChainPayload = req.JumpChain
	}

	return scanGroup(s.pool.QueryRow(context.Background(),
		`INSERT INTO groups (
			id, name, slug, parent_group_id, icon, sort_order,
			timezone, location, latitude, longitude, metadata, jump_chain,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb, $13, $13)
		RETURNING `+groupColumns,
		idgen.New("grp"),
		strings.TrimSpace(req.Name),
		slug,
		nullIfBlank(req.ParentGroupID),
		strings.TrimSpace(req.Icon),
		req.SortOrder,
		strings.TrimSpace(req.Timezone),
		strings.TrimSpace(req.Location),
		nullFloat64(req.Latitude),
		nullFloat64(req.Longitude),
		metadataPayload,
		jumpChainPayload,
		now,
	))
}

func (s *PostgresStore) GetGroup(id string) (groups.Group, bool, error) {
	g, err := scanGroup(s.pool.QueryRow(context.Background(),
		`SELECT `+groupColumns+` FROM groups WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.Group{}, false, nil
		}
		return groups.Group{}, false, err
	}
	return g, true, nil
}

func (s *PostgresStore) ListGroups() ([]groups.Group, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT `+groupColumns+` FROM groups ORDER BY sort_order, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]groups.Group, 0, 32)
	for rows.Next() {
		g, scanErr := scanGroup(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, g)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) GetGroupTree() ([]groups.TreeNode, error) {
	rows, err := s.pool.Query(context.Background(),
		`WITH RECURSIVE group_tree AS (
			SELECT id, name, slug, parent_group_id, icon, sort_order,
			       timezone, location, latitude, longitude, metadata, jump_chain,
			       created_at, updated_at, 0 AS depth
			FROM groups WHERE parent_group_id IS NULL
			UNION ALL
			SELECT g.id, g.name, g.slug, g.parent_group_id, g.icon, g.sort_order,
			       g.timezone, g.location, g.latitude, g.longitude, g.metadata, g.jump_chain,
			       g.created_at, g.updated_at, gt.depth + 1
			FROM groups g JOIN group_tree gt ON g.parent_group_id = gt.id
			WHERE gt.depth < 20
		)
		SELECT * FROM group_tree ORDER BY depth, sort_order, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	flat := make([]groupFlatEntry, 0, 32)
	for rows.Next() {
		g, depth, scanErr := scanGroupWithDepth(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		flat = append(flat, groupFlatEntry{group: g, depth: depth})
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return buildGroupTree(flat), nil
}

// groupFlatEntry holds a group with its depth from a recursive CTE result.
type groupFlatEntry struct {
	group groups.Group
	depth int
}

// treeNodePtr is a pointer-based tree node used during tree construction
// to avoid stale slice element pointers when children slices grow.
type treeNodePtr struct {
	group    groups.Group
	depth    int
	children []*treeNodePtr
}

// toTreeNode recursively converts a pointer-based tree into the value-based
// groups.TreeNode returned by the API.
func (n *treeNodePtr) toTreeNode() groups.TreeNode {
	node := groups.TreeNode{
		Group: n.group,
		Depth: n.depth,
	}
	if len(n.children) > 0 {
		node.Children = make([]groups.TreeNode, len(n.children))
		for i, child := range n.children {
			node.Children[i] = child.toTreeNode()
		}
	}
	return node
}

// buildGroupTree assembles a flat depth-sorted list into a nested TreeNode slice.
// The input must be sorted by depth ascending so that parents appear before children.
func buildGroupTree(flat []groupFlatEntry) []groups.TreeNode {
	nodeMap := make(map[string]*treeNodePtr, len(flat))
	var roots []*treeNodePtr

	for _, entry := range flat {
		node := &treeNodePtr{
			group: entry.group,
			depth: entry.depth,
		}
		nodeMap[entry.group.ID] = node

		if entry.group.ParentGroupID == "" {
			roots = append(roots, node)
		} else if parent, ok := nodeMap[entry.group.ParentGroupID]; ok {
			parent.children = append(parent.children, node)
		}
	}

	out := make([]groups.TreeNode, len(roots))
	for i, r := range roots {
		out[i] = r.toTreeNode()
	}
	return out
}

func (s *PostgresStore) UpdateGroup(id string, req groups.UpdateRequest) (groups.Group, error) {
	id = strings.TrimSpace(id)

	setClauses := make([]string, 0, 10)
	args := make([]any, 0, 12)
	args = append(args, id) // $1 = id
	next := 2

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", next))
		args = append(args, strings.TrimSpace(*req.Name))
		next++
	}
	if req.Slug != nil {
		setClauses = append(setClauses, fmt.Sprintf("slug = $%d", next))
		args = append(args, strings.TrimSpace(*req.Slug))
		next++
	}
	if req.ParentGroupID != nil {
		setClauses = append(setClauses, fmt.Sprintf("parent_group_id = $%d", next))
		args = append(args, nullIfBlank(*req.ParentGroupID))
		next++
	}
	if req.Icon != nil {
		setClauses = append(setClauses, fmt.Sprintf("icon = $%d", next))
		args = append(args, strings.TrimSpace(*req.Icon))
		next++
	}
	if req.SortOrder != nil {
		setClauses = append(setClauses, fmt.Sprintf("sort_order = $%d", next))
		args = append(args, *req.SortOrder)
		next++
	}
	if req.Timezone != nil {
		setClauses = append(setClauses, fmt.Sprintf("timezone = $%d", next))
		args = append(args, strings.TrimSpace(*req.Timezone))
		next++
	}
	if req.Location != nil {
		setClauses = append(setClauses, fmt.Sprintf("location = $%d", next))
		args = append(args, strings.TrimSpace(*req.Location))
		next++
	}
	if req.Latitude != nil {
		setClauses = append(setClauses, fmt.Sprintf("latitude = $%d", next))
		args = append(args, *req.Latitude)
		next++
	}
	if req.Longitude != nil {
		setClauses = append(setClauses, fmt.Sprintf("longitude = $%d", next))
		args = append(args, *req.Longitude)
		next++
	}
	if req.Metadata != nil {
		metadataPayload, err := marshalStringMap(req.Metadata)
		if err != nil {
			return groups.Group{}, err
		}
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d::jsonb", next))
		args = append(args, metadataPayload)
		next++
	}
	if req.JumpChain != nil {
		setClauses = append(setClauses, fmt.Sprintf("jump_chain = $%d::jsonb", next))
		args = append(args, []byte(req.JumpChain))
		next++
	}

	// Always bump updated_at.
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", next))
	args = append(args, time.Now().UTC())

	if len(setClauses) == 1 {
		// Only updated_at — still a valid (no-op content) update, proceed.
	}

	sql := `UPDATE groups SET ` + strings.Join(setClauses, ", ") +
		` WHERE id = $1 RETURNING ` + groupColumns

	g, err := scanGroup(s.pool.QueryRow(context.Background(), sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return groups.Group{}, ErrNotFound
		}
		return groups.Group{}, err
	}
	return g, nil
}

func (s *PostgresStore) DeleteGroup(id string) error {
	id = strings.TrimSpace(id)

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// Look up the group's parent so children can be reparented.
	var parentGroupID *string
	err = tx.QueryRow(context.Background(),
		`SELECT parent_group_id FROM groups WHERE id = $1`, id,
	).Scan(&parentGroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	// Reparent children to the deleted group's parent.
	_, err = tx.Exec(context.Background(),
		`UPDATE groups SET parent_group_id = $2, updated_at = $3
		 WHERE parent_group_id = $1`,
		id, parentGroupID, time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	// Delete the group. Assets with this group_id are handled by ON DELETE SET NULL.
	tag, err := tx.Exec(context.Background(),
		`DELETE FROM groups WHERE id = $1`, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) IsAncestor(candidateAncestorID, descendantID string) (bool, error) {
	candidateAncestorID = strings.TrimSpace(candidateAncestorID)
	descendantID = strings.TrimSpace(descendantID)
	if candidateAncestorID == "" || descendantID == "" {
		return false, nil
	}

	var isAnc bool
	err := s.pool.QueryRow(context.Background(),
		`WITH RECURSIVE ancestors AS (
			SELECT parent_group_id FROM groups WHERE id = $1
			UNION ALL
			SELECT g.parent_group_id FROM groups g JOIN ancestors a ON g.id = a.parent_group_id
			WHERE a.parent_group_id IS NOT NULL
		)
		SELECT EXISTS (SELECT 1 FROM ancestors WHERE parent_group_id = $2)`,
		descendantID,
		candidateAncestorID,
	).Scan(&isAnc)
	if err != nil {
		return false, err
	}
	return isAnc, nil
}
