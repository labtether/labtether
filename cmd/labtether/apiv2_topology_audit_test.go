package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/topology"
)

func TestHandleV2TopologyAutoPlaceRejectsDuplicateAssetPlacements(t *testing.T) {
	s := newTestAPIServer(t)
	s.topologyStore = &mockTopologyStore{
		layout: topology.Layout{ID: "topo-1", Name: "Test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/topology/auto-place", strings.NewReader(`{
		"placements": [
			{"asset_id":"asset-1","zone_id":"zone-a"},
			{"asset_id":"asset-1","zone_id":"zone-b"}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2TopologyAutoPlace(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2TopologyZoneReorderReturnsNotFound(t *testing.T) {
	s := newTestAPIServer(t)
	s.topologyStore = &mockTopologyStore{
		reorderErr: topology.ErrNotFound,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v2/topology/zones/reorder", bytes.NewReader([]byte(`{
		"updates": [{"zone_id":"zone-missing","sort_order":0}]
	}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2TopologyZoneReorder(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTopologyAutoSeedClearsPartialStateOnMemberFailure(t *testing.T) {
	s := newTestAPIServer(t)
	s.topologyStore = &mockTopologyStore{
		layout:        topology.Layout{ID: "topo-1", Name: "Test"},
		setMembersErr: errors.New("boom"),
	}
	s.edgeStore = &topologyAuditEdgeStore{}

	_, err := s.assetStore.UpsertAssetHeartbeat(testTopologyHeartbeat("asset-1"))
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}

	_, err = s.topologyAutoSeed("topo-1")
	if err == nil {
		t.Fatal("expected auto-seed error, got nil")
	}

	store := s.topologyStore.(*mockTopologyStore)
	if store.clearTopologyCalls != 1 {
		t.Fatalf("expected ClearTopology to be called once, got %d", store.clearTopologyCalls)
	}
}

func TestListTopologyEdgesSplitsTruncatedBatchesAndDedupes(t *testing.T) {
	s := newTestAPIServer(t)
	s.edgeStore = newTopologyAuditEdgeStoreWithTruncatedBatches()

	assetIDs := []string{"asset-1", "asset-2", "asset-3", "asset-4"}
	got, err := s.listTopologyEdges(assetIDs)
	if err != nil {
		t.Fatalf("listTopologyEdges: %v", err)
	}
	if len(got) != len(assetIDs) {
		t.Fatalf("expected %d unique edges, got %d", len(assetIDs), len(got))
	}

	store := s.edgeStore.(*topologyAuditEdgeStore)
	if store.largeBatchCalls == 0 {
		t.Fatal("expected truncated large-batch path to be exercised")
	}
	if store.singleAssetCalls < len(assetIDs) {
		t.Fatalf("expected single-asset fallback calls, got %d", store.singleAssetCalls)
	}
}

func testTopologyHeartbeat(assetID string) assets.HeartbeatRequest {
	return assets.HeartbeatRequest{
		AssetID: assetID,
		Type:    "server",
		Name:    assetID,
		Source:  "test",
		Status:  "online",
	}
}

type mockTopologyStore struct {
	layout             topology.Layout
	members            []topology.ZoneMember
	zones              []topology.Zone
	reorderErr         error
	setMembersErr      error
	clearTopologyCalls int
}

func (m *mockTopologyStore) GetOrCreateLayout() (topology.Layout, error) {
	if m.layout.ID == "" {
		m.layout = topology.Layout{ID: "topo-1", Name: "Test"}
	}
	return m.layout, nil
}

func (m *mockTopologyStore) UpdateViewport(viewport topology.Viewport) error { return nil }
func (m *mockTopologyStore) CreateZone(z topology.Zone) (topology.Zone, error) {
	if z.ID == "" {
		z.ID = "zone-" + z.Label
	}
	m.zones = append(m.zones, z)
	return z, nil
}
func (m *mockTopologyStore) UpdateZone(z topology.Zone) error { return nil }
func (m *mockTopologyStore) DeleteZone(id string) error       { return nil }
func (m *mockTopologyStore) ListZones(topologyID string) ([]topology.Zone, error) {
	return append([]topology.Zone(nil), m.zones...), nil
}
func (m *mockTopologyStore) ReorderZones(updates []topology.ZoneReorder) error { return m.reorderErr }
func (m *mockTopologyStore) SetMembers(zoneID string, members []topology.ZoneMember) error {
	if m.setMembersErr != nil {
		return m.setMembersErr
	}
	m.members = append([]topology.ZoneMember(nil), members...)
	return nil
}
func (m *mockTopologyStore) RemoveMember(assetID string) error { return nil }
func (m *mockTopologyStore) ListMembers(topologyID string) ([]topology.ZoneMember, error) {
	return append([]topology.ZoneMember(nil), m.members...), nil
}
func (m *mockTopologyStore) CreateConnection(c topology.Connection) (topology.Connection, error) {
	return c, nil
}
func (m *mockTopologyStore) UpdateConnection(id string, relationship, label string) error { return nil }
func (m *mockTopologyStore) DeleteConnection(id string) error                             { return nil }
func (m *mockTopologyStore) ListConnections(topologyID string) ([]topology.Connection, error) {
	return nil, nil
}
func (m *mockTopologyStore) DismissAsset(topologyID, assetID string) error { return nil }
func (m *mockTopologyStore) UndismissAsset(topologyID, assetID string) error {
	return nil
}
func (m *mockTopologyStore) ListDismissed(topologyID string) ([]string, error) { return nil, nil }
func (m *mockTopologyStore) ClearTopology(topologyID string) error {
	m.clearTopologyCalls++
	m.zones = nil
	m.members = nil
	return nil
}

type topologyAuditEdgeStore struct {
	truncatedBatch   []edges.Edge
	largeBatchCalls  int
	singleAssetCalls int
}

func newTopologyAuditEdgeStoreWithTruncatedBatches() *topologyAuditEdgeStore {
	truncated := make([]edges.Edge, 50000)
	for i := range truncated {
		truncated[i] = edges.Edge{ID: "truncated-edge", CreatedAt: time.Unix(0, 0).UTC()}
	}
	return &topologyAuditEdgeStore{truncatedBatch: truncated}
}

func (m *topologyAuditEdgeStore) CreateEdge(req edges.CreateEdgeRequest) (edges.Edge, error) {
	return edges.Edge{}, nil
}
func (m *topologyAuditEdgeStore) GetEdge(id string) (edges.Edge, bool, error) {
	return edges.Edge{}, false, nil
}
func (m *topologyAuditEdgeStore) UpdateEdge(id string, relType, criticality string) error {
	return nil
}
func (m *topologyAuditEdgeStore) DeleteEdge(id string) error { return nil }
func (m *topologyAuditEdgeStore) ListEdgesByAsset(assetID string, limit int) ([]edges.Edge, error) {
	return nil, nil
}
func (m *topologyAuditEdgeStore) ListEdgesBatch(assetIDs []string, limit int) ([]edges.Edge, error) {
	if len(assetIDs) > 1 {
		m.largeBatchCalls++
		return m.truncatedBatch, nil
	}
	m.singleAssetCalls++
	return []edges.Edge{{
		ID:               "edge-" + assetIDs[0],
		SourceAssetID:    assetIDs[0],
		TargetAssetID:    "target-" + assetIDs[0],
		RelationshipType: "runs_on",
		CreatedAt:        time.Unix(int64(len(assetIDs[0])), 0).UTC(),
	}}, nil
}
func (m *topologyAuditEdgeStore) Descendants(rootAssetID string, maxDepth int) ([]edges.TreeNode, error) {
	return nil, nil
}
func (m *topologyAuditEdgeStore) Ancestors(assetID string, maxDepth int) ([]edges.TreeNode, error) {
	return nil, nil
}
func (m *topologyAuditEdgeStore) ListProposals() ([]edges.Edge, error) { return nil, nil }
func (m *topologyAuditEdgeStore) AcceptProposal(edgeID string) error   { return nil }
func (m *topologyAuditEdgeStore) DismissProposal(edgeID string) error  { return nil }
func (m *topologyAuditEdgeStore) CreateComposite(req edges.CreateCompositeRequest) (edges.Composite, error) {
	return edges.Composite{}, nil
}
func (m *topologyAuditEdgeStore) GetComposite(compositeID string) (edges.Composite, bool, error) {
	return edges.Composite{}, false, nil
}
func (m *topologyAuditEdgeStore) ChangePrimary(compositeID, newPrimaryAssetID string) error {
	return nil
}
func (m *topologyAuditEdgeStore) DetachMember(compositeID, memberAssetID string) error {
	return nil
}
func (m *topologyAuditEdgeStore) ListCompositesByAssets(assetIDs []string) ([]edges.Composite, error) {
	return nil, nil
}
func (m *topologyAuditEdgeStore) ResolveCompositeID(assetID string) (string, bool, error) {
	return "", false, nil
}
