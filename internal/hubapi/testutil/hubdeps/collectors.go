package hubdeps

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	truenasconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

// BuildCollectorsDeps mirrors cmd's buildCollectorsDeps field by field. Like
// cmd's test server, the Hub has no database.
//
// Stand-ins replace cmd's route auth and rate limit (see
// testutil/standins.go) and the one field cmd fills with its own code: the
// Settings switch for integration actions, which fails the test if reached.
// Heartbeats go through the real resources Deps, which stops at its
// canonical heartbeat stand-in. The canonical connector snapshot, link
// suggestions, the maintenance checks, and the Proxmox and TrueNAS
// callbacks are the real package code over the Hub's stores.
func (h *Hub) BuildCollectorsDeps() *collectors.Deps {
	var db *persistence.PostgresStore
	return &collectors.Deps{
		IntegrationActionsAllowed:   integrationActionsSwitchNotTested(h.t),
		AssetStore:                  h.AssetStore,
		HubCollectorStore:           h.HubCollectorStore,
		CredentialStore:             h.CredentialStore,
		SecretsManager:              h.SecretsManager,
		TelemetryStore:              h.TelemetryStore,
		LogStore:                    h.LogStore,
		DependencyStore:             h.DependencyStore,
		ConnectorRegistry:           h.ConnectorRegistry,
		EvaluateConnectorGuardrails: h.ConnectorMaintenanceEvaluator(),
		ConfiguredConnectorActions:  h.ConfiguredConnectorActions(),
		RuntimeStore:                h.RuntimeStore,
		DB:                          db,

		AgentMgr:              h.AgentMgr,
		WebServiceCoordinator: h.WebServiceCoordinator,
		CollectorDispatchSem:  h.CollectorDispatchSem,

		EnforceRateLimit:        h.RateLimiter.Enforce,
		EvaluateAssetGuardrails: h.GroupFeaturesDeps().EvaluateAssetGuardrails,

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return h.ResourcesDeps().ProcessHeartbeatRequest(req)
		},
		// cmd's persistCanonicalConnectorSnapshot and detectLinkSuggestions
		// pass apiServer's stores to these package functions, read at the call.
		PersistCanonicalConnectorSnapshot: func(connectorID, collectorID, displayName, parentAssetID string, connector connectorsdk.Connector, discovered []connectorsdk.Asset) {
			resources.PersistCanonicalConnectorSnapshot(h.CanonicalStore, h.AssetStore, h.ConnectorRegistry,
				connectorID, collectorID, displayName, parentAssetID, connector, discovered)
		},
		DetectLinkSuggestions: func() error {
			return (&shared.LinkSuggestionDeps{AssetStore: h.AssetStore, EdgeStore: h.EdgeStore}).DetectLinkSuggestions()
		},
		ExecuteProxmoxActionDirect: func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return h.ProxmoxDeps().ExecuteProxmoxActionDirect(ctx, actionID, req)
		},

		LoadProxmoxRuntime: func(collectorID string) (*proxmoxconnector.Client, error) {
			runtime, err := h.ProxmoxDeps().LoadProxmoxRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client(), nil
		},
		LoadTrueNASRuntime: func(collectorID string) (*truenasconnector.Client, error) {
			runtime, err := h.TrueNASDeps().LoadTrueNASRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client, nil
		},
		EnsureTrueNASSubscriptionWorker: func(ctx context.Context, collectorID string, _ *truenasconnector.Client) {
			runtime, err := h.TrueNASDeps().LoadTrueNASRuntime(collectorID)
			if err != nil {
				return
			}
			collector, ok, loadErr := h.HubCollectorStore.GetHubCollector(collectorID)
			if loadErr != nil || !ok {
				return
			}
			h.TrueNASDeps().EnsureTrueNASSubscriptionWorker(ctx, collector, runtime)
		},

		BuildKnownHostsHostKeyCallback: shared.BuildKnownHostsHostKeyCallback,

		WrapAuth:  testutil.RouteAuthNotTested(h.t),
		WrapAdmin: testutil.RouteAuthNotTested(h.t),
	}
}

// CollectorsDeps mirrors cmd's ensureCollectorsDeps: built on first use and
// kept, so its run tracking, grouping cache and link scan state last for the
// test, and stores a test sets before the first call are the ones used.
func (h *Hub) CollectorsDeps() *collectors.Deps {
	h.collectorsOnce.Do(func() { h.collectorsDeps = h.BuildCollectorsDeps() })
	return h.collectorsDeps
}

// integrationActionsSwitchNotTested stands in for cmd's integrationActionsOn,
// Settings › Integration actions. That is cmd code; cmd tests it in
// integration_actions_switch_test.go. It fails the test if reached and turns
// connector writes off, so a write is refused.
func integrationActionsSwitchNotTested(t *testing.T) func() bool {
	return func() bool {
		t.Errorf("a connector write reached the integration actions switch stand-in; the switch is tested in cmd/labtether")
		return false
	}
}
