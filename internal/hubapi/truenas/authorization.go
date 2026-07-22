package truenas

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func (d *Deps) requireTrueNASCollectorAccess(w http.ResponseWriter, r *http.Request, collectorID string) bool {
	allowed, err := shared.AllCollectorAssetsAllowed(r.Context(), d.AssetStore, "truenas", collectorID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset managed by this TrueNAS collector")
	return false
}
