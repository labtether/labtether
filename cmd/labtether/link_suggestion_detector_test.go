package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/edges"
)

// ---------------------------------------------------------------------------
// mockEdgeStore — minimal in-memory edges.Store for unit tests
// ---------------------------------------------------------------------------

type mockEdgeStore struct {
	mu     sync.Mutex
	edges  []edges.Edge
	nextID int
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

func (s *mockEdgeStore) UpdateEdge(id string, relType, criticality string) error { return nil }

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

// allEdges returns a snapshot of all edges in the store.
func (s *mockEdgeStore) allEdges() []edges.Edge {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]edges.Edge, len(s.edges))
	copy(out, s.edges)
	return out
}

// ---------------------------------------------------------------------------
// Tests for detectLinkSuggestions (now wired to the discovery engine)
// ---------------------------------------------------------------------------

func newTestAPIServerWithEdgeStore(t *testing.T) (*apiServer, *mockEdgeStore) {
	t.Helper()
	sut := newTestAPIServer(t)
	store := newMockEdgeStore()
	sut.edgeStore = store
	return sut, store
}

// TestDetectLinkSuggestionsHostnameMatch verifies that two assets from different
// sources sharing a hostname produce an edge via the discovery engine.
func TestDetectLinkSuggestionsHostnameMatch(t *testing.T) {
	sut, store := newTestAPIServerWithEdgeStore(t)

	// Proxmox VM named "omeganas" and TrueNAS host named "omeganas" from different sources.
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "proxmox-vm-101",
		Type:     "vm",
		Name:     "omeganas",
		Source:   "proxmox",
		Metadata: map[string]string{"vmid": "101"},
	}); err != nil {
		t.Fatalf("upsert proxmox vm: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "truenas-host-omeganas",
		Type:     "nas",
		Name:     "omeganas",
		Source:   "truenas",
		Metadata: map[string]string{"hostname": "omeganas"},
	}); err != nil {
		t.Fatalf("upsert truenas host: %v", err)
	}

	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("detectLinkSuggestions() error = %v", err)
	}

	all := store.allEdges()
	if len(all) == 0 {
		t.Fatalf("expected at least one edge from hostname match, got 0")
	}

	// At least one edge must involve both assets.
	found := false
	for _, e := range all {
		if (e.SourceAssetID == "proxmox-vm-101" && e.TargetAssetID == "truenas-host-omeganas") ||
			(e.SourceAssetID == "truenas-host-omeganas" && e.TargetAssetID == "proxmox-vm-101") {
			found = true
			// Hostname match → confidence 0.90 → origin auto.
			if e.Confidence < 0.60 {
				t.Fatalf("expected confidence >= 0.60, got %v", e.Confidence)
			}
		}
	}
	if !found {
		t.Fatalf("no edge found between proxmox-vm-101 and truenas-host-omeganas")
	}
}

// TestDetectLinkSuggestionsIPMatch verifies that assets from different sources
// sharing an IP address produce an edge.
func TestDetectLinkSuggestionsIPMatch(t *testing.T) {
	sut, store := newTestAPIServerWithEdgeStore(t)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "proxmox-vm-200",
		Type:     "vm",
		Name:     "webserver-vm",
		Source:   "proxmox",
		Metadata: map[string]string{"ip": "10.0.5.50"},
	}); err != nil {
		t.Fatalf("upsert proxmox vm: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "truenas-host-webnas",
		Type:     "nas",
		Name:     "webnas",
		Source:   "truenas",
		Metadata: map[string]string{"ip": "10.0.5.50"},
	}); err != nil {
		t.Fatalf("upsert truenas host: %v", err)
	}

	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("detectLinkSuggestions() error = %v", err)
	}

	all := store.allEdges()
	found := false
	for _, e := range all {
		if (e.SourceAssetID == "proxmox-vm-200" && e.TargetAssetID == "truenas-host-webnas") ||
			(e.SourceAssetID == "truenas-host-webnas" && e.TargetAssetID == "proxmox-vm-200") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no edge found between proxmox-vm-200 and truenas-host-webnas")
	}
}

