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

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/pbs"
	"github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
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

	// The PBS and TrueNAS Deps are built once and kept, as cmd keeps
	// apiServer.pbsDeps and truenasDeps, so their runtime and read caches
	// last for the test. See PBSDeps and TrueNASDeps.
	pbsOnce     sync.Once
	pbsDeps     *pbs.Deps
	truenasOnce sync.Once
	truenasDeps *truenas.Deps

	// State cmd's apiServer holds for the desktop and terminal Deps. As on
	// cmd's test server, AgentMgr is nil until a test sets it, and New sets
	// PolicyState to the default policy (NewEmpty leaves it nil).
	AgentMgr       *agentmgr.AgentManager
	PolicyState    *operations.PolicyRuntimeState
	SessionStreams shared.SessionStreamRegistry

	DesktopBridges    sync.Map
	WebRTCBridges     sync.Map
	ClipboardReplies  desktop.ClipboardReplies
	DisplayReplies    desktop.DisplayReplies
	DiagnosticReplies desktop.DiagnosticReplies

	TerminalBookmarkStore   persistence.TerminalBookmarkStore
	TerminalScrollbackStore persistence.TerminalScrollbackStore
	TerminalInMemStore      *terminal.Store
	JobQueue                *jobqueue.Queue
	EphemeralSSHConfigs     *terminalpkg.EphemeralSSHConfigStore
	TerminalBridges         sync.Map
	PendingAgentCmds        sync.Map

	// The desktop and terminal Deps are built once and kept, as cmd keeps
	// apiServer.desktopDeps and terminalDeps; see DesktopDeps and
	// TerminalDeps.
	desktopOnce    sync.Once
	desktopDeps    *desktop.Deps
	terminalOnce   sync.Once
	terminalDeps   *terminalpkg.Deps
	desktopSession struct {
		mu   sync.RWMutex
		opts map[string]desktop.DesktopSessionOptions
	}

	// State cmd's apiServer holds for the Docker Deps. DockerCoordinator is
	// nil until a test sets it, as on cmd's test server. The Docker Deps is
	// built once and kept, as cmd keeps apiServer.dockerDeps.
	DockerCoordinator *docker.Coordinator
	DockerExecBridges sync.Map
	dockerOnce        sync.Once
	dockerDeps        *dockerpkg.Deps

	// State cmd's apiServer holds for the alerting Deps. The three stores are
	// nil until a test sets them, as on cmd's test server. New gives the
	// dispatcher stand-in senders and cmd's limit of 32 sends in flight;
	// NewEmpty leaves it empty, like a bare apiServer.
	NotificationStore      persistence.NotificationStore
	IncidentEventStore     persistence.IncidentEventStore
	DependencyStore        persistence.DependencyStore
	NotificationDispatcher NotificationDispatcher
	alertDevicesOnce       sync.Once
	alertDevices           *alerting.AgentAlertDevices

	// State cmd's apiServer holds for the resources Deps. The four stores
	// and the file protocol pool are nil until a test sets them, as on cmd's
	// test server. New sets WebServiceCoordinator as newTestAPIServer does.
	FailoverStore         persistence.FailoverStore
	EdgeStore             edges.Store
	FileTransferStore     persistence.FileTransferStore
	FileProtoPool         *fileproto.Pool
	WebServiceCoordinator *webservice.Coordinator
	ActiveTransfers       sync.Map
	FileBridges           sync.Map
	ProcessReplies        resources.AgentReplies
	ServiceReplies        resources.AgentReplies
	JournalReplies        resources.AgentReplies
	DiskReplies           resources.AgentReplies
	NetworkReplies        resources.AgentReplies
	WoLPending            resources.WoLPendingRegistry
	PackageReplies        resources.AgentReplies
	CronReplies           resources.AgentReplies
	UsersReplies          resources.AgentReplies

	// State cmd's apiServer holds for the agents Deps. New sets
	// PendingAgents as newTestAPIServer does; PresenceStore and AgentCache
	// are nil until a test sets them, as on cmd's test server. The agents
	// Deps is built once and kept, as cmd keeps apiServer.agentsDeps; see
	// AgentsDeps.
	PendingAgents *agents.PendingAgents
	PresenceStore persistence.PresenceStore
	AgentCache    *agents.AgentCache
	agentsOnce    sync.Once
	agentsDeps    *agents.Deps

	t            *testing.T
	desktopSPICE struct {
		mu      sync.RWMutex
		targets map[string]desktop.DesktopSPICEProxyTarget
	}
}

// New returns a Hub with the stores of cmd's newTestAPIServer.
func New(t *testing.T) *Hub {
	t.Helper()
	h := newHub(t, testutil.NewStores(t))
	h.PolicyState = operations.NewPolicyRuntimeState(policy.DefaultEvaluatorConfig())
	h.NotificationDispatcher.Adapters = notificationSendersNotTested(t)
	h.NotificationDispatcher.DispatchSem = make(chan struct{}, 32)
	h.WebServiceCoordinator = webservice.NewCoordinator(persistence.NewMemoryWebServiceStore())
	h.PendingAgents = agents.NewPendingAgents()
	return h
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
