package main

import (
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/topology"
	"net/http"
	"strings"
)

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
			writeTopologyInternalError(w, "failed to get layout", err)
			return
		}
		req.TopologyID = layout.ID
	}

	req.UserDefined = true

	conn, err := s.topologyStore.CreateConnection(req)
	if err != nil {
		writeTopologyInternalError(w, "failed to create connection", err)
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
			writeTopologyInternalError(w, "failed to update connection", err)
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
			writeTopologyInternalError(w, "failed to delete connection", err)
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
		writeTopologyInternalError(w, "failed to update viewport", err)
		return
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
