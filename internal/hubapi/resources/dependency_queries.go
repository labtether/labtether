package resources

import (
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strconv"
	"strings"
)

func (d *Deps) HandleAssetBlastRadius(w http.ResponseWriter, r *http.Request, assetID string) {
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireAPIScope(w, r, "topology:read") {
		return
	}
	if !requireAssetAccess(w, r, assetID) {
		return
	}

	maxDepth := 5
	if raw := r.URL.Query().Get("max_depth"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxDepth = parsed
		}
	}

	nodes, err := d.DependencyStore.BlastRadius(assetID, maxDepth)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute blast radius")
		return
	}
	nodes = filterImpactNodesByAssetAccess(r, nodes)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"asset_id": assetID, "nodes": nodes, "max_depth": maxDepth})
}

func (d *Deps) HandleAssetUpstream(w http.ResponseWriter, r *http.Request, assetID string) {
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireAPIScope(w, r, "topology:read") {
		return
	}
	if !requireAssetAccess(w, r, assetID) {
		return
	}

	maxDepth := 5
	if raw := r.URL.Query().Get("max_depth"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxDepth = parsed
		}
	}

	nodes, err := d.DependencyStore.UpstreamCauses(assetID, maxDepth)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to compute upstream causes")
		return
	}
	nodes = filterImpactNodesByAssetAccess(r, nodes)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"asset_id": assetID, "nodes": nodes, "max_depth": maxDepth})
}

type DependencyBatchLister interface {
	ListAssetDependenciesBatch(assetIDs []string, limit int) ([]dependencies.Dependency, error)
}

type DependencySingleLister interface {
	ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error)
}

func ParseDependencyAssetIDs(csv string, singular []string) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0, 16)

	appendID := func(value string) {
		id := strings.TrimSpace(value)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, value := range strings.Split(csv, ",") {
		appendID(value)
	}
	for _, raw := range singular {
		for _, value := range strings.Split(raw, ",") {
			appendID(value)
		}
	}

	return ids
}

func ListAssetDependenciesBatch(store DependencySingleLister, assetIDs []string, limit int) ([]dependencies.Dependency, error) {
	if len(assetIDs) == 0 {
		return []dependencies.Dependency{}, nil
	}
	if limit <= 0 {
		limit = 5000
	}
	if limit > 50000 {
		limit = 50000
	}

	if batchStore, ok := any(store).(DependencyBatchLister); ok {
		return batchStore.ListAssetDependenciesBatch(assetIDs, limit)
	}

	perAssetLimit := limit
	if perAssetLimit < 50 {
		perAssetLimit = 50
	}
	if perAssetLimit > 1000 {
		perAssetLimit = 1000
	}

	merged := make([]dependencies.Dependency, 0)
	seen := make(map[string]struct{})
	for _, assetID := range assetIDs {
		deps, err := store.ListAssetDependencies(assetID, perAssetLimit)
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			if _, exists := seen[dep.ID]; exists {
				continue
			}
			seen[dep.ID] = struct{}{}
			merged = append(merged, dep)
			if len(merged) >= limit {
				return merged, nil
			}
		}
	}
	return merged, nil
}
