package main

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/topology"
)

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
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}

	zones, err := s.topologyStore.ListZones(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list zones: "+err.Error())
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
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list members: "+err.Error())
		return
	}

	topoConns, err := s.topologyStore.ListConnections(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list connections: "+err.Error())
		return
	}

	// Get all asset IDs for edge lookup.
	allAssets, err := s.assetStore.ListAssets()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list assets: "+err.Error())
		return
	}

	assetIDs := make([]string, len(allAssets))
	for i, a := range allAssets {
		assetIDs[i] = a.ID
	}

	// Fetch discovered edges for connection merge.
	var merged []topology.MergedConnection
	if len(assetIDs) > 0 {
		discoveredEdges, edgeErr := s.edgeStore.ListEdgesBatch(assetIDs, 10000)
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
		discoveredEdges, edgeErr := s.edgeStore.ListEdgesBatch(assetIDs, 10000)
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
	for _, z := range result.Zones {
		created, createErr := s.topologyStore.CreateZone(z)
		if createErr != nil {
			log.Printf("topology: auto-seed create zone %q failed: %v", z.Label, createErr)
			continue
		}
		seedToDBZoneID[z.ID] = created.ID
	}

	// Rewrite member ZoneIDs from seed IDs to DB IDs.
	for i := range result.Members {
		if dbID, ok := seedToDBZoneID[result.Members[i].ZoneID]; ok {
			result.Members[i].ZoneID = dbID
		}
	}

	// Group members by zone for SetMembers calls.
	membersByZone := make(map[string][]topology.ZoneMember)
	for _, m := range result.Members {
		membersByZone[m.ZoneID] = append(membersByZone[m.ZoneID], m)
	}
	for zoneID, ms := range membersByZone {
		if err := s.topologyStore.SetMembers(zoneID, ms); err != nil {
			log.Printf("topology: auto-seed set members for zone %s failed: %v", zoneID, err)
		}
	}

	for _, c := range result.Connections {
		if _, err := s.topologyStore.CreateConnection(c); err != nil {
			log.Printf("topology: auto-seed create connection failed: %v", err)
		}
	}

	// Re-read zones from store (they now have DB-generated IDs).
	return s.topologyStore.ListZones(topologyID)
}

// ---------------------------------------------------------------------------
// POST /api/v2/topology/zones — create a zone
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req topology.Zone
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	// Ensure topology_id is set.
	if req.TopologyID == "" {
		layout, err := s.topologyStore.GetOrCreateLayout()
		if err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
			return
		}
		req.TopologyID = layout.ID
	}

	if req.Label == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "label is required")
		return
	}

	zone, err := s.topologyStore.CreateZone(req)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create zone: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusCreated, zone)
}

// ---------------------------------------------------------------------------
// PUT/DELETE /api/v2/topology/zones/{id}
// PUT        /api/v2/topology/zones/{id}/members
// PUT        /api/v2/topology/zones/reorder
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyZoneActions(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v2/topology/zones/")
	if suffix == "" || suffix == r.URL.Path {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "zone id or action required")
		return
	}

	// Handle /zones/reorder
	if suffix == "reorder" {
		s.handleV2TopologyZoneReorder(w, r)
		return
	}

	// Handle /zones/{id}/members
	if strings.HasSuffix(suffix, "/members") {
		zoneID := strings.TrimSuffix(suffix, "/members")
		s.handleV2TopologyZoneMembers(w, r, zoneID)
		return
	}

	// Single zone: PUT or DELETE /zones/{id}
	zoneID := strings.TrimRight(suffix, "/")
	if strings.Contains(zoneID, "/") {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "unknown zone sub-path")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
			apiv2.WriteScopeForbidden(w, "topology:write")
			return
		}
		var req topology.Zone
		if err := decodeJSONBody(w, r, &req); err != nil {
			return
		}
		req.ID = zoneID
		if err := s.topologyStore.UpdateZone(req); err != nil {
			if errors.Is(err, topology.ErrNotFound) {
				apiv2.WriteError(w, http.StatusNotFound, "not_found", "zone not found")
				return
			}
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update zone: "+err.Error())
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
			apiv2.WriteScopeForbidden(w, "topology:write")
			return
		}
		if err := s.topologyStore.DeleteZone(zoneID); err != nil {
			if errors.Is(err, topology.ErrNotFound) {
				apiv2.WriteError(w, http.StatusNotFound, "not_found", "zone not found")
				return
			}
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete zone: "+err.Error())
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PUT or DELETE required")
	}
}

