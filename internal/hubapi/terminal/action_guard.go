package terminal

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
)

func (d *Deps) enforceAssetActionGuard(w http.ResponseWriter, assetID string) bool {
	return maintenanceguard.EnforceAssetAction(w, assetID, d.EvaluateAssetGuardrails)
}
