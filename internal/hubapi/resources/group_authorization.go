package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func denyAssetRestrictedGlobal(w http.ResponseWriter, r *http.Request, object string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset-restricted api keys cannot access global "+object)
	return true
}

func (d *Deps) loadGroupAuthorizationScope(r *http.Request) ([]groups.Group, []assets.Asset, map[string]struct{}, bool) {
	if d.GroupStore == nil || d.AssetStore == nil {
		return nil, nil, nil, false
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		return nil, nil, nil, false
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, nil, nil, false
	}
	return groupList, assetList, shared.AccessibleGroupIDs(r.Context(), groupList, assetList), true
}

func (d *Deps) requireGroupAccess(w http.ResponseWriter, r *http.Request, groupIDs ...string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	_, _, accessible, ok := d.loadGroupAuthorizationScope(r)
	if !ok {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset authorization stores unavailable")
		return false
	}
	for _, groupID := range groupIDs {
		if _, allowed := accessible[strings.TrimSpace(groupID)]; !allowed {
			apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset in this group")
			return false
		}
	}
	return true
}

func filterGroupTreeForAccess(nodes []groups.TreeNode, accessible map[string]struct{}) []groups.TreeNode {
	return filterGroupTreeLevel(nodes, accessible, "", 0)
}

func filterGroupTreeLevel(nodes []groups.TreeNode, accessible map[string]struct{}, parentID string, depth int) []groups.TreeNode {
	filtered := make([]groups.TreeNode, 0, len(nodes))
	for _, node := range nodes {
		groupID := strings.TrimSpace(node.Group.ID)
		if _, ok := accessible[groupID]; !ok {
			filtered = append(filtered, filterGroupTreeLevel(node.Children, accessible, parentID, depth)...)
			continue
		}
		node.Group.ParentGroupID = parentID
		node.Depth = depth
		node.Children = filterGroupTreeLevel(node.Children, accessible, groupID, depth+1)
		filtered = append(filtered, node)
	}
	return filtered
}