// handleV2TopologyZoneMembers handles PUT /api/v2/topology/zones/{id}/members.
func (s *apiServer) handleV2TopologyZoneMembers(w http.ResponseWriter, r *http.Request, zoneID string) {
	if r.Method != http.MethodPut {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PUT required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req struct {
		Members []topology.ZoneMember `json:"members"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	// Set zone_id on all members.
	for i := range req.Members {
		req.Members[i].ZoneID = zoneID
	}

	if err := s.topologyStore.SetMembers(zoneID, req.Members); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to set members: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleV2TopologyZoneReorder handles PUT /api/v2/topology/zones/reorder.
func (s *apiServer) handleV2TopologyZoneReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PUT required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req struct {
		Updates []topology.ZoneReorder `json:"updates"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if len(req.Updates) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "updates array is required")
		return
	}

	if err := s.topologyStore.ReorderZones(req.Updates); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to reorder zones: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
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
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}

	if err := s.topologyStore.ClearTopology(layout.ID); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to clear topology: "+err.Error())
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

// ---------------------------------------------------------------------------
// POST /api/v2/topology/connections — create a connection
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req topology.Connection
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	// Validate relationship type.
	if !topology.ValidRelationships[req.Relationship] {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			"invalid relationship type: "+req.Relationship)
		return
	}

	if req.SourceAssetID == "" || req.TargetAssetID == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			"source_asset_id and target_asset_id are required")
		return
	}

	if req.SourceAssetID == req.TargetAssetID {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			"self-referencing connection not allowed")
		return
	}

	// Ensure topology_id is set.
	if req.TopologyID == "" {
		layout, err := s.topologyStore.GetOrCreateLayout()
		if err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
			return
		}
		req.TopologyID = layout.ID
	}

	req.UserDefined = true

	conn, err := s.topologyStore.CreateConnection(req)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create connection: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusCreated, conn)
}

// ---------------------------------------------------------------------------
// PUT/DELETE /api/v2/topology/connections/{id}
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyConnection(w http.ResponseWriter, r *http.Request) {
	connID := strings.TrimPrefix(r.URL.Path, "/api/v2/topology/connections/")
	connID = strings.TrimRight(connID, "/")
	if connID == "" || strings.Contains(connID, "/") {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "connection id required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
			apiv2.WriteScopeForbidden(w, "topology:write")
			return
		}
		var req struct {
			Relationship string `json:"relationship"`
			Label        string `json:"label"`
		}
		if err := decodeJSONBody(w, r, &req); err != nil {
			return
		}
		if req.Relationship != "" && !topology.ValidRelationships[req.Relationship] {
			apiv2.WriteError(w, http.StatusBadRequest, "validation",
				"invalid relationship type: "+req.Relationship)
			return
		}
		if err := s.topologyStore.UpdateConnection(connID, req.Relationship, req.Label); err != nil {
			if errors.Is(err, topology.ErrNotFound) {
				apiv2.WriteError(w, http.StatusNotFound, "not_found", "connection not found")
				return
			}
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update connection: "+err.Error())
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
			apiv2.WriteScopeForbidden(w, "topology:write")
			return
		}
		if err := s.topologyStore.DeleteConnection(connID); err != nil {
			if errors.Is(err, topology.ErrNotFound) {
				apiv2.WriteError(w, http.StatusNotFound, "not_found", "connection not found")
				return
			}
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete connection: "+err.Error())
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PUT or DELETE required")
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v2/topology/viewport — save viewport state
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyViewport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PUT required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req topology.Viewport
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if err := s.topologyStore.UpdateViewport(req); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "no layout exists")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update viewport: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ---------------------------------------------------------------------------
// GET /api/v2/topology/unsorted — unsorted assets with placement suggestions
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyUnsorted(w http.ResponseWriter, r *http.Request) {
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
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}

	allAssets, err := s.assetStore.ListAssets()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list assets: "+err.Error())
		return
	}

	zones, err := s.topologyStore.ListZones(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list zones: "+err.Error())
		return
	}

	members, err := s.topologyStore.ListMembers(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list members: "+err.Error())
		return
	}

	dismissed, err := s.topologyStore.ListDismissed(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list dismissed: "+err.Error())
		return
	}

	// Build lookup maps.
	memberSet := make(map[string]bool, len(members))
	for _, m := range members {
		memberSet[m.AssetID] = true
	}
	dismissedSet := make(map[string]bool, len(dismissed))
	for _, d := range dismissed {
		dismissedSet[d] = true
	}

	// Build asset info maps.
	assetInfoMap := make(map[string]topology.AssetInfo, len(allAssets))
	for _, a := range allAssets {
		assetInfoMap[a.ID] = topology.AssetInfo{
			ID:     a.ID,
			Label:  a.Name,
			Source: a.Source,
			Type:   a.Type,
		}
	}

	// Compute unsorted assets.
	var unsortedAssets []topology.AssetInfo
	for _, a := range allAssets {
		if !memberSet[a.ID] && !dismissedSet[a.ID] {
			unsortedAssets = append(unsortedAssets, assetInfoMap[a.ID])
		}
	}

	// Build member asset info map for placed assets.
	memberAssets := make(map[string]topology.AssetInfo, len(members))
	for _, m := range members {
		if info, ok := assetInfoMap[m.AssetID]; ok {
			memberAssets[m.AssetID] = info
		}
	}

	// Build parent map from "contains" edges.
	assetIDs := make([]string, len(allAssets))
	for i, a := range allAssets {
		assetIDs[i] = a.ID
	}
	parentMap := make(map[string]string)
	if len(assetIDs) > 0 {
		allEdges, edgeErr := s.edgeStore.ListEdgesBatch(assetIDs, 10000)
		if edgeErr == nil {
			for _, e := range allEdges {
				if e.RelationshipType == "contains" {
					parentMap[e.TargetAssetID] = e.SourceAssetID
				}
			}
		}
	}

	suggestions := topology.SuggestPlacements(unsortedAssets, zones, members, memberAssets, parentMap)

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"unsorted":    unsortedAssets,
		"suggestions": suggestions,
	})
}

