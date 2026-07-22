package updatespkg

import (
	"context"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
)

func updatePlanAllowed(ctx context.Context, plan updates.Plan) bool {
	return shared.AllAssetsAllowed(ctx, plan.Targets...)
}

func requireUpdatePlanAccess(w http.ResponseWriter, r *http.Request, plan updates.Plan) bool {
	if updatePlanAllowed(r.Context(), plan) {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset targeted by this update plan")
	return false
}

func (d *Deps) requireGroupAccess(w http.ResponseWriter, r *http.Request, groupID string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	if d.GroupStore == nil || d.AssetStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset authorization stores unavailable")
		return false
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize group")
		return false
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize group")
		return false
	}
	accessible := shared.AccessibleGroupIDs(r.Context(), groupList, assetList)
	if _, ok := accessible[strings.TrimSpace(groupID)]; ok {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset in this group")
	return false
}
