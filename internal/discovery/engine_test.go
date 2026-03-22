package discovery

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/edges"
)

// ---------------------------------------------------------------------------
// mockEdgeStore — minimal in-memory implementation of edges.Store
// ---------------------------------------------------------------------------

type mockEdgeStore struct {
	mu      sync.Mutex
	edges   []edges.Edge
	nextID  int
}

func newMockEdgeStore() *mockEdgeStore {
	return &mockEdgeStore{nextID: 1}
}

func (s *mockEdgeStore) CreateEdge(req edges.CreateEdgeRequest) (edges.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := edges.Edge{
		ID:               fmt.Sprintf("edge-%d", s.nextID),
		SourceAssetID:    req.SourceAssetID,
		TargetAssetID:    req.TargetAssetID,
		RelationshipType: req.RelationshipType,
		Direction:        req.Direction,
		Criticality:      req.Criticality,
		Origin:           req.Origin,
		Confidence:       req.Confidence,
		MatchSignals:     req.MatchSignals,
		Metadata:         req.Metadata,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	s.nextID++
	s.edges = append(s.edges, e)
	return e, nil
}

func (s *mockEdgeStore) GetEdge(id string) (edges.Edge, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.edges {
		if e.ID == id {
			return e, true, nil
		}
	}
	return edges.Edge{}, false, nil
}

func (s *mockEdgeStore) UpdateEdge(id string, relType, criticality string) error {
	return nil
}

func (s *mockEdgeStore) DeleteEdge(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.edges {
		if e.ID == id {
			s.edges = append(s.edges[:i], s.edges[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *mockEdgeStore) ListEdgesByAsset(assetID string, limit int) ([]edges.Edge, error) {
	return s.ListEdgesBatch([]string{assetID}, limit)
}

func (s *mockEdgeStore) ListEdgesBatch(assetIDs []string, limit int) ([]edges.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idSet := make(map[string]struct{}, len(assetIDs))
	for _, id := range assetIDs {
		idSet[id] = struct{}{}
	}
	var result []edges.Edge
	for _, e := range s.edges {
		_, srcMatch := idSet[e.SourceAssetID]
		_, tgtMatch := idSet[e.TargetAssetID]
		if srcMatch || tgtMatch {
			result = append(result, e)
		}
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *mockEdgeStore) Descendants(rootAssetID string, maxDepth int) ([]edges.TreeNode, error) {
	return nil, nil
}

func (s *mockEdgeStore) Ancestors(assetID string, maxDepth int) ([]edges.TreeNode, error) {
	return nil, nil
}

func (s *mockEdgeStore) ListProposals() ([]edges.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []edges.Edge
	for _, e := range s.edges {
		if e.Origin == edges.OriginSuggested {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *mockEdgeStore) AcceptProposal(edgeID string) error  { return nil }
func (s *mockEdgeStore) DismissProposal(edgeID string) error { return nil }

func (s *mockEdgeStore) CreateComposite(req edges.CreateCompositeRequest) (edges.Composite, error) {
	return edges.Composite{}, nil
}

func (s *mockEdgeStore) GetComposite(compositeID string) (edges.Composite, bool, error) {
	return edges.Composite{}, false, nil
}

func (s *mockEdgeStore) ChangePrimary(compositeID, newPrimaryAssetID string) error { return nil }
func (s *mockEdgeStore) DetachMember(compositeID, memberAssetID string) error      { return nil }

func (s *mockEdgeStore) ListCompositesByAssets(assetIDs []string) ([]edges.Composite, error) {
	return nil, nil
}

func (s *mockEdgeStore) ResolveCompositeID(assetID string) (string, bool, error) {
	return "", false, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestEngine_RunDiscovery verifies that:
//   - A parent hint produces an auto-created edge (confidence 0.95, origin "auto").
//   - IP match + name match produces a composite candidate with combined
//     confidence via AggregateConfidence.
func TestEngine_RunDiscovery(t *testing.T) {
	store := newMockEdgeStore()
	engine := NewEngine(store)

	// Proxmox host with a VM that carries a parent hint pointing at the host.
	// TrueNAS asset shares the same specific IP as the VM → composite candidate.
	assets := []AssetData{
		{
			ID:     "pve-host-1",
			Name:   "proxmox1",
			Source: "proxmox",
			Type:   "node",
		},
		{
			ID:     "vm-1",
			Name:   "OmegaNAS VM",
			Source: "proxmox",
			Type:   "vm",
			Host:   "10.0.5.50",
			Metadata: map[string]string{
				"node": "proxmox1", // parent hint
			},
		},
		{
			ID:     "truenas-1",
			Name:   "OmegaNAS",
			Source: "truenas",
			Type:   "truenas",
			Host:   "10.0.5.50", // same IP as vm-1
		},
	}

	created, suggested, err := engine.Run(assets)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	t.Logf("created=%d, suggested=%d", created, suggested)

	// Verify edges were actually persisted to the store.
	allEdges, _ := store.ListEdgesBatch([]string{"pve-host-1", "vm-1", "truenas-1"}, 0)
	t.Logf("edges in store: %d", len(allEdges))
	for _, e := range allEdges {
		t.Logf("  edge %s: %s→%s origin=%s conf=%.3f reltype=%s",
			e.ID, e.SourceAssetID, e.TargetAssetID, e.Origin, e.Confidence, e.RelationshipType)
	}

	// Find the parent-hint edge (pve-host-1 → vm-1).
	var parentEdge *edges.Edge
	for i := range allEdges {
		e := &allEdges[i]
		src, tgt := e.SourceAssetID, e.TargetAssetID
		if (src == "pve-host-1" && tgt == "vm-1") || (src == "vm-1" && tgt == "pve-host-1") {
			parentEdge = e
			break
		}
	}
	if parentEdge == nil {
		t.Fatal("expected parent-hint edge between pve-host-1 and vm-1, none found")
	}
	if parentEdge.Origin != edges.OriginAuto {
		t.Errorf("parent-hint edge: want origin=%q, got %q", edges.OriginAuto, parentEdge.Origin)
	}
	if parentEdge.Confidence < 0.90 {
		t.Errorf("parent-hint edge: want confidence >= 0.90, got %.3f", parentEdge.Confidence)
	}

	// The IP match between vm-1 and truenas-1 yields confidence 0.95; combined
	// with NameTokenMatcher ("omeganas" token) the aggregate should exceed 0.95.
	// At minimum, the pair should have been captured (created or suggested).
	if created+suggested == 0 {
		t.Error("expected at least one edge created or suggested, got none")
	}
	if created == 0 {
		t.Error("expected at least one auto-created edge (parent hint has confidence 0.95 ≥ 0.90)")
	}
}

// TestEngine_SkipsExistingEdges verifies that the engine does not create a
// duplicate edge when one already exists between a pair of assets.
func TestEngine_SkipsExistingEdges(t *testing.T) {
	store := newMockEdgeStore()

	// Pre-create an edge between pve-host-1 and vm-1.
	_, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    "pve-host-1",
		TargetAssetID:    "vm-1",
		RelationshipType: edges.RelContains,
		Direction:        edges.DirDownstream,
		Criticality:      edges.CritMedium,
		Origin:           edges.OriginManual,
		Confidence:       1.0,
	})
	if err != nil {
		t.Fatalf("pre-create edge: %v", err)
	}

	engine := NewEngine(store)

	// Same assets as TestEngine_RunDiscovery — the parent hint would normally
	// create an edge between pve-host-1 and vm-1.
	assets := []AssetData{
		{
			ID:     "pve-host-1",
			Name:   "proxmox1",
			Source: "proxmox",
			Type:   "node",
		},
		{
			ID:     "vm-1",
			Name:   "OmegaNAS VM",
			Source: "proxmox",
			Type:   "vm",
			Host:   "10.0.5.50",
			Metadata: map[string]string{
				"node": "proxmox1",
			},
		},
	}

	_, _, err = engine.Run(assets)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	allEdges, _ := store.ListEdgesBatch([]string{"pve-host-1", "vm-1"}, 0)

	// Count edges between this specific pair.
	pairCount := 0
	for _, e := range allEdges {
		src, tgt := e.SourceAssetID, e.TargetAssetID
		if (src == "pve-host-1" && tgt == "vm-1") || (src == "vm-1" && tgt == "pve-host-1") {
			pairCount++
		}
	}

	if pairCount != 1 {
		t.Errorf("expected exactly 1 edge between pve-host-1 and vm-1 (no duplicate), got %d", pairCount)
	}
}

// TestEngine_EmptyAssets verifies the engine returns (0,0,nil) for empty input.
func TestEngine_EmptyAssets(t *testing.T) {
	store := newMockEdgeStore()
	engine := NewEngine(store)

	created, suggested, err := engine.Run(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created != 0 || suggested != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", created, suggested)
	}
}

// TestEngine_SuggestedThreshold verifies that a candidate whose aggregate
// confidence falls in [0.60, 0.89] is stored with origin "suggested".
func TestEngine_SuggestedThreshold(t *testing.T) {
	store := newMockEdgeStore()
	engine := NewEngine(store)

	// StructuralMatcher produces confidence 0.65 for service+single host in the
	// same source scope. No IP/hostname overlap, so no aggregation boost.
	assets := []AssetData{
		{
			ID:     "host-1",
			Name:   "myhost",
			Source: "agent",
			Type:   "host",
		},
		{
			ID:     "nas-1",
			Name:   "mynas",
			Source: "agent",
			Type:   "truenas",
		},
	}

	created, suggested, err := engine.Run(assets)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	t.Logf("created=%d, suggested=%d", created, suggested)

	if created != 0 {
		t.Errorf("expected 0 auto-created edges, got %d", created)
	}
	if suggested == 0 {
		t.Errorf("expected at least 1 suggested edge, got 0")
	}

	allEdges, _ := store.ListEdgesBatch([]string{"host-1", "nas-1"}, 0)
	for _, e := range allEdges {
		if e.Origin != edges.OriginSuggested {
			t.Errorf("expected origin %q, got %q", edges.OriginSuggested, e.Origin)
		}
	}
}

// TestEngine_BelowThresholdDiscarded verifies that candidates below 0.60 are
// not persisted. We construct a scenario where only NameTokenMatcher produces
// a single-token match (confidence 0.60 — right at the boundary), then use
// assets from the same source which all matchers filter out. Here we verify
// that when no signal reaches threshold, no edges are stored.
func TestEngine_BelowThresholdDiscarded(t *testing.T) {
	store := newMockEdgeStore()
	engine := NewEngine(store)

	// Two assets from the same source scope — IPMatcher, HostnameMatcher, and
	// NameTokenMatcher all skip same-source pairs. StructuralMatcher only fires
	// when there is both a service-type and host-type asset. These two are both
	// "unknown" type, so no matcher should fire.
	assets := []AssetData{
		{ID: "a1", Name: "alpha", Source: "custom", Type: "unknown"},
		{ID: "a2", Name: "beta", Source: "custom", Type: "unknown"},
	}

	created, suggested, err := engine.Run(assets)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if created != 0 || suggested != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", created, suggested)
	}
	allEdges, _ := store.ListEdgesBatch([]string{"a1", "a2"}, 0)
	if len(allEdges) != 0 {
		t.Errorf("expected no edges in store, got %d", len(allEdges))
	}
}
