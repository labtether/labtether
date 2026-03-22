package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleComposites handles POST /composites (create composite).
// Registered at /composites (exact match).
func (d *Deps) HandleComposites(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/composites" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "composites.create", 60, time.Minute) {
			return
		}
		var req edges.CreateCompositeRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid composite payload")
			return
		}
		req.PrimaryAssetID = strings.TrimSpace(req.PrimaryAssetID)
		if req.PrimaryAssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "primary_asset_id is required")
			return
		}
		if len(req.FacetAssetIDs) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "facet_asset_ids must contain at least one entry")
			return
		}
		for i, id := range req.FacetAssetIDs {
			req.FacetAssetIDs[i] = strings.TrimSpace(id)
			if req.FacetAssetIDs[i] == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, "facet_asset_ids must not contain empty values")
				return
			}
			if req.FacetAssetIDs[i] == req.PrimaryAssetID {
				servicehttp.WriteError(w, http.StatusBadRequest, "facet_asset_ids must not include primary_asset_id")
				return
			}
		}
		composite, err := d.EdgeStore.CreateComposite(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create composite")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"composite": composite})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleCompositeActions handles sub-paths under /composites/:
//   - GET  /composites/{id}                    — fetch composite with members
//   - PATCH /composites/{id}                   — change primary asset
//   - DELETE /composites/{id}/members/{assetId} — detach a member
//
// Registered at /composites/ (prefix match).
func (d *Deps) HandleCompositeActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/composites/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "composite path not found")
		return
	}
	if d.EdgeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "edge store unavailable")
		return
	}

	// Expected forms:
	//   {id}
	//   {id}/members/{assetId}
	parts := strings.SplitN(path, "/", 3)
	compositeID := strings.TrimSpace(parts[0])
	if compositeID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "composite id is required")
		return
	}

	// DELETE /composites/{id}/members/{assetId}
	if len(parts) == 3 && parts[1] == "members" {
		memberAssetID := strings.TrimSpace(parts[2])
		if memberAssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "member asset id is required")
			return
		}
		if r.Method != http.MethodDelete {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := d.EdgeStore.DetachMember(compositeID, memberAssetID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to detach member")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "detached"})
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown composite action")
		return
	}

	switch r.Method {
	case http.MethodGet:
		composite, ok, err := d.EdgeStore.GetComposite(compositeID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load composite")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "composite not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"composite": composite})

	case http.MethodPatch:
		var body struct {
			PrimaryAssetID string `json:"primary_asset_id"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid composite patch payload")
			return
		}
		body.PrimaryAssetID = strings.TrimSpace(body.PrimaryAssetID)
		if body.PrimaryAssetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "primary_asset_id is required")
			return
		}
		if err := d.EdgeStore.ChangePrimary(compositeID, body.PrimaryAssetID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to change primary asset")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