// TestDetectLinkSuggestionsSameSourceNoMatch verifies that two assets from the
// same source sharing an IP do not produce a cross-source edge.
func TestDetectLinkSuggestionsSameSourceNoMatch(t *testing.T) {
	sut, store := newTestAPIServerWithEdgeStore(t)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "proxmox-vm-100",
		Type:     "vm",
		Name:     "vm-alpha",
		Source:   "proxmox",
		Metadata: map[string]string{"ip": "10.0.0.10"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "proxmox-node-pve1",
		Type:     "hypervisor-node",
		Name:     "pve1",
		Source:   "proxmox",
		Metadata: map[string]string{"ip": "10.0.0.10"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("detectLinkSuggestions() error = %v", err)
	}

	// No cross-source edge should have been created between these two same-source assets.
	for _, e := range store.allEdges() {
		if (e.SourceAssetID == "proxmox-vm-100" && e.TargetAssetID == "proxmox-node-pve1") ||
			(e.SourceAssetID == "proxmox-node-pve1" && e.TargetAssetID == "proxmox-vm-100") {
			t.Fatalf("unexpected cross-source edge between same-source assets: %+v", e)
		}
	}
}

// TestDetectLinkSuggestionsSkipsExistingEdge verifies that running the detector
// a second time does not duplicate edges that already exist in the store.
func TestDetectLinkSuggestionsSkipsExistingEdge(t *testing.T) {
	sut, store := newTestAPIServerWithEdgeStore(t)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "proxmox-vm-300",
		Type:     "vm",
		Name:     "myhost",
		Source:   "proxmox",
		Metadata: map[string]string{},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "agent-myhost",
		Type:     "host",
		Name:     "myhost",
		Source:   "agent",
		Metadata: map[string]string{},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// First run creates edges.
	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("first detectLinkSuggestions() error = %v", err)
	}
	countAfterFirst := len(store.allEdges())

	// Second run should not create any new edges.
	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("second detectLinkSuggestions() error = %v", err)
	}
	countAfterSecond := len(store.allEdges())

	if countAfterSecond != countAfterFirst {
		t.Fatalf("second run created %d new edges; expected 0 new (had %d, now %d)",
			countAfterSecond-countAfterFirst, countAfterFirst, countAfterSecond)
	}
}

// TestDetectLinkSuggestionsNilStoresNoPanic verifies that a nil assetStore or
// edgeStore causes early return without panic.
func TestDetectLinkSuggestionsNilStoresNoPanic(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.assetStore = nil

	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("detectLinkSuggestions() with nil stores should return nil, got %v", err)
	}
}

// TestDetectLinkSuggestionsNilEdgeStoreNoPanic verifies that nil edgeStore
// causes early return without panic.
func TestDetectLinkSuggestionsNilEdgeStoreNoPanic(t *testing.T) {
	sut := newTestAPIServer(t)
	// edgeStore is nil by default in newTestAPIServer.

	if err := sut.detectLinkSuggestions(); err != nil {
		t.Fatalf("detectLinkSuggestions() with nil edgeStore should return nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// normalizeMACAddress tests (unchanged — helper kept in link_suggestion_detector.go)
// ---------------------------------------------------------------------------

func TestNormalizeMACAddress(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"aa-bb-cc-dd-ee-ff", "aa:bb:cc:dd:ee:ff"},
		{"aabb.ccdd.eeff", "aa:bb:cc:dd:ee:ff"},
		{"", ""},
		{"not-a-mac", ""},
		{"aa:bb:cc:dd:ee", ""},
	}

	for _, tc := range cases {
		got := normalizeMACAddress(tc.input)
		if got != tc.want {
			t.Errorf("normalizeMACAddress(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
