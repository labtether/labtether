package persistence

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/edges"
)

func createDirectionTestEdge(t *testing.T, store *PostgresStore, req edges.CreateEdgeRequest) edges.Edge {
	t.Helper()
	edge, err := store.CreateEdge(req)
	if err != nil {
		t.Fatalf("CreateEdge %s %s->%s: %v", req.RelationshipType, req.SourceAssetID, req.TargetAssetID, err)
	}
	t.Cleanup(func() { _ = store.DeleteEdge(edge.ID) })
	return edge
}

func treeDepths(nodes []edges.TreeNode) map[string]int {
	out := make(map[string]int, len(nodes))
	for _, node := range nodes {
		out[node.AssetID] = node.Depth
	}
	return out
}

// runs_on and hosted_on point from the child to its host. Read the other way,
// the device breadcrumb of a Docker host listed its own containers and Docker
// engine as its parents.
func TestPostgresEdgesStore_RunsOnPointsFromChildToHost(t *testing.T) {
	store := newTestPostgresStore(t)
	host := createTestAsset(t, store, "dirhost")
	engine := createTestAsset(t, store, "direngine")
	container := createTestAsset(t, store, "dircontainer")
	cluster := createTestAsset(t, store, "dircluster")

	createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: engine.ID, TargetAssetID: host.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginManual})
	createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: container.ID, TargetAssetID: engine.ID, RelationshipType: edges.RelHostedOn, Origin: edges.OriginManual})
	createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: container.ID, TargetAssetID: host.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginAuto})
	createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: cluster.ID, TargetAssetID: host.ID, RelationshipType: edges.RelContains, Origin: edges.OriginSuggested})

	hostAncestors, err := store.Ancestors(host.ID, 5)
	if err != nil {
		t.Fatalf("Ancestors(host): %v", err)
	}
	if len(hostAncestors) != 0 {
		t.Fatalf("host ancestors = %+v, want none (a suggested link is not a parent)", hostAncestors)
	}

	chain, err := store.Ancestors(container.ID, 5)
	if err != nil {
		t.Fatalf("Ancestors(container): %v", err)
	}
	if len(chain) != 2 || chain[0].AssetID != host.ID || chain[0].Depth != 2 || chain[1].AssetID != engine.ID || chain[1].Depth != 1 {
		t.Fatalf("container ancestors = %+v, want host (2) then engine (1), each once", chain)
	}

	children, err := store.Descendants(host.ID, 5)
	if err != nil {
		t.Fatalf("Descendants(host): %v", err)
	}
	if got := treeDepths(children); len(got) != 2 || got[engine.ID] != 1 || got[container.ID] != 1 {
		t.Fatalf("host descendants = %+v, want engine and container at depth 1", children)
	}
	engineChildren, err := store.Descendants(engine.ID, 5)
	if err != nil {
		t.Fatalf("Descendants(engine): %v", err)
	}
	if got := treeDepths(engineChildren); len(got) != 1 || got[container.ID] != 1 {
		t.Fatalf("engine descendants = %+v, want only the container", engineChildren)
	}
}

// Migration 110 turns runs_on links that link discovery stored from the host
// to the child, removes such a link when its right-way twin exists, and leaves
// a link that is already the right way round alone, even when run again.
func TestDiscoveryRunsOnMigrationTurnsOnlyReversedLinks(t *testing.T) {
	store := newTestPostgresStore(t)
	host := createTestAsset(t, store, "mighost")
	reversedChild := createTestAsset(t, store, "migrev")
	twinChild := createTestAsset(t, store, "migtwin")
	rightChild := createTestAsset(t, store, "migright")
	hint := map[string]any{"signals": []string{"parent_hint:agent_id=" + host.ID}}

	reversed := createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: host.ID, TargetAssetID: reversedChild.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginAuto, MatchSignals: hint})
	twin := createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: host.ID, TargetAssetID: twinChild.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginAuto, MatchSignals: hint})
	createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: twinChild.ID, TargetAssetID: host.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginManual})
	right := createDirectionTestEdge(t, store, edges.CreateEdgeRequest{SourceAssetID: rightChild.ID, TargetAssetID: host.ID, RelationshipType: edges.RelRunsOn, Origin: edges.OriginAuto, MatchSignals: hint})

	for run := 0; run < 2; run++ {
		for _, statement := range schemaMigrationDiscoveryRunsOnChildToHost().Statements {
			if _, err := store.pool.Exec(context.Background(), statement); err != nil {
				t.Fatalf("run %d: %v", run, err)
			}
		}
	}

	turned, ok, err := store.GetEdge(reversed.ID)
	if err != nil || !ok || turned.SourceAssetID != reversedChild.ID || turned.TargetAssetID != host.ID {
		t.Fatalf("reversed link = %+v (found %v, err %v), want child -> host", turned, ok, err)
	}
	if _, ok, err := store.GetEdge(twin.ID); err != nil || ok {
		t.Fatalf("reversed link with a right-way twin still exists (err %v)", err)
	}
	kept, ok, err := store.GetEdge(right.ID)
	if err != nil || !ok || kept.SourceAssetID != rightChild.ID || kept.TargetAssetID != host.ID {
		t.Fatalf("right-way link = %+v (found %v, err %v), want it unchanged", kept, ok, err)
	}
}
