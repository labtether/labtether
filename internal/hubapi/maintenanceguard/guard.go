package maintenanceguard

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/servicehttp"
)

// EvaluateAssetFunc resolves the active group maintenance constraints for an
// asset. The hub injects groupfeatures.Deps.EvaluateAssetGuardrails so every
// operator action uses the same asset-to-group resolution path.
type EvaluateAssetFunc func(assetID string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error)

// EnforceAssetAction evaluates the guardrails immediately before an
// operator-triggered mutation is dispatched. It fails closed on evaluation
// errors and writes the canonical HTTP response when an action is blocked.
func EnforceAssetAction(w http.ResponseWriter, assetID string, evaluate EvaluateAssetFunc) bool {
	return enforceAssetGuardrail(w, assetID, evaluate, false)
}

// EnforceAssetUpdate applies both the general action guard and the dedicated
// update guard. Package install/remove/upgrade requests are update operations,
// so either active maintenance flag blocks dispatch.
func EnforceAssetUpdate(w http.ResponseWriter, assetID string, evaluate EvaluateAssetFunc) bool {
	return enforceAssetGuardrail(w, assetID, evaluate, true)
}

func enforceAssetGuardrail(w http.ResponseWriter, assetID string, evaluate EvaluateAssetFunc, includeUpdates bool) bool {
	if evaluate == nil {
		return true
	}

	guardrails, err := evaluate(assetID, time.Now().UTC())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to evaluate maintenance windows")
		return false
	}
	if !guardrails.BlockActions && (!includeUpdates || !guardrails.BlockUpdates) {
		return true
	}
	message := "actions are blocked by active maintenance windows"
	if includeUpdates && guardrails.BlockUpdates && !guardrails.BlockActions {
		message = "updates are blocked by active maintenance windows"
	}

	servicehttp.WriteJSON(w, http.StatusLocked, map[string]any{
		"error":    "maintenance_blocked",
		"message":  message,
		"group_id": guardrails.GroupID,
		"windows":  guardrails.ActiveWindows,
	})
	return false
}
