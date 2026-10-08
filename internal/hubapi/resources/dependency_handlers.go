package resources

import (
	"errors"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (d *Deps) HandleDependencies(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dependencies" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !requireAPIScope(w, r, "topology:read") {
			return
		}
		assetID := strings.TrimSpace(r.URL.Query().Get("asset_id"))
		if assetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_id query parameter is required")
			return
		}
		if !requireAssetAccess(w, r, assetID) {
			return
		}
		deps, err := d.DependencyStore.ListAssetDependencies(assetID, parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list dependencies")
			return
		}
		deps = filterDependenciesByAssetAccess(r, deps)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
	case http.MethodPost:
		if !requireAPIScope(w, r, "topology:write") {
			return
		}
		if !d.EnforceRateLimit(w, r, "dependencies.create", 120, time.Minute) {
			return
		}
		var req dependencies.CreateDependencyRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid dependency payload")
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
		if !requireAssetAccess(w, r, req.SourceAssetID, req.TargetAssetID) {
			return
		}
		if err := ValidateDependencyRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		dep, err := d.DependencyStore.CreateAssetDependency(req)
		if err != nil {
			if err == dependencies.ErrSelfReference {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err == dependencies.ErrDuplicateDependency {
				servicehttp.WriteError(w, http.StatusConflict, "dependency already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create dependency")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"dependency": dep})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleDependencyActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/dependencies/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "dependency path not found")
		return
	}
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}

	parts := strings.Split(path, "/")

	// GET /dependencies/batch?asset_ids=id1,id2&limit=N
	if parts[0] == "batch" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !requireAPIScope(w, r, "topology:read") {
			return
		}

		assetIDs := ParseDependencyAssetIDs(
			r.URL.Query().Get("asset_ids"),
			r.URL.Query()["asset_id"],
		)
		if len(assetIDs) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_ids query parameter is required")
			return
		}
		if !requireAssetAccess(w, r, assetIDs...) {
			return
		}

		limit := 5000
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 50000 {
			limit = 50000
		}

		deps, err := ListAssetDependenciesBatch(d.DependencyStore, assetIDs, limit)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list dependencies")
			return
		}
		deps = filterDependenciesByAssetAccess(r, deps)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"dependencies": deps,
			"asset_ids":    assetIDs,
		})
		return
	}

	// GET /dependencies/graph?root={id}&depth=N
	if parts[0] == "graph" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !requireAPIScope(w, r, "topology:read") {
			return
		}
		root := strings.TrimSpace(r.URL.Query().Get("root"))
		if root == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "root query parameter is required")
			return
		}
		if !requireAssetAccess(w, r, root) {
			return
		}
		depth := 3
		if raw := r.URL.Query().Get("depth"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 10 {
				depth = parsed
			}
		}
		downstream, err := d.DependencyStore.BlastRadius(root, depth)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute blast radius")
			return
		}
		upstream, err := d.DependencyStore.UpstreamCauses(root, depth)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute upstream causes")
			return
		}
		downstream = filterImpactNodesByAssetAccess(r, downstream)
		upstream = filterImpactNodesByAssetAccess(r, upstream)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"root":       root,
			"depth":      depth,
			"downstream": downstream,
			"upstream":   upstream,
		})
		return
	}

	// GET/DELETE /dependencies/{id}
	depID := strings.TrimSpace(parts[0])
	if depID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "dependency path not found")
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown dependency action")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !requireAPIScope(w, r, "topology:read") {
			return
		}
		dep, ok, err := d.DependencyStore.GetAssetDependency(depID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load dependency")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
			return
		}
		if !requireAssetAccess(w, r, dep.SourceAssetID, dep.TargetAssetID) {
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"dependency": dep})
	case http.MethodDelete:
		if !requireAPIScope(w, r, "topology:write") {
			return
		}
		dep, ok, err := d.DependencyStore.GetAssetDependency(depID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load dependency")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
			return
		}
		if !requireAssetAccess(w, r, dep.SourceAssetID, dep.TargetAssetID) {
			return
		}
		if err := d.DependencyStore.DeleteAssetDependency(depID); err != nil {
			if err == dependencies.ErrDependencyNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete dependency")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAssetDependencies handles /assets/{id}/dependencies and sub-paths.
func (d *Deps) HandleAssetDependencies(w http.ResponseWriter, r *http.Request, assetID string, subParts []string) {
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}
	if !requireAssetAccess(w, r, assetID) {
		return
	}

	// /assets/{id}/dependencies
	if len(subParts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if !requireAPIScope(w, r, "topology:read") {
				return
			}
			deps, err := d.DependencyStore.ListAssetDependencies(assetID, parseLimit(r, 50))
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list dependencies")
				return
			}
			deps = filterDependenciesByAssetAccess(r, deps)
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
		case http.MethodPost:
			if !requireAPIScope(w, r, "topology:write") {
				return
			}
			if !d.EnforceRateLimit(w, r, "dependencies.create", 120, time.Minute) {
				return
			}
			var req dependencies.CreateDependencyRequest
			if err := d.DecodeJSONBody(w, r, &req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, "invalid dependency payload")
				return
			}
			if req.SourceAssetID == "" {
				req.SourceAssetID = assetID
			}
			if !requireAssetAccess(w, r, req.SourceAssetID, req.TargetAssetID) {
				return
			}
			if err := ValidateDependencyRequest(req); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Validate referenced assets exist
			if _, ok, err := d.AssetStore.GetAsset(req.SourceAssetID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate source asset")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "source asset not found")
				return
			}
			if _, ok, err := d.AssetStore.GetAsset(req.TargetAssetID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate target asset")
				return
			} else if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "target asset not found")
				return
			}

			dep, err := d.DependencyStore.CreateAssetDependency(req)
			if err != nil {
				if err == dependencies.ErrSelfReference {
					servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
					return
				}
				if err == dependencies.ErrDuplicateDependency {
					servicehttp.WriteError(w, http.StatusConflict, err.Error())
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create dependency")
				return
			}
			servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"dependency": dep})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /assets/{id}/dependencies/{depId}
	if len(subParts) == 1 {
		depID := strings.TrimSpace(subParts[0])
		switch r.Method {
		case http.MethodGet:
			if !requireAPIScope(w, r, "topology:read") {
				return
			}
			dep, ok, err := d.DependencyStore.GetAssetDependency(depID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load dependency")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
				return
			}
			if !requireAssetAccess(w, r, dep.SourceAssetID, dep.TargetAssetID) {
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"dependency": dep})
		case http.MethodDelete:
			if !requireAPIScope(w, r, "topology:write") {
				return
			}
			dep, ok, err := d.DependencyStore.GetAssetDependency(depID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load dependency")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
				return
			}
			if !requireAssetAccess(w, r, dep.SourceAssetID, dep.TargetAssetID) {
				return
			}
			if err := d.DependencyStore.DeleteAssetDependency(depID); err != nil {
				if err == dependencies.ErrDependencyNotFound {
					servicehttp.WriteError(w, http.StatusNotFound, "dependency not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete dependency")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown dependency action")
}

// --- Dependency validation and batch helpers ---

func ValidateDependencyRequest(req dependencies.CreateDependencyRequest) error {
	if strings.TrimSpace(req.SourceAssetID) == "" {
		return errors.New("source_asset_id is required")
	}
	if strings.TrimSpace(req.TargetAssetID) == "" {
		return errors.New("target_asset_id is required")
	}
	if strings.TrimSpace(req.SourceAssetID) == strings.TrimSpace(req.TargetAssetID) {
		return errors.New("source and target asset cannot be the same")
	}
	if dependencies.NormalizeRelationshipType(req.RelationshipType) == "" {
		return errors.New("relationship_type must be runs_on, depends_on, provides_to, or connected_to")
	}
	if req.Direction != "" && dependencies.NormalizeDirection(req.Direction) == "" {
		return errors.New("direction must be upstream, downstream, or bidirectional")
	}
	if req.Criticality != "" && dependencies.NormalizeCriticality(req.Criticality) == "" {
		return errors.New("criticality must be critical, high, medium, or low")
	}
	return nil
}
