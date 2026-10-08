package main

import (
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/topology"
	"net/http"
	"strings"
)

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
			writeTopologyInternalError(w, "failed to get layout", err)
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
		writeTopologyInternalError(w, "failed to create zone", err)
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
			writeTopologyInternalError(w, "failed to update zone", err)
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
			writeTopologyInternalError(w, "failed to delete zone", err)
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
	if !s.enforceTopologyMutationLimit(w, r) {
		return
	}
	zoneID = strings.TrimSpace(zoneID)
	if zoneID == "" || len(zoneID) > maxTopologyIdentifierLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "zone_id is invalid")
		return
	}

	var req struct {
		Members []topology.ZoneMember `json:"members"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}
	if len(req.Members) > maxTopologyMutationItems {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "members exceeds maximum of 1000")
		return
	}

	// Set zone_id on all members.
	seenAssets := make(map[string]struct{}, len(req.Members))
	for i := range req.Members {
		assetID := strings.TrimSpace(req.Members[i].AssetID)
		if assetID == "" || len(assetID) > maxTopologyIdentifierLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "each member requires a valid asset_id")
			return
		}
		if _, duplicate := seenAssets[assetID]; duplicate {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "members must not contain duplicate asset_ids")
			return
		}
		seenAssets[assetID] = struct{}{}
		req.Members[i].AssetID = assetID
		req.Members[i].ZoneID = zoneID
	}

	if err := s.topologyStore.SetMembers(zoneID, req.Members); err != nil {
		writeTopologyInternalError(w, "failed to set members", err)
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
	if !s.enforceTopologyMutationLimit(w, r) {
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
	if len(req.Updates) > maxTopologyMutationItems {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "updates exceeds maximum of 1000")
		return
	}
	seenZones := make(map[string]struct{}, len(req.Updates))
	for i := range req.Updates {
		zoneID := strings.TrimSpace(req.Updates[i].ZoneID)
		parentZoneID := strings.TrimSpace(req.Updates[i].ParentZoneID)
		if zoneID == "" || len(zoneID) > maxTopologyIdentifierLength || len(parentZoneID) > maxTopologyIdentifierLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "each update requires valid zone identifiers")
			return
		}
		if _, duplicate := seenZones[zoneID]; duplicate {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "updates must not contain duplicate zone_ids")
			return
		}
		seenZones[zoneID] = struct{}{}
		req.Updates[i].ZoneID = zoneID
		req.Updates[i].ParentZoneID = parentZoneID
	}

	if err := s.topologyStore.ReorderZones(req.Updates); err != nil {
		if errors.Is(err, topology.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "zone not found")
			return
		}
		writeTopologyInternalError(w, "failed to reorder zones", err)
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}
