package topology

import "github.com/labtether/labtether/internal/edges"

// MergeConnections combines topology_connections with asset_edges into a unified list.
// Rules:
//   - topology_connections with deleted=false are included (origin "user" or "accepted")
//   - asset_edges are included only if no matching topology_connection exists for the same
//     (source, target, relationship) tuple — including soft-deleted ones (which suppress discovered edges)
//   - "contains" edges are excluded (containment is shown via expandable cards, not connection lines)
//   - Edge relationship types not in ValidRelationships are excluded
func MergeConnections(topologyID string, topoConns []Connection, assetEdges []edges.Edge) []MergedConnection {
	type key struct{ src, tgt, rel string }
	topoByKey := make(map[key]Connection, len(topoConns))
	for _, tc := range topoConns {
		k := key{tc.SourceAssetID, tc.TargetAssetID, tc.Relationship}
		topoByKey[k] = tc
	}

	var result []MergedConnection

	// Add all non-deleted topology connections
	for _, tc := range topoConns {
		if tc.Deleted {
			continue
		}
		origin := "user"
		if !tc.UserDefined {
			origin = "accepted"
		}
		result = append(result, MergedConnection{Connection: tc, Origin: origin})
	}

	// Add discovered edges that don't conflict with topology connections
	for _, edge := range assetEdges {
		if edge.RelationshipType == "contains" {
			continue
		}
		if !ValidRelationships[edge.RelationshipType] {
			continue
		}
		k := key{edge.SourceAssetID, edge.TargetAssetID, edge.RelationshipType}
		if _, exists := topoByKey[k]; exists {
			continue // topology connection takes precedence (including soft-deletes)
		}
		result = append(result, MergedConnection{
			Connection: Connection{
				ID:            edge.ID, // NOTE: this is the edge ID, not a topology connection ID
				TopologyID:    topologyID,
				SourceAssetID: edge.SourceAssetID,
				TargetAssetID: edge.TargetAssetID,
				Relationship:  edge.RelationshipType,
				UserDefined:   false,
			},
			Origin: "discovered",
		})
	}

	return result
}
