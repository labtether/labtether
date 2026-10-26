package collectors

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Direct and queued actions use the same binding and provider execution. The
// startup registry remains an action catalog, never this dispatch's client.
func (d *Deps) handleConfiguredConnectorAction(w http.ResponseWriter, r *http.Request, connectorID, actionID string, req connectorsdk.ActionRequest) bool {
	configured, ok := d.ConfiguredConnectorActions[connectorID]
	if !ok {
		return false
	}
	if configured.Bind == nil || configured.Execute == nil {
		servicehttp.WriteError(w, http.StatusConflict, "configured connector action unavailable")
		return true
	}
	bound, err := configured.Bind(r.Context(), actionID, req)
	if err != nil {
		if errors.Is(err, connectorsdk.ErrComposeUpgradeRequired) {
			servicehttp.WriteError(w, http.StatusConflict, connectorsdk.ErrComposeUpgradeRequired.Error())
		} else if !maintenanceguard.WriteBlockedAction(w, err) {
			servicehttp.WriteError(w, http.StatusConflict, "cannot bind connector action target")
		}
		return true
	}
	if bound.TargetID != req.TargetID || bound.DryRun != req.DryRun {
		servicehttp.WriteError(w, http.StatusConflict, "connector binding changed the logical request")
		return true
	}
	if !d.prepareConnectorAction(w, connectorID, actionID, &bound) {
		return true
	}
	result, err := configured.Execute(r.Context(), actionID, bound)
	if err != nil {
		if errors.Is(err, connectorsdk.ErrComposeUpgradeRequired) {
			servicehttp.WriteError(w, http.StatusConflict, connectorsdk.ErrComposeUpgradeRequired.Error())
		} else if !maintenanceguard.WriteBlockedAction(w, err) {
			servicehttp.WriteError(w, http.StatusBadGateway, "action execution failed")
		}
		return true
	}
	status := http.StatusOK
	if strings.EqualFold(result.Status, "failed") {
		status = http.StatusBadRequest
	}
	servicehttp.WriteJSON(w, status, map[string]any{"result": result})
	return true
}
