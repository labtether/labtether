package groupfeatures

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) authorizationScope(w http.ResponseWriter, r *http.Request) ([]groups.Group, []assets.Asset, map[string]struct{}, bool) {
	if d.GroupStore == nil || d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset authorization stores unavailable")
		return nil, nil, nil, false
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize group")
		return nil, nil, nil, false
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize group")
		return nil, nil, nil, false
	}
	return groupList, assetList, shared.AccessibleGroupIDs(r.Context(), groupList, assetList), true
}

func (d *Deps) requireGroupAccess(w http.ResponseWriter, r *http.Request, groupID string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	_, _, accessible, ok := d.authorizationScope(w, r)
	if !ok {
		return false
	}
	if _, allowed := accessible[strings.TrimSpace(groupID)]; allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset in this group")
	return false
}
