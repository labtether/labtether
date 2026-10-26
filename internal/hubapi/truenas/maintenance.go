package truenas

import (
	"maps"
	"net/http"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
)

// Guard the exact loaded appliance and provider parameters just before a write.
// Collector-wide read authorization remains separate from maintenance policy.
func (d *Deps) enforceMutation(w http.ResponseWriter, asset assets.Asset, runtime *TruenasRuntime, action string, params map[string]string) bool {
	params = maps.Clone(params)
	if params == nil {
		params = make(map[string]string)
	}
	params["collector_id"] = runtime.CollectorID
	return maintenanceguard.EnforceConnectorAction(w, "truenas", action,
		connectorsdk.ActionRequest{TargetID: asset.ID, Params: params}, d.EvaluateConnectorGuardrails)
}
