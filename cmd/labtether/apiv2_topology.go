package main

import (
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/topology"
	"log"
	"net/http"
	"time"
)

const (
	maxTopologyMutationItems    = 1000
	maxTopologyIdentifierLength = 255
	topologyMutationRateLimit   = 120
)

func (s *apiServer) enforceTopologyMutationLimit(w http.ResponseWriter, r *http.Request) bool {
	return s.enforceRateLimit(w, r, "v2.topology.mutate", topologyMutationRateLimit, time.Minute)
}

func writeTopologyInternalError(w http.ResponseWriter, clientMessage string, err error) {
	securityruntime.Logf("topology: %s: %v", clientMessage, err)
	apiv2.WriteError(w, http.StatusInternalServerError, "internal", clientMessage)
}

// ---------------------------------------------------------------------------
// GET /api/v2/topology — full topology state
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2Topology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:read") {
		apiv2.WriteScopeForbidden(w, "topology:read")
		return
	}

	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}

	zones, err := s.topologyStore.ListZones(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list zones", err)
		return
	}

	// Auto-seed if no zones exist.
	if len(zones) == 0 {
		zones, err = s.topologyAutoSeed(layout.ID)
		if err != nil {
			log.Printf("topology: auto-seed failed: %v", err)
			// Non-fatal — continue with empty zones.
			zones = []topology.Zone{}
		}
	}

	members, err := s.topologyStore.ListMembers(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list members", err)
		return
	}

	topoConns, err := s.topologyStore.ListConnections(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list connections", err)
		return
	}

	// Get all asset IDs for edge lookup.
	allAssets, err := s.assetStore.ListAssets()
	if err != nil {
		writeTopologyInternalError(w, "failed to list assets", err)
		return
	}

	assetIDs := make([]string, len(allAssets))
	for i, a := range allAssets {
		assetIDs[i] = a.ID
	}

	// Fetch discovered edges for connection merge.
	var merged []topology.MergedConnection
	if len(assetIDs) > 0 {
		discoveredEdges, edgeErr := s.listTopologyEdges(assetIDs)
		if edgeErr != nil {
			log.Printf("topology: failed to list edges for merge: %v", edgeErr)
		}
		merged = topology.MergeConnections(layout.ID, topoConns, discoveredEdges)
	} else {
		merged = topology.MergeConnections(layout.ID, topoConns, nil)
	}

	// Compute unsorted: all asset IDs minus those in zone_members minus dismissed.
	memberSet := make(map[string]bool, len(members))
	for _, m := range members {
		memberSet[m.AssetID] = true
	}

	dismissed, err := s.topologyStore.ListDismissed(layout.ID)
	if err != nil {
		log.Printf("topology: failed to list dismissed: %v", err)
	}
	dismissedSet := make(map[string]bool, len(dismissed))
	for _, d := range dismissed {
		dismissedSet[d] = true
	}

	unsorted := make([]string, 0)
	for _, id := range assetIDs {
		if !memberSet[id] && !dismissedSet[id] {
			unsorted = append(unsorted, id)
		}
	}

	state := topology.TopologyState{
		ID:          layout.ID,
		Name:        layout.Name,
		Zones:       zones,
		Members:     members,
		Connections: merged,
		Unsorted:    unsorted,
		Viewport:    layout.Viewport,
	}

	apiv2.WriteJSON(w, http.StatusOK, state)
}