// ---------------------------------------------------------------------------
// POST /api/v2/topology/auto-place — bulk auto-place unsorted assets
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyAutoPlace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req struct {
		Placements []struct {
			AssetID string `json:"asset_id"`
			ZoneID  string `json:"zone_id"`
		} `json:"placements"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if len(req.Placements) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "placements array is required")
		return
	}

	// Group placements by zone.
	byZone := make(map[string][]topology.ZoneMember)
	sortIdx := 0
	for _, p := range req.Placements {
		if p.AssetID == "" || p.ZoneID == "" {
			continue
		}
		byZone[p.ZoneID] = append(byZone[p.ZoneID], topology.ZoneMember{
			ZoneID:    p.ZoneID,
			AssetID:   p.AssetID,
			SortOrder: sortIdx,
		})
		sortIdx++
	}

	// Fetch layout and all existing members once, outside the loop.
	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}
	allMembers, err := s.topologyStore.ListMembers(layout.ID)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list members: "+err.Error())
		return
	}

	// Index existing members by zone for fast lookup.
	existingByZone := make(map[string][]topology.ZoneMember)
	for _, m := range allMembers {
		existingByZone[m.ZoneID] = append(existingByZone[m.ZoneID], m)
	}

	// For each zone, merge existing members with new placements.
	placed := 0
	for zoneID, newMembers := range byZone {
		merged := append([]topology.ZoneMember{}, existingByZone[zoneID]...)

		// Append new members with positions offset from existing count.
		for i, nm := range newMembers {
			nm.SortOrder = len(merged) + i
			merged = append(merged, nm)
		}

		if err := s.topologyStore.SetMembers(zoneID, merged); err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to place assets: "+err.Error())
			return
		}
		placed += len(newMembers)
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "placed",
		"count":  placed,
	})
}

// ---------------------------------------------------------------------------
// POST   /api/v2/topology/dismiss — dismiss an asset from the topology
// DELETE /api/v2/topology/dismiss/{assetId} — undismiss an asset
// ---------------------------------------------------------------------------

func (s *apiServer) handleV2TopologyDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	var req struct {
		AssetID string `json:"asset_id"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}
	if req.AssetID == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset_id is required")
		return
	}

	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}

	if err := s.topologyStore.DismissAsset(layout.ID, req.AssetID); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to dismiss asset: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "dismissed"})
}

func (s *apiServer) handleV2TopologyUndismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "DELETE required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "topology:write") {
		apiv2.WriteScopeForbidden(w, "topology:write")
		return
	}

	assetID := strings.TrimPrefix(r.URL.Path, "/api/v2/topology/dismiss/")
	assetID = strings.TrimRight(assetID, "/")
	if assetID == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset id is required in path")
		return
	}

	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get layout: "+err.Error())
		return
	}

	if err := s.topologyStore.UndismissAsset(layout.ID, assetID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "dismissed asset not found")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to undismiss asset: "+err.Error())
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "undismissed"})
}
