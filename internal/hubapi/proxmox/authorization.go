package proxmox

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func denyAssetRestrictedGlobal(w http.ResponseWriter, r *http.Request, object string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "API keys limited to some devices cannot access global Proxmox "+object)
	return true
}

func (d *Deps) requireProxmoxCollectorAccess(w http.ResponseWriter, r *http.Request, collectorID string) bool {
	allowed, err := shared.AllCollectorAssetsAllowed(r.Context(), d.AssetStore, "proxmox", collectorID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "This API key does not have access to every device of this Proxmox connection")
	return false
}
