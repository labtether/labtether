package persistence

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/edges"
)

// newTestPostgresStore creates a PostgresStore connected to the DATABASE_URL.
// It skips the test when the env var is not set.
func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		t.Skip("DATABASE_URL not set, skipping persistence integration test")
	}

	store, err := NewPostgresStore(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// createTestAsset upserts a minimal asset and registers a cleanup deletion.
func createTestAsset(t *testing.T, store *PostgresStore, suffix string) assets.Asset {
	t.Helper()

	ts := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	assetID := "test-asset-" + suffix + "-" + ts

	asset, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "server",
		Name:     "Test Asset " + suffix,
		Source:   "test",
		Status:   "online",
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("createTestAsset(%s) failed: %v", suffix, err)
	}

	t.Cleanup(func() {
		_ = store.DeleteAsset(asset.ID)
	})

	return asset
}

func TestPostgresEdgesStore_CreateAndGet(t *testing.T) {
	store := newTestPostgresStore(t)
	a1 := createTestAsset(t, store, "src")
	a2 := createTestAsset(t, store, "tgt")

	// Create edge
	created, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    a1.ID,
		TargetAssetID:    a2.ID,
		RelationshipType: edges.RelContains,
		Direction:        edges.DirDownstream,
		Criticality:      edges.CritHigh,
		Origin:           edges.OriginManual,
		Confidence:       0.95,
		MatchSignals:     map[string]any{"hostname": true},
		Metadata:         map[string]string{"label": "test"},
	})
	if err != nil {
		t.Fatalf("CreateEdge failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty edge ID")
	}
	if created.SourceAssetID != a1.ID {
		t.Fatalf("expected source %s, got %s", a1.ID, created.SourceAssetID)
	}
	if created.TargetAssetID != a2.ID {
		t.Fatalf("expected target %s, got %s", a2.ID, created.TargetAssetID)
	}
	if created.RelationshipType != edges.RelContains {
		t.Fatalf("expected relationship_type %s, got %s", edges.RelContains, created.RelationshipType)
	}
	if created.Origin != edges.OriginManual {
		t.Fatalf("expected origin %s, got %s", edges.OriginManual, created.Origin)
	}
	if created.Confidence != 0.95 {
		t.Fatalf("expected confidence 0.95, got %f", created.Confidence)
	}

	// Get edge
	got, found, err := store.GetEdge(created.ID)
	if err != nil {
		t.Fatalf("GetEdge failed: %v", err)
	}
	if !found {
		t.Fatal("expected edge to be found")
	}
	if got.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, got.ID)
	}

	// Get non-existent
	_, found, err = store.GetEdge("nonexistent-id")
	if err != nil {
		t.Fatalf("GetEdge(nonexistent) failed: %v", err)
	}
	if found {
		t.Fatal("expected edge not to be found")
	}

	// Update edge
	if err := store.UpdateEdge(created.ID, edges.RelRunsOn, edges.CritCritical); err != nil {
		t.Fatalf("UpdateEdge failed: %v", err)
	}
	updated, found, _ := store.GetEdge(created.ID)
	if !found {
		t.Fatal("expected edge to be found after update")
	}
	if updated.RelationshipType != edges.RelRunsOn {
		t.Fatalf("expected relationship_type %s, got %s", edges.RelRunsOn, updated.RelationshipType)
	}
	if updated.Criticality != edges.CritCritical {
		t.Fatalf("expected criticality %s, got %s", edges.CritCritical, updated.Criticality)
	}

	// List by asset
	edgesByAsset, err := store.ListEdgesByAsset(a1.ID, 10)
	if err != nil {
		t.Fatalf("ListEdgesByAsset failed: %v", err)
	}
	if len(edgesByAsset) == 0 {
		t.Fatal("expected at least one edge for asset")
	}

	// List batch
	edgesBatch, err := store.ListEdgesBatch([]string{a1.ID, a2.ID}, 10)
	if err != nil {
		t.Fatalf("ListEdgesBatch failed: %v", err)
	}
	if len(edgesBatch) == 0 {
		t.Fatal("expected at least one edge in batch")
	}

	// Delete edge
	if err := store.DeleteEdge(created.ID); err != nil {
		t.Fatalf("DeleteEdge failed: %v", err)
	}
	_, found, _ = store.GetEdge(created.ID)
	if found {
		t.Fatal("expected edge to be deleted")
	}
}

