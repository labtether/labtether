package agents

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func denyAssetRestrictedEnrollment(w http.ResponseWriter, r *http.Request) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "API keys limited to some devices cannot access global pending enrollment operations")
	return true
}
