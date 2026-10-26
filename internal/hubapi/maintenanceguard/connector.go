package maintenanceguard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type EvaluateConnectorFunc func(connectorID, actionID string, req connectorsdk.ActionRequest, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error)

// ConnectorEvaluator checks stored owners of effective provider targets. It
// does not rewrite the submitted audit target or invent owners for global or
// multi-resource operations. Provider aliases and discovered storage owners
// are mapped by their actual dispatch identity.
type ConnectorEvaluator struct {
	Assets                    persistence.AssetStore
	Collectors                persistence.HubCollectorStore
	Registry                  *connectorsdk.Registry
	EvaluateAsset             EvaluateAssetFunc
	ResolveProxmoxDestination func(actionID, target string, params map[string]string) (node, vmID, collectorID string, err error)
}

func (e ConnectorEvaluator) Evaluate(connectorID, actionID string, req connectorsdk.ActionRequest, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
	connectorID = strings.ToLower(strings.TrimSpace(connectorID))
	if e.Registry != nil {
		if connector, ok := e.Registry.Get(connectorID); ok {
			actionID = modelmap.ResolveActionID(actionID, req.TargetID, connector.Actions())
		}
	}
	targets := []string{strings.TrimSpace(req.TargetID)}
	var actual []string
	var err error
	switch {
	case connectorID == "proxmox" && (strings.HasPrefix(actionID, "vm.") || strings.HasPrefix(actionID, "ct.")):
		actual, err = e.proxmoxTargets(actionID, req)
	case connectorID == "truenas":
		actual, err = e.trueNASTargets(actionID, req)
	case connectorID == "pbs":
		actual, err = e.pbsTargets(actionID, req)
	}
	if err != nil {
		return groupfeatures.GroupMaintenanceGuardrails{}, err
	}
	targets = append(targets, actual...)
	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target == "" || seen[target] || e.EvaluateAsset == nil {
			continue
		}
		seen[target] = true
		guard, err := e.EvaluateAsset(target, at)
		if err != nil || guard.BlockActions {
			return guard, err
		}
	}
	return groupfeatures.GroupMaintenanceGuardrails{}, nil
}

func EnforceConnectorAction(w http.ResponseWriter, connectorID, actionID string, req connectorsdk.ActionRequest, evaluate EvaluateConnectorFunc) bool {
	if evaluate == nil {
		return true
	}
	guard, err := evaluate(connectorID, actionID, req, time.Now().UTC())
	if err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify connector action maintenance target")
		return false
	}
	if guard.BlockActions {
		servicehttp.WriteJSON(w, http.StatusLocked, map[string]any{
			"error": "maintenance_blocked", "message": "actions are blocked by active maintenance windows",
			"group_id": guard.GroupID, "windows": guard.ActiveWindows,
		})
		return false
	}
	return true
}

// CheckConnectorExecution is used by the worker immediately before provider
// dispatch, including jobs that were queued before a maintenance window began.
func CheckConnectorExecution(connectorID, actionID string, req connectorsdk.ActionRequest, evaluate EvaluateConnectorFunc) error {
	if evaluate == nil {
		return nil
	}
	guard, err := evaluate(connectorID, actionID, req, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("cannot verify connector action maintenance target: %w", err)
	}
	if guard.BlockActions {
		return &BlockedActionError{Guard: guard}
	}
	return nil
}

func (e ConnectorEvaluator) storedAsset(id string) (assets.Asset, bool, error) {
	if e.Assets == nil {
		return assets.Asset{}, false, fmt.Errorf("asset inventory unavailable")
	}
	return e.Assets.GetAsset(strings.TrimSpace(id))
}