// topologyAutoSeed generates initial zones, members, and connections from
// existing assets and groups, persists them, and returns the created zones.
func (s *apiServer) topologyAutoSeed(topologyID string) ([]topology.Zone, error) {
	allAssets, err := s.assetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	if len(allAssets) == 0 {
		return []topology.Zone{}, nil
	}

	groups, err := s.groupStore.ListGroups()
	if err != nil {
		return nil, err
	}

	groupLabels := make(map[string]string, len(groups))
	for _, g := range groups {
		groupLabels[g.ID] = g.Name
	}

	assetGroups := make(map[string]string, len(allAssets))
	assets := make([]topology.AssetInfo, len(allAssets))
	assetIDs := make([]string, len(allAssets))
	for i, a := range allAssets {
		assets[i] = topology.AssetInfo{
			ID:     a.ID,
			Label:  a.Name,
			Source: a.Source,
			Type:   a.Type,
		}
		assetIDs[i] = a.ID
		if a.GroupID != "" {
			assetGroups[a.ID] = a.GroupID
		}
	}

	// Fetch edges for seed connections.
	var seedEdges []topology.EdgeInfo
	if len(assetIDs) > 0 {
		discoveredEdges, edgeErr := s.listTopologyEdges(assetIDs)
		if edgeErr == nil {
			for _, e := range discoveredEdges {
				seedEdges = append(seedEdges, topology.EdgeInfo{
					SourceAssetID: e.SourceAssetID,
					TargetAssetID: e.TargetAssetID,
					Relationship:  e.RelationshipType,
				})
			}
		}
	}

	result := topology.Seed(topology.SeedInput{
		TopologyID:  topologyID,
		Assets:      assets,
		Groups:      groupLabels,
		AssetGroups: assetGroups,
		Edges:       seedEdges,
	})

	// Persist seeded data.
	// CreateZone uses INSERT ... RETURNING id, so the DB generates the real UUID.
	// Build a mapping from seed-generated IDs to DB-generated IDs so member
	// ZoneIDs can be rewritten before calling SetMembers.
	seedToDBZoneID := make(map[string]string, len(result.Zones))
	var persistErr error
	for _, z := range result.Zones {
		created, createErr := s.topologyStore.CreateZone(z)
		if createErr != nil {
			persistErr = createErr
			break
		}
		seedToDBZoneID[z.ID] = created.ID
	}
	if persistErr != nil {
		_ = s.topologyStore.ClearTopology(topologyID)
		return nil, persistErr
	}

	// Rewrite member ZoneIDs from seed IDs to DB IDs.
	for i := range result.Members {
		dbID, ok := seedToDBZoneID[result.Members[i].ZoneID]
		if !ok {
			_ = s.topologyStore.ClearTopology(topologyID)
			return nil, errors.New("topology auto-seed produced members for an unknown zone")
		}
		result.Members[i].ZoneID = dbID
	}

	// Group members by zone for SetMembers calls.
	membersByZone := make(map[string][]topology.ZoneMember)
	for _, m := range result.Members {
		membersByZone[m.ZoneID] = append(membersByZone[m.ZoneID], m)
	}
	for zoneID, ms := range membersByZone {
		if err := s.topologyStore.SetMembers(zoneID, ms); err != nil {
			_ = s.topologyStore.ClearTopology(topologyID)
			return nil, err
		}
	}

	for _, c := range result.Connections {
		if _, err := s.topologyStore.CreateConnection(c); err != nil {
			_ = s.topologyStore.ClearTopology(topologyID)
			return nil, err
		}
	}

	// Re-read zones from store (they now have DB-generated IDs).
	return s.topologyStore.ListZones(topologyID)
}

// ---------------------------------------------------------------------------
// POST /api/v2/topology/reset — clear all zones/members/connections/dismissed
//                                and re-run auto-seed
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}

	if err := s.topologyStore.ClearTopology(layout.ID); err != nil {
		writeTopologyInternalError(w, "failed to clear topology", err)
		return
	}

	zones, err := s.topologyAutoSeed(layout.ID)
	if err != nil {
		log.Printf("topology: reset auto-seed failed: %v", err)
		zones = []topology.Zone{}
	}

	// Re-read full state to return.
	members, _ := s.topologyStore.ListMembers(layout.ID)
	if members == nil {
		members = []topology.ZoneMember{}
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "reset",
		"zones":   len(zones),
		"members": len(members),
	})
}
