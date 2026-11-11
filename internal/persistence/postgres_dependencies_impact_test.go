package persistence

import (
	"testing"

	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/edges"
)

func impactDepths(t *testing.T, nodes []dependencies.ImpactNode) map[string]int {
	t.Helper()
	out := make(map[string]int, len(nodes))
	for _, node := range nodes {
		if _, twice := out[node.AssetID]; twice {
			t.Fatalf("%s listed twice in %+v", node.AssetID, nodes)
		}
		out[node.AssetID] = node.Depth
	}
	return out
}

// "nas depends on host" put the host in the NAS's blast radius and left the
// NAS out of the host's: the walk read every link as cause to affected, and
// the upstream walk only followed links marked upstream. A device that needs
// another is hit when that one goes down.
func TestPostgresBlastRadiusFollowsWhatEachLinkMeans(t *testing.T) {
	store := newTestPostgresStore(t)
	host := createTestAsset(t, store, "impacthost")
	dependent := createTestAsset(t, store, "impactdep")
	engine := createTestAsset(t, store, "impactengine")
	container := createTestAsset(t, store, "impactct")
	cluster := createTestAsset(t, store, "impactcluster")

	for _, req := range []dependencies.CreateDependencyRequest{
		{SourceAssetID: dependent.ID, TargetAssetID: host.ID, RelationshipType: dependencies.RelationshipDependsOn, Direction: dependencies.DirectionDownstream, Criticality: dependencies.CriticalityCritical},
		{SourceAssetID: engine.ID, TargetAssetID: host.ID, RelationshipType: dependencies.RelationshipRunsOn},
		{SourceAssetID: container.ID, TargetAssetID: engine.ID, RelationshipType: dependencies.RelationshipHostedOn},
	} {
		dep, err := store.CreateAssetDependency(req)
		if err != nil {
			t.Fatalf("CreateAssetDependency %+v: %v", req, err)
		}
		t.Cleanup(func() { _ = store.DeleteAssetDependency(dep.ID) })
	}
	suggested, err := store.CreateEdge(edges.CreateEdgeRequest{SourceAssetID: cluster.ID, TargetAssetID: host.ID, RelationshipType: edges.RelContains, Origin: edges.OriginSuggested})
	if err != nil {
		t.Fatalf("CreateEdge suggested: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(suggested.ID) })

	blast, err := store.BlastRadius(host.ID, 5)
	if err != nil {
		t.Fatalf("BlastRadius(host): %v", err)
	}
	if got := impactDepths(t, blast); len(got) != 3 || got[dependent.ID] != 1 || got[engine.ID] != 1 || got[container.ID] != 2 {
		t.Fatalf("host blast radius = %+v, want dependent and engine at 1, container at 2", blast)
	}
	dependentBlast, err := store.BlastRadius(dependent.ID, 5)
	if err != nil || len(dependentBlast) != 0 {
		t.Fatalf("dependent blast radius = %+v (err %v), want none", dependentBlast, err)
	}

	upstream, err := store.UpstreamCauses(dependent.ID, 5)
	if err != nil {
		t.Fatalf("UpstreamCauses(dependent): %v", err)
	}
	if got := impactDepths(t, upstream); len(got) != 1 || got[host.ID] != 1 {
		t.Fatalf("dependent upstream = %+v, want the host", upstream)
	}
	chain, err := store.UpstreamCauses(container.ID, 5)
	if err != nil {
		t.Fatalf("UpstreamCauses(container): %v", err)
	}
	if got := impactDepths(t, chain); len(got) != 2 || got[engine.ID] != 1 || got[host.ID] != 2 {
		t.Fatalf("container upstream = %+v, want engine (1) and host (2); a suggested link is no cause", chain)
	}
}
