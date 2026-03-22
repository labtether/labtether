package resources

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleEdges handles POST (create edge) and GET (list by asset_id).
// Registered at /edges (exact match).
func (d *Deps) HandleEdges(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/edges" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Support both asset_id (single) and asset_ids (comma-separated batch)
		assetIDsRaw := strings.TrimSpace(r.URL.Query().Get("asset_ids"))
		if assetIDsRaw == "" {
			assetIDsRaw = strings.TrimSpace(r.URL.Query().Get("asset_id"))
		}
		if assetIDsRaw == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_id or asset_ids query parameter is required")
			return
		}
		limit := parseLimit(r, 50)
		var edgeList []edges.Edge
		var err error
		if strings.Contains(assetIDsRaw, ",") {
			ids := strings.Split(assetIDsRaw, ",")
			for i := range ids {
				ids[i] = strings.TrimSpace(ids[i])
			}
			edgeList, err = d.EdgeStore.ListEdgesBatch(ids, limit)
		} else {
			edgeList, err = d.EdgeStore.ListEdgesByAsset(assetIDsRaw, limit)
		}
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list edges")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"edges": edgeList})

	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "edges.create", 120, time.Minute) {
			return
		}
		var req edges.CreateEdgeRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid edge payload")
			return
		}
		req.SourceAssetID = strings.TrimSpace(req.SourceAssetID)
		req.TargetAssetID = strings.TrimSpace(req.TargetAssetID)
		if req.SourceAssetID == "" || req.TargetAssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "source_asset_id and target_asset_id are required")
			return
		}
		if req.SourceAssetID == req.TargetAssetID {
			servicehttp.WriteError(w, http.StatusBadRequest, "source_asset_id and target_asset_id must be different")
			return
		}
		if err := validateEdgeRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		edge, err := d.EdgeStore.CreateEdge(req)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				servicehttp.WriteError(w, http.StatusConflict, "edge already exists between these assets")
				return
			}
			log.Printf("edges: CreateEdge error: %v", err)
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create edge")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"edge": edge})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleEdgeByID handles sub-paths under /edges/:
//   - GET/PATCH/DELETE /edges/{id}   — single edge by ID
//   - GET /edges/tree?root={id}&depth={N}   — descendant tree
//   - GET /edges/ancestors?id={id}&depth={N} — ancestor chain
//
// Registered at /edges/ (prefix match).
func (d *Deps) HandleEdgeByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/edges/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "edge path not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	parts := strings.Split(path, "/")

	// GET /edges/tree?root={id}&depth={N}
	if parts[0] == "tree" {
		d.HandleEdgeTree(w, r)
		return
	}

	// GET /edges/ancestors?id={id}&depth={N}
	if parts[0] == "ancestors" {
		d.HandleEdgeAncestors(w, r)
		return
	}

	edgeID := strings.TrimSpace(parts[0])
	if edgeID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "edge id is required")
		return
	}
	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown edge action")
		return
	}

	switch r.Method {
	case http.MethodGet:
		edge, ok, err := d.EdgeStore.GetEdge(edgeID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load edge")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "edge not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"edge": edge})

	case http.MethodPatch:
		var body struct {
			RelationshipType string `json:"relationship_type"`
			Criticality      string `json:"criticality"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid edge patch payload")
			return
		}
		if err := d.EdgeStore.UpdateEdge(edgeID, body.RelationshipType, body.Criticality); err != nil {
			if strings.Contains(err.Error(), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "edge not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update edge")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})

	case http.MethodDelete:
		if err := d.EdgeStore.DeleteEdge(edgeID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				servicehttp.WriteError(w, http.StatusNotFound, "edge not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete edge")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleEdgeTree handles GET /edges/tree?root={id}&depth={N}.
// Called via HandleEdgeByID dispatch.
func (d *Deps) HandleEdgeTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	if root == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "root query parameter is required")
		return
	}
	depth := 3
	if raw := r.URL.Query().Get("depth"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			depth = parsed
			if depth > 10 {
				depth = 10
			}
		}
	}
	nodes, err := d.EdgeStore.Descendants(root, depth)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute edge tree")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"root":  root,
		"depth": depth,
		"nodes": nodes,
	})
}

// HandleEdgeAncestors handles GET /edges/ancestors?id={id}&depth={N}.
// Called via HandleEdgeByID dispatch.
func (d *Deps) HandleEdgeAncestors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	assetID := strings.TrimSpace(r.URL.Query().Get("id"))
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}
	depth := 3
	if raw := r.URL.Query().Get("depth"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			depth = parsed
			if depth > 10 {
				depth = 10
			}
		}
	}
	nodes, err := d.EdgeStore.Ancestors(assetID, depth)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute edge ancestors")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"id":    assetID,
		"depth": depth,
		"nodes": nodes,
	})
}

// validateEdgeRequest performs basic field validation on a CreateEdgeRequest.
func validateEdgeRequest(req edges.CreateEdgeRequest) error {
	validRelTypes := map[string]bool{
		edges.RelContains:    true,
		edges.RelRunsOn:      true,
		edges.RelHostedOn:    true,
		edges.RelDependsOn:   true,
		edges.RelProvidesTo:  true,
		edges.RelConnectedTo: true,
	}
	if !validRelTypes[strings.ToLower(strings.TrimSpace(req.RelationshipType))] {
		return errors.New("relationship_type must be one of: contains, runs_on, hosted_on, depends_on, provides_to, connected_to")
	}
	if req.Direction != "" {
		switch strings.ToLower(strings.TrimSpace(req.Direction)) {
		case edges.DirUpstream, edges.DirDownstream, edges.DirBidirectional:
			// valid
		default:
			return errors.New("direction must be upstream, downstream, or bidirectional")
		}
	}
	if req.Criticality != "" {
		switch strings.ToLower(strings.TrimSpace(req.Criticality)) {
		case edges.CritCritical, edges.CritHigh, edges.CritMedium, edges.CritLow:
			// valid
		default:
			return errors.New("criticality must be critical, high, medium, or low")
		}
	}
	return nil
}
