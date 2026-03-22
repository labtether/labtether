package shared

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
)

// SearchDeps holds the dependencies for the unified search handler.
type SearchDeps struct {
	AssetStore persistence.AssetStore
	GroupStore persistence.GroupStore
}

// HandleV2Search handles GET /api/v2/search requests, returning assets and
// groups whose id, name, or platform fields contain the query string.
func (d *SearchDeps) HandleV2Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "search:read") {
		apiv2.WriteScopeForbidden(w, "search:read")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "q parameter required")
		return
	}
	queryLower := strings.ToLower(query)

	results := map[string]any{}

	// Search assets.
	if allAssets, err := d.AssetStore.ListAssets(); err == nil {
		var matchingAssets []map[string]any
		allowed := apiv2.AllowedAssetsFromContext(r.Context())
		for _, a := range allAssets {
			if !apiv2.AssetCheck(allowed, a.ID) {
				continue
			}
			if strings.Contains(strings.ToLower(a.ID), queryLower) ||
				strings.Contains(strings.ToLower(a.Name), queryLower) ||
				strings.Contains(strings.ToLower(a.Platform), queryLower) {
				matchingAssets = append(matchingAssets, map[string]any{
					"id": a.ID, "name": a.Name, "type": "asset",
					"status": a.Status, "platform": a.Platform,
				})
			}
		}
		if len(matchingAssets) > 0 {
			results["assets"] = matchingAssets
		}
	}

	// Search groups.
	if d.GroupStore != nil {
		if groups, err := d.GroupStore.ListGroups(); err == nil {
			var matchingGroups []map[string]any
			for _, g := range groups {
				if strings.Contains(strings.ToLower(g.ID), queryLower) ||
					strings.Contains(strings.ToLower(g.Name), queryLower) {
					matchingGroups = append(matchingGroups, map[string]any{
						"id": g.ID, "name": g.Name, "type": "group",
					})
				}
			}
			if len(matchingGroups) > 0 {
				results["groups"] = matchingGroups
			}
		}
	}

	apiv2.WriteJSON(w, http.StatusOK, results)
}
