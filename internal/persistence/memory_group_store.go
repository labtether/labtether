package persistence

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/idgen"
)

// MemoryGroupStore is an in-memory GroupStore for unit tests.
type MemoryGroupStore struct {
	mu     sync.RWMutex
	groups map[string]groups.Group
}

// NewMemoryGroupStore returns an empty MemoryGroupStore.
func NewMemoryGroupStore() *MemoryGroupStore {
	return &MemoryGroupStore{
		groups: make(map[string]groups.Group),
	}
}

func (m *MemoryGroupStore) CreateGroup(req groups.CreateRequest) (groups.Group, error) {
	now := time.Now().UTC()

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Name)
	}

	g := groups.Group{
		ID:            idgen.New("grp"),
		Name:          strings.TrimSpace(req.Name),
		Slug:          slug,
		ParentGroupID: strings.TrimSpace(req.ParentGroupID),
		Icon:          strings.TrimSpace(req.Icon),
		SortOrder:     req.SortOrder,
		Timezone:      strings.TrimSpace(req.Timezone),
		Location:      strings.TrimSpace(req.Location),
		Latitude:      cloneFloatPtr(req.Latitude),
		Longitude:     cloneFloatPtr(req.Longitude),
		Metadata:      cloneMetadata(req.Metadata),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	m.mu.Lock()
	m.groups[g.ID] = g
	m.mu.Unlock()

	return cloneGroup(g), nil
}

func (m *MemoryGroupStore) GetGroup(id string) (groups.Group, bool, error) {
	m.mu.RLock()
	g, ok := m.groups[strings.TrimSpace(id)]
	m.mu.RUnlock()
	if !ok {
		return groups.Group{}, false, nil
	}
	return cloneGroup(g), true, nil
}

func (m *MemoryGroupStore) ListGroups() ([]groups.Group, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]groups.Group, 0, len(m.groups))
	for _, g := range m.groups {
		out = append(out, cloneGroup(g))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func (m *MemoryGroupStore) GetGroupTree() ([]groups.TreeNode, error) {
	all, err := m.ListGroups()
	if err != nil {
		return nil, err
	}

	// Build flat entries with computed depth, then use buildGroupTree.
	// First compute depth per group by walking parents.
	depthOf := make(map[string]int, len(all))
	byID := make(map[string]groups.Group, len(all))
	for _, g := range all {
		byID[g.ID] = g
	}
	var computeDepth func(id string) int
	computeDepth = func(id string) int {
		if d, ok := depthOf[id]; ok {
			return d
		}
		g, ok := byID[id]
		if !ok || g.ParentGroupID == "" {
			depthOf[id] = 0
			return 0
		}
		d := computeDepth(g.ParentGroupID) + 1
		depthOf[id] = d
		return d
	}
	for _, g := range all {
		computeDepth(g.ID)
	}

	flat := make([]groupFlatEntry, 0, len(all))
	for _, g := range all {
		flat = append(flat, groupFlatEntry{group: g, depth: depthOf[g.ID]})
	}
	sort.Slice(flat, func(i, j int) bool {
		if flat[i].depth != flat[j].depth {
			return flat[i].depth < flat[j].depth
		}
		if flat[i].group.SortOrder != flat[j].group.SortOrder {
			return flat[i].group.SortOrder < flat[j].group.SortOrder
		}
		return flat[i].group.Name < flat[j].group.Name
	})

	return buildGroupTree(flat), nil
}

func (m *MemoryGroupStore) UpdateGroup(id string, req groups.UpdateRequest) (groups.Group, error) {
	now := time.Now().UTC()
	id = strings.TrimSpace(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.groups[id]
	if !ok {
		return groups.Group{}, ErrNotFound
	}

	if req.Name != nil {
		g.Name = strings.TrimSpace(*req.Name)
	}
	if req.Slug != nil {
		g.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.ParentGroupID != nil {
		g.ParentGroupID = strings.TrimSpace(*req.ParentGroupID)
	}
	if req.Icon != nil {
		g.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Timezone != nil {
		g.Timezone = strings.TrimSpace(*req.Timezone)
	}
	if req.Location != nil {
		g.Location = strings.TrimSpace(*req.Location)
	}
	if req.Latitude != nil {
		g.Latitude = cloneFloatPtr(req.Latitude)
	}
	if req.Longitude != nil {
		g.Longitude = cloneFloatPtr(req.Longitude)
	}
	if req.Metadata != nil {
		g.Metadata = cloneMetadata(req.Metadata)
	}

	g.UpdatedAt = now
	m.groups[id] = g
	return cloneGroup(g), nil
}

func (m *MemoryGroupStore) DeleteGroup(id string) error {
	id = strings.TrimSpace(id)
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	g, ok := m.groups[id]
	if !ok {
		return ErrNotFound
	}

	// Reparent children to the deleted group's parent.
	for childID, child := range m.groups {
		if child.ParentGroupID == id {
			child.ParentGroupID = g.ParentGroupID
			child.UpdatedAt = now
			m.groups[childID] = child
		}
	}

	delete(m.groups, id)
	return nil
}

func (m *MemoryGroupStore) IsAncestor(candidateAncestorID, descendantID string) (bool, error) {
	candidateAncestorID = strings.TrimSpace(candidateAncestorID)
	descendantID = strings.TrimSpace(descendantID)
	if candidateAncestorID == "" || descendantID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Walk up from descendant checking for the candidate ancestor.
	visited := make(map[string]bool, 8)
	current := descendantID
	for {
		g, ok := m.groups[current]
		if !ok {
			return false, nil
		}
		parent := g.ParentGroupID
		if parent == "" {
			return false, nil
		}
		if parent == candidateAncestorID {
			return true, nil
		}
		if visited[parent] {
			return false, nil // cycle guard
		}
		visited[parent] = true
		current = parent
	}
}

func cloneGroup(input groups.Group) groups.Group {
	out := input
	out.Latitude = cloneFloatPtr(input.Latitude)
	out.Longitude = cloneFloatPtr(input.Longitude)
	out.Metadata = cloneMetadata(input.Metadata)
	return out
}
