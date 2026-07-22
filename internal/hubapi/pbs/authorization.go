package pbs

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func denyAssetRestrictedGlobal(w http.ResponseWriter, r *http.Request, object string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset-restricted api keys cannot access global PBS "+object)
	return true
}

func (d *Deps) requirePBSCollectorAccess(w http.ResponseWriter, r *http.Request, collectorID string) bool {
	allowed, err := shared.AllCollectorAssetsAllowed(r.Context(), d.AssetStore, "pbs", collectorID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset managed by this PBS collector")
	return false
}

func (d *Deps) requirePBSAggregateAccess(w http.ResponseWriter, r *http.Request, asset assets.Asset, collectorID string) bool {
	if !shared.HasAssetRestriction(r.Context()) || PBSStoreFromAsset(asset) != "" {
		return true
	}
	return d.requirePBSCollectorAccess(w, r, collectorID)
}

func (d *Deps) requirePBSStoreAccess(w http.ResponseWriter, r *http.Request, asset assets.Asset, collectorID, requestedStore string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	requestedStore = strings.TrimSpace(requestedStore)
	if requestedStore == "" {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "PBS datastore scope cannot be determined")
		return false
	}
	if assetStore := PBSStoreFromAsset(asset); assetStore != "" {
		if assetStore == requestedStore {
			return true
		}
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to this PBS datastore")
		return false
	}
	return d.requirePBSCollectorAccess(w, r, collectorID)
}
