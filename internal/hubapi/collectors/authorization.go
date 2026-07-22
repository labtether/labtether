package collectors

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func denyAssetRestrictedGlobal(w http.ResponseWriter, r *http.Request, object string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset-restricted api keys cannot access global "+object)
	return true
}
