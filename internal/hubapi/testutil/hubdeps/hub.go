// Package hubdeps builds a handler package's Deps for that package's own
// tests, with the same wiring cmd/labtether's build*Deps uses in production:
// the real stores (in memory) and the real cross-package callbacks, with
// stand-ins only for cmd's own middleware and agent dispatch.
//
// It is a separate package from testutil because it imports the handler
// packages, and testutil must stay importable from their internal tests.
// Import it only from external test packages (package foo_test).
package hubdeps

import (
	"context"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// Hub stands in for cmd/labtether's apiServer in package tests. Tests change
// its stores the way cmd tests change apiServer fields, and each *Deps call
// builds from the stores as they are at that moment, as cmd's ensure*Deps
// does on a test server.
type Hub struct {
	*testutil.Stores

	RateLimiter   *testutil.RateLimiter
	StreamTickets *testutil.StreamTickets

	// Proxmox, when set, is returned by ProxmoxDeps instead of a fresh
	// build, as apiServer.proxmoxDeps is. Production sets it once at start.
	Proxmox *proxmox.Deps

	t            *testing.T
	desktopSPICE struct {
		mu      sync.RWMutex
		targets map[string]desktop.DesktopSPICEProxyTarget
	}
}

// New returns a Hub with the stores of cmd's newTestAPIServer.
func New(t *testing.T) *Hub {
	t.Helper()
	return newHub(t, testutil.NewStores(t))
}

// NewEmpty returns a Hub with no stores, like a bare &apiServer{} in cmd.
func NewEmpty(t *testing.T) *Hub {
	t.Helper()
	return newHub(t, &testutil.Stores{})
}

func newHub(t *testing.T, stores *testutil.Stores) *Hub {
	return &Hub{
		Stores:        stores,
		RateLimiter:   &testutil.RateLimiter{},
		StreamTickets: &testutil.StreamTickets{},
		t:             t,
	}
}

// GroupFeaturesDeps mirrors cmd's buildGroupFeaturesDeps. cmd caches it with
// sync.Once on first use; this builds a new one per call, which differs only
// for a test that swaps a group store after its first use.
func (h *Hub) GroupFeaturesDeps() *groupfeatures.Deps {
	return &groupfeatures.Deps{
		GroupStore:            h.GroupStore,
		AssetStore:            h.AssetStore,
		LogStore:              h.LogStore,
		ActionStore:           h.ActionStore,
		UpdateStore:           h.UpdateStore,
		GroupMaintenanceStore: h.GroupMaintenanceStore,
		GroupProfileStore:     h.GroupProfileStore,
		AuditStore:            h.AuditStore,
	}
}

// DesktopSPICEDeps returns a desktop.Deps holding only the SPICE proxy
// target map, the part of cmd's buildDesktopDeps that proxmox writes to.
func (h *Hub) DesktopSPICEDeps() *desktop.Deps {
	return &desktop.Deps{
		DesktopSPICEMu: &h.desktopSPICE.mu,
		DesktopSPICE:   &h.desktopSPICE.targets,
	}
}

// ConnectorMaintenanceEvaluator mirrors cmd's connectorMaintenanceOwner:
// the real maintenanceguard evaluator over this Hub's stores, with group
// guardrails from groupfeatures and Proxmox destinations from proxmox.
func (h *Hub) ConnectorMaintenanceEvaluator() maintenanceguard.EvaluateConnectorFunc {
	return maintenanceguard.ConnectorEvaluator{
		Assets: h.AssetStore, Collectors: h.HubCollectorStore, Registry: h.ConnectorRegistry,
		EvaluateAsset: h.GroupFeaturesDeps().EvaluateAssetGuardrails,
		ResolveProxmoxDestination: func(actionID, target string, params map[string]string) (string, string, string, error) {
			return h.ProxmoxDeps().ResolveProxmoxActionDestination(actionID, target, params)
		},
	}.Evaluate
}

// ConfiguredConnectorActions has the same keys and options as cmd's
// configuredConnectorActions. The Bind and Execute calls go to other
// connector packages (Home Assistant, Docker, TrueNAS, PBS, Portainer) that
// this builder does not wire yet; they fail the test and return an error.
func (h *Hub) ConfiguredConnectorActions() map[string]shared.ConfiguredConnectorAction {
	notWired := func(name string) shared.ConfiguredConnectorAction {
		return shared.ConfiguredConnectorAction{
			Bind: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
				h.t.Errorf("%s action binding is not wired in hubdeps", name)
				return connectorsdk.ActionRequest{}, errNotWired(name)
			},
			Execute: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				h.t.Errorf("%s action execution is not wired in hubdeps", name)
				return connectorsdk.ActionResult{}, errNotWired(name)
			},
		}
	}
	ha := notWired("home-assistant")
	docker := notWired("docker")
	docker.BindingParam, docker.UseCallerTimeout = dockerpkg.ActionAgentBindingParam, true
	return map[string]shared.ConfiguredConnectorAction{
		"home-assistant": ha,
		"homeassistant":  ha,
		"docker":         docker,
		"truenas":        notWired("truenas"),
		"pbs":            notWired("pbs"),
		"portainer":      notWired("portainer"),
	}
}

type errNotWired string

func (e errNotWired) Error() string {
	return string(e) + " connector actions are not wired in package tests"
}