func TestPostgresEdgesStore_Descendants(t *testing.T) {
	store := newTestPostgresStore(t)

	// Create a 3-level hierarchy: root -> mid -> leaf
	root := createTestAsset(t, store, "root")
	mid := createTestAsset(t, store, "mid")
	leaf := createTestAsset(t, store, "leaf")

	// root contains mid
	e1, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    root.ID,
		TargetAssetID:    mid.ID,
		RelationshipType: edges.RelContains,
		Direction:        edges.DirDownstream,
		Origin:           edges.OriginManual,
	})
	if err != nil {
		t.Fatalf("CreateEdge root->mid failed: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(e1.ID) })

	// mid contains leaf
	e2, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    mid.ID,
		TargetAssetID:    leaf.ID,
		RelationshipType: edges.RelContains,
		Direction:        edges.DirDownstream,
		Origin:           edges.OriginManual,
	})
	if err != nil {
		t.Fatalf("CreateEdge mid->leaf failed: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(e2.ID) })

	// Descendants from root should find mid (depth 1) and leaf (depth 2)
	desc, err := store.Descendants(root.ID, 5)
	if err != nil {
		t.Fatalf("Descendants failed: %v", err)
	}
	if len(desc) < 2 {
		t.Fatalf("expected at least 2 descendants, got %d", len(desc))
	}

	foundMid := false
	foundLeaf := false
	for _, n := range desc {
		if n.AssetID == mid.ID && n.Depth == 1 {
			foundMid = true
		}
		if n.AssetID == leaf.ID && n.Depth == 2 {
			foundLeaf = true
		}
	}
	if !foundMid {
		t.Fatal("expected mid asset at depth 1")
	}
	if !foundLeaf {
		t.Fatal("expected leaf asset at depth 2")
	}

	// Descendants with maxDepth 1 should only find mid
	shallow, err := store.Descendants(root.ID, 1)
	if err != nil {
		t.Fatalf("Descendants(depth=1) failed: %v", err)
	}
	if len(shallow) != 1 {
		t.Fatalf("expected 1 descendant at depth 1, got %d", len(shallow))
	}
	if shallow[0].AssetID != mid.ID {
		t.Fatalf("expected mid asset, got %s", shallow[0].AssetID)
	}

	// Ancestors from leaf should find mid (depth 1) and root (depth 2)
	anc, err := store.Ancestors(leaf.ID, 5)
	if err != nil {
		t.Fatalf("Ancestors failed: %v", err)
	}
	if len(anc) < 2 {
		t.Fatalf("expected at least 2 ancestors, got %d", len(anc))
	}

	foundMid = false
	foundRoot := false
	for _, n := range anc {
		if n.AssetID == mid.ID && n.Depth == 1 {
			foundMid = true
		}
		if n.AssetID == root.ID && n.Depth == 2 {
			foundRoot = true
		}
	}
	if !foundMid {
		t.Fatal("expected mid asset at depth 1 in ancestors")
	}
	if !foundRoot {
		t.Fatal("expected root asset at depth 2 in ancestors")
	}
}

func TestPostgresEdgesStore_Composites(t *testing.T) {
	store := newTestPostgresStore(t)

	primary := createTestAsset(t, store, "primary")
	facet1 := createTestAsset(t, store, "facet1")
	facet2 := createTestAsset(t, store, "facet2")

	// Create composite
	comp, err := store.CreateComposite(edges.CreateCompositeRequest{
		PrimaryAssetID: primary.ID,
		FacetAssetIDs:  []string{facet1.ID, facet2.ID},
	})
	if err != nil {
		t.Fatalf("CreateComposite failed: %v", err)
	}
	if comp.CompositeID != primary.ID {
		t.Fatalf("expected composite_id %s, got %s", primary.ID, comp.CompositeID)
	}
	if len(comp.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(comp.Members))
	}

	// Clean up composite at end
	t.Cleanup(func() {
		// Delete all composite members
		_ = store.DetachMember(comp.CompositeID, facet2.ID)
		_ = store.DetachMember(comp.CompositeID, facet1.ID)
		_ = store.DetachMember(comp.CompositeID, primary.ID)
	})

	// Verify roles
	hasPrimary := false
	facetCount := 0
	for _, m := range comp.Members {
		if m.Role == "primary" && m.AssetID == primary.ID {
			hasPrimary = true
		}
		if m.Role == "facet" {
			facetCount++
		}
	}
	if !hasPrimary {
		t.Fatal("expected primary member")
	}
	if facetCount != 2 {
		t.Fatalf("expected 2 facet members, got %d", facetCount)
	}

	// Get composite
	got, found, err := store.GetComposite(comp.CompositeID)
	if err != nil {
		t.Fatalf("GetComposite failed: %v", err)
	}
	if !found {
		t.Fatal("expected composite to be found")
	}
	if len(got.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(got.Members))
	}

	// Get non-existent composite
	_, found, err = store.GetComposite("nonexistent-composite-id")
	if err != nil {
		t.Fatalf("GetComposite(nonexistent) failed: %v", err)
	}
	if found {
		t.Fatal("expected composite not to be found")
	}

	// ResolveCompositeID
	resolvedID, found, err := store.ResolveCompositeID(facet1.ID)
	if err != nil {
		t.Fatalf("ResolveCompositeID failed: %v", err)
	}
	if !found {
		t.Fatal("expected composite ID to be resolved")
	}
	if resolvedID != comp.CompositeID {
		t.Fatalf("expected composite_id %s, got %s", comp.CompositeID, resolvedID)
	}

	// ListCompositesByAssets
	composites, err := store.ListCompositesByAssets([]string{facet1.ID})
	if err != nil {
		t.Fatalf("ListCompositesByAssets failed: %v", err)
	}
	if len(composites) != 1 {
		t.Fatalf("expected 1 composite, got %d", len(composites))
	}
	if composites[0].CompositeID != comp.CompositeID {
		t.Fatalf("expected composite_id %s, got %s", comp.CompositeID, composites[0].CompositeID)
	}

	// DetachMember — removing facet2 should leave 2 members
	if err := store.DetachMember(comp.CompositeID, facet2.ID); err != nil {
		t.Fatalf("DetachMember(facet2) failed: %v", err)
	}
	after, found, _ := store.GetComposite(comp.CompositeID)
	if !found {
		t.Fatal("expected composite to still exist after detaching one facet")
	}
	if len(after.Members) != 2 {
		t.Fatalf("expected 2 members after detach, got %d", len(after.Members))
	}

	// DetachMember — removing facet1 leaves only 1 member, which should dissolve the composite
	if err := store.DetachMember(comp.CompositeID, facet1.ID); err != nil {
		t.Fatalf("DetachMember(facet1) failed: %v", err)
	}
	_, found, _ = store.GetComposite(comp.CompositeID)
	if found {
		t.Fatal("expected composite to be dissolved when only 1 member remains")
	}
}

