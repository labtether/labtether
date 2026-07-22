package resources

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
)

func (d *Deps) enforceAssetActionGuard(w http.ResponseWriter, assetID string) bool {
	return maintenanceguard.EnforceAssetAction(w, assetID, d.EvaluateAssetGuardrails)
}

func (d *Deps) enforceAssetUpdateGuard(w http.ResponseWriter, assetID string) bool {
	return maintenanceguard.EnforceAssetUpdate(w, assetID, d.EvaluateAssetGuardrails)
}
