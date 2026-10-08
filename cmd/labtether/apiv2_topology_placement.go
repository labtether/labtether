package main

import (
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/topology"
	"net/http"
	"slices"
	"strings"
)

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
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}

	allAssets, err := s.assetStore.ListAssets()
	if err != nil {
		writeTopologyInternalError(w, "failed to list assets", err)
		return
	}

	zones, err := s.topologyStore.ListZones(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list zones", err)
		return
	}

	members, err := s.topologyStore.ListMembers(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list members", err)
		return
	}

	dismissed, err := s.topologyStore.ListDismissed(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list dismissed", err)
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
		allEdges, edgeErr := s.listTopologyEdges(assetIDs)
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
	if !s.enforceTopologyMutationLimit(w, r) {
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
	if len(req.Placements) > maxTopologyMutationItems {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "placements exceeds maximum of 1000")
		return
	}

	// Group placements by zone.
	byZone := make(map[string][]topology.ZoneMember)
	zoneOrder := make([]string, 0, len(req.Placements))
	seenAssets := make(map[string]struct{}, len(req.Placements))
	sortIdx := 0
	for _, p := range req.Placements {
		if strings.TrimSpace(p.AssetID) == "" || strings.TrimSpace(p.ZoneID) == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "each placement requires asset_id and zone_id")
			return
		}
		assetID := strings.TrimSpace(p.AssetID)
		zoneID := strings.TrimSpace(p.ZoneID)
		if len(assetID) > maxTopologyIdentifierLength || len(zoneID) > maxTopologyIdentifierLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "placement identifiers are too long")
			return
		}
		if _, exists := seenAssets[assetID]; exists {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "each asset may only be placed once per request")
			return
		}
		seenAssets[assetID] = struct{}{}
		if _, ok := byZone[zoneID]; !ok {
			zoneOrder = append(zoneOrder, zoneID)
		}
		byZone[zoneID] = append(byZone[zoneID], topology.ZoneMember{
			ZoneID:    zoneID,
			AssetID:   assetID,
			SortOrder: sortIdx,
		})
		sortIdx++
	}
	if len(zoneOrder) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "placements array is required")
		return
	}

	// Fetch layout and all existing members once, outside the loop.
	layout, err := s.topologyStore.GetOrCreateLayout()
	if err != nil {
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}
	allMembers, err := s.topologyStore.ListMembers(layout.ID)
	if err != nil {
		writeTopologyInternalError(w, "failed to list members", err)
		return
	}

	// Index existing members by zone for fast lookup.
	existingByZone := make(map[string][]topology.ZoneMember)
	for _, m := range allMembers {
		existingByZone[m.ZoneID] = append(existingByZone[m.ZoneID], m)
	}

	// For each zone, merge existing members with new placements.
	placed := 0
	for _, zoneID := range zoneOrder {
		newMembers := byZone[zoneID]
		merged := append([]topology.ZoneMember{}, existingByZone[zoneID]...)

		// Append new members with positions offset from existing count.
		for i, nm := range newMembers {
			nm.SortOrder = len(merged) + i
			merged = append(merged, nm)
		}

		if err := s.topologyStore.SetMembers(zoneID, merged); err != nil {
			writeTopologyInternalError(w, "failed to place assets", err)
			return
		}
		placed += len(newMembers)
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "placed",
		"count":  placed,
	})
}

const topologyEdgeChunkSize = 200

func (s *apiServer) listTopologyEdges(assetIDs []string) ([]edges.Edge, error) {
	if s == nil || s.edgeStore == nil {
		return nil, nil
	}
	trimmedIDs := make([]string, 0, len(assetIDs))
	seenIDs := make(map[string]struct{}, len(assetIDs))
	for _, assetID := range assetIDs {
		assetID = strings.TrimSpace(assetID)
		if assetID == "" {
			continue
		}
		if _, exists := seenIDs[assetID]; exists {
			continue
		}
		seenIDs[assetID] = struct{}{}
		trimmedIDs = append(trimmedIDs, assetID)
	}
	if len(trimmedIDs) == 0 {
		return []edges.Edge{}, nil
	}

	allEdges := make([]edges.Edge, 0)
	seenEdges := make(map[string]struct{})
	for start := 0; start < len(trimmedIDs); start += topologyEdgeChunkSize {
		end := min(start+topologyEdgeChunkSize, len(trimmedIDs))
		chunkEdges, err := s.listTopologyEdgesChunk(trimmedIDs[start:end])
		if err != nil {
			return nil, err
		}
		for _, edge := range chunkEdges {
			if _, exists := seenEdges[edge.ID]; exists {
				continue
			}
			seenEdges[edge.ID] = struct{}{}
			allEdges = append(allEdges, edge)
		}
	}
	slices.SortFunc(allEdges, func(a, b edges.Edge) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return allEdges, nil
}

func (s *apiServer) listTopologyEdgesChunk(assetIDs []string) ([]edges.Edge, error) {
	const topologyEdgeBatchLimit = 50000

	edgesBatch, err := s.edgeStore.ListEdgesBatch(assetIDs, topologyEdgeBatchLimit)
	if err != nil {
		return nil, err
	}
	if len(edgesBatch) < topologyEdgeBatchLimit || len(assetIDs) <= 1 {
		return edgesBatch, nil
	}

	mid := len(assetIDs) / 2
	left, err := s.listTopologyEdgesChunk(assetIDs[:mid])
	if err != nil {
		return nil, err
	}
	right, err := s.listTopologyEdgesChunk(assetIDs[mid:])
	if err != nil {
		return nil, err
	}

	merged := make([]edges.Edge, 0, len(left)+len(right))
	seenEdges := make(map[string]struct{}, len(left)+len(right))
	for _, edge := range left {
		if _, exists := seenEdges[edge.ID]; exists {
			continue
		}
		seenEdges[edge.ID] = struct{}{}
		merged = append(merged, edge)
	}
	for _, edge := range right {
		if _, exists := seenEdges[edge.ID]; exists {
			continue
		}
		seenEdges[edge.ID] = struct{}{}
		merged = append(merged, edge)
	}
	return merged, nil
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
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}

	if err := s.topologyStore.DismissAsset(layout.ID, req.AssetID); err != nil {
		writeTopologyInternalError(w, "failed to dismiss asset", err)
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
		writeTopologyInternalError(w, "failed to get layout", err)
		return
	}

	if err := s.topologyStore.UndismissAsset(layout.ID, assetID); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "dismissed asset not found")
			return
		}
		writeTopologyInternalError(w, "failed to undismiss asset", err)
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "undismissed"})
}