func TestPostgresEdgesStore_Proposals(t *testing.T) {
	store := newTestPostgresStore(t)

	a1 := createTestAsset(t, store, "prop-src")
	a2 := createTestAsset(t, store, "prop-tgt")
	a3 := createTestAsset(t, store, "prop-tgt2")

	// Create two suggested edges
	e1, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    a1.ID,
		TargetAssetID:    a2.ID,
		RelationshipType: edges.RelContains,
		Direction:        edges.DirDownstream,
		Origin:           edges.OriginSuggested,
		Confidence:       0.85,
	})
	if err != nil {
		t.Fatalf("CreateEdge(suggested) failed: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(e1.ID) })

	e2, err := store.CreateEdge(edges.CreateEdgeRequest{
		SourceAssetID:    a1.ID,
		TargetAssetID:    a3.ID,
		RelationshipType: edges.RelRunsOn,
		Direction:        edges.DirDownstream,
		Origin:           edges.OriginSuggested,
		Confidence:       0.70,
	})
	if err != nil {
		t.Fatalf("CreateEdge(suggested 2) failed: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(e2.ID) })

	// List proposals
	proposals, err := store.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals failed: %v", err)
	}
	if len(proposals) < 2 {
		t.Fatalf("expected at least 2 proposals, got %d", len(proposals))
	}

	// Accept first proposal
	if err := store.AcceptProposal(e1.ID); err != nil {
		t.Fatalf("AcceptProposal failed: %v", err)
	}
	accepted, found, _ := store.GetEdge(e1.ID)
	if !found {
		t.Fatal("expected accepted edge to exist")
	}
	if accepted.Origin != edges.OriginManual {
		t.Fatalf("expected origin %s after accept, got %s", edges.OriginManual, accepted.Origin)
	}

	// Dismiss second proposal
	if err := store.DismissProposal(e2.ID); err != nil {
		t.Fatalf("DismissProposal failed: %v", err)
	}
	dismissed, found, _ := store.GetEdge(e2.ID)
	if !found {
		t.Fatal("expected dismissed edge to exist")
	}
	if dismissed.Origin != edges.OriginDismissed {
		t.Fatalf("expected origin %s after dismiss, got %s", edges.OriginDismissed, dismissed.Origin)
	}

	// Proposals list should no longer include them
	remaining, err := store.ListProposals()
	if err != nil {
		t.Fatalf("ListProposals (after resolution) failed: %v", err)
	}
	for _, p := range remaining {
		if p.ID == e1.ID || p.ID == e2.ID {
			t.Fatalf("resolved proposal %s should not appear in list", p.ID)
		}
	}

	// Dismissed proposals should be excluded from tree traversal
	desc, err := store.Descendants(a1.ID, 5)
	if err != nil {
		t.Fatalf("Descendants after dismiss failed: %v", err)
	}
	for _, d := range desc {
		if d.AssetID == a3.ID {
			t.Fatal("dismissed edge should not appear in descendants")
		}
	}
}
