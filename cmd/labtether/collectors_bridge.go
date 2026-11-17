package main

import (
	"context"

	"github.com/labtether/labtether/internal/assets"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	truenasconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// buildCollectorsDeps constructs the collectors.Deps from the apiServer's fields.
func (s *apiServer) buildCollectorsDeps() *collectorspkg.Deps {
	return &collectorspkg.Deps{
		IntegrationActionsAllowed:   s.integrationActionsOn,
		AssetStore:                  s.assetStore,
		HubCollectorStore:           s.hubCollectorStore,
		CredentialStore:             s.credentialStore,
		SecretsManager:              s.secretsManager,
		TelemetryStore:              s.telemetryStore,
		LogStore:                    s.logStore,
		DependencyStore:             s.dependencyStore,
		ConnectorRegistry:           s.connectorRegistry,
		EvaluateConnectorGuardrails: s.connectorMaintenanceEvaluator(),
		ConfiguredConnectorActions:  s.configuredConnectorActions(),
		RuntimeStore:                s.runtimeStore,
		DB:                          s.db,

		AgentMgr:              s.agentMgr,
		WebServiceCoordinator: s.webServiceCoordinator,
		CollectorDispatchSem:  s.collectorDispatchSem,

		EnforceRateLimit:        s.enforceRateLimit,
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return s.ensureResourcesDeps().ProcessHeartbeatRequest(req)
		},
		PersistCanonicalConnectorSnapshot: func(connectorID, collectorID, displayName, parentAssetID string, connector connectorsdk.Connector, discovered []connectorsdk.Asset) {
			s.persistCanonicalConnectorSnapshot(connectorID, collectorID, displayName, parentAssetID, connector, discovered)
		},
		DetectLinkSuggestions: func() error {
			return s.detectLinkSuggestions()
		},
		ExecuteProxmoxActionDirect: func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return s.ensureProxmoxDeps().ExecuteProxmoxActionDirect(ctx, actionID, req)
		},

		LoadProxmoxRuntime: func(collectorID string) (*proxmoxconnector.Client, error) {
			runtime, err := s.ensureProxmoxDeps().LoadProxmoxRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client(), nil
		},
		LoadTrueNASRuntime: func(collectorID string) (*truenasconnector.Client, error) {
			runtime, err := s.ensureTruenasDeps().LoadTrueNASRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client, nil
		},
		EnsureTrueNASSubscriptionWorker: func(ctx context.Context, collectorID string, client *truenasconnector.Client) {
			runtime, err := s.ensureTruenasDeps().LoadTrueNASRuntime(collectorID)
			if err != nil {
				return
			}
			collector, ok, loadErr := s.hubCollectorStore.GetHubCollector(collectorID)
			if loadErr != nil || !ok {
				return
			}
			s.ensureTruenasDeps().EnsureTrueNASSubscriptionWorker(ctx, collector, runtime)
		},

		BuildKnownHostsHostKeyCallback: shared.BuildKnownHostsHostKeyCallback,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensureCollectorsDeps returns the collectors deps, creating and caching on first call.
func (s *apiServer) ensureCollectorsDeps() *collectorspkg.Deps {
	// Use sync.Once so concurrent callers don't each build their own
	// *Deps with independent CollectorRunState trackers. Before this,
	// concurrent executeCollector calls could each see collectorsDeps==nil,
	// build separate Deps, and bypass singleflight dedup.
	s.collectorsDepsOnce.Do(func() {
		s.collectorsDeps = s.buildCollectorsDeps()
	})
	return s.collectorsDeps
}

// The /connectors/ routes call HandleConnectorActions with no cmd check of
// Settings › Integration actions: the handler checks the switch itself, so
// Test connection stays open while it is off.
