package main

import "github.com/labtether/labtether/internal/hubapi/maintenanceguard"

// Construct without resolving a provider: buildProxmoxDeps also calls this
// wiring, and the closure must not recursively construct those dependencies.
func (s *apiServer) connectorMaintenanceEvaluator() maintenanceguard.EvaluateConnectorFunc {
	return s.connectorMaintenanceOwner().Evaluate
}

func (s *apiServer) connectorMaintenanceOwner() maintenanceguard.ConnectorEvaluator {
	return maintenanceguard.ConnectorEvaluator{
		Assets: s.assetStore, Collectors: s.hubCollectorStore, Registry: s.connectorRegistry,
		EvaluateAsset: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
		ResolveProxmoxDestination: func(actionID, target string, params map[string]string) (string, string, string, error) {
			return s.ensureProxmoxDeps().ResolveProxmoxActionDestination(actionID, target, params)
		},
	}
}
