package topology

import (
	"testing"

	"github.com/labtether/labtether/internal/edges"
)

func TestMergeConnections_EmptyInputs(t *testing.T) {
	result := MergeConnections("test-topology", nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestMergeConnections_UserConnection(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-1",
			SourceAssetID: "asset-a",
			TargetAssetID: "asset-b",
			Relationship:  "depends_on",
			UserDefined:   true,
			Deleted:       false,
		},
	}
	result := MergeConnections("test-topology", topoConns, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Origin != "user" {
		t.Errorf("expected origin 'user', got %q", result[0].Origin)
	}
	if result[0].ID != "tc-1" {
		t.Errorf("expected ID 'tc-1', got %q", result[0].ID)
	}
}

func TestMergeConnections_AcceptedConnection(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-2",
			SourceAssetID: "asset-a",
			TargetAssetID: "asset-b",
			Relationship:  "depends_on",
			UserDefined:   false, // accepted (not user-defined)
			Deleted:       false,
		},
	}
	result := MergeConnections("test-topology", topoConns, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Origin != "accepted" {
		t.Errorf("expected origin 'accepted', got %q", result[0].Origin)
	}
}

func TestMergeConnections_SoftDeletedExcluded(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-3",
			SourceAssetID: "asset-a",
			TargetAssetID: "asset-b",
			Relationship:  "depends_on",
			UserDefined:   true,
			Deleted:       true, // soft-deleted
		},
	}
	result := MergeConnections("test-topology", topoConns, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for soft-deleted connection, got %d items", len(result))
	}
}

func TestMergeConnections_SoftDeletedSuppressesDiscovered(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-4",
			SourceAssetID: "asset-a",
			TargetAssetID: "asset-b",
			Relationship:  "depends_on",
			UserDefined:   true,
			Deleted:       true, // soft-deleted but still suppresses
		},
	}
	assetEdges := []edges.Edge{
		{
			ID:               "edge-1",
			SourceAssetID:    "asset-a",
			TargetAssetID:    "asset-b",
			RelationshipType: "depends_on",
		},
	}
	result := MergeConnections("test-topology", topoConns, assetEdges)
	if len(result) != 0 {
		t.Errorf("expected soft-deleted topology connection to suppress discovered edge, got %d items", len(result))
	}
}

func TestMergeConnections_DiscoveredEdgeIncluded(t *testing.T) {
	assetEdges := []edges.Edge{
		{
			ID:               "edge-2",
			SourceAssetID:    "asset-c",
			TargetAssetID:    "asset-d",
			RelationshipType: "connected_to",
		},
	}
	result := MergeConnections("test-topology", nil, assetEdges)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Origin != "discovered" {
		t.Errorf("expected origin 'discovered', got %q", result[0].Origin)
	}
	if result[0].ID != "edge-2" {
		t.Errorf("expected ID 'edge-2', got %q", result[0].ID)
	}
}

func TestMergeConnections_ContainsEdgeExcluded(t *testing.T) {
	assetEdges := []edges.Edge{
		{
			ID:               "edge-3",
			SourceAssetID:    "asset-e",
			TargetAssetID:    "asset-f",
			RelationshipType: "contains",
		},
	}
	result := MergeConnections("test-topology", nil, assetEdges)
	if len(result) != 0 {
		t.Errorf("expected 'contains' edge to be excluded, got %d items", len(result))
	}
}

func TestMergeConnections_UnknownRelationshipExcluded(t *testing.T) {
	assetEdges := []edges.Edge{
		{
			ID:               "edge-4",
			SourceAssetID:    "asset-g",
			TargetAssetID:    "asset-h",
			RelationshipType: "unknown_rel",
		},
	}
	result := MergeConnections("test-topology", nil, assetEdges)
	if len(result) != 0 {
		t.Errorf("expected unknown relationship type to be excluded, got %d items", len(result))
	}
}

func TestMergeConnections_BothSourcesContributeUnique(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-5",
			SourceAssetID: "asset-a",
			TargetAssetID: "asset-b",
			Relationship:  "depends_on",
			UserDefined:   true,
			Deleted:       false,
		},
	}
	assetEdges := []edges.Edge{
		{
			ID:               "edge-5",
			SourceAssetID:    "asset-c",
			TargetAssetID:    "asset-d",
			RelationshipType: "peer_of",
		},
	}
	result := MergeConnections("test-topology", topoConns, assetEdges)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	origins := make(map[string]bool)
	for _, mc := range result {
		origins[mc.Origin] = true
	}
	if !origins["user"] {
		t.Error("expected a 'user' origin in results")
	}
	if !origins["discovered"] {
		t.Error("expected a 'discovered' origin in results")
	}
}

func TestMergeConnections_TopologyConnectionTakesPrecedenceOverDiscovered(t *testing.T) {
	topoConns := []Connection{
		{
			ID:            "tc-6",
			SourceAssetID: "asset-x",
			TargetAssetID: "asset-y",
			Relationship:  "runs_on",
			UserDefined:   true,
			Deleted:       false,
		},
	}
	assetEdges := []edges.Edge{
		{
			ID:               "edge-6",
			SourceAssetID:    "asset-x",
			TargetAssetID:    "asset-y",
			RelationshipType: "runs_on", // same tuple as topology connection
		},
	}
	result := MergeConnections("test-topology", topoConns, assetEdges)
	if len(result) != 1 {
		t.Fatalf("expected 1 result (topology wins), got %d", len(result))
	}
	if result[0].Origin != "user" {
		t.Errorf("expected origin 'user', got %q", result[0].Origin)
	}
	if result[0].ID != "tc-6" {
		t.Errorf("expected topology connection ID 'tc-6', got %q", result[0].ID)
	}
}
