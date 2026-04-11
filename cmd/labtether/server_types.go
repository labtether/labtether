package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/fileproto"
	actionspkg "github.com/labtether/labtether/internal/hubapi/actionspkg"
	adminpkg "github.com/labtether/labtether/internal/hubapi/admin"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	authpkg "github.com/labtether/labtether/internal/hubapi/auth"
	bulkpkg "github.com/labtether/labtether/internal/hubapi/bulkpkg"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	desktoppkg "github.com/labtether/labtether/internal/hubapi/desktop"
	dockerpkg "github.com/labtether/labtether/internal/hubapi/dockerpkg"
	groupfeaturespkg "github.com/labtether/labtether/internal/hubapi/groupfeatures"
	homeassistantpkg "github.com/labtether/labtether/internal/hubapi/homeassistantpkg"
	logspkg "github.com/labtether/labtether/internal/hubapi/logspkg"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	pbspkg "github.com/labtether/labtether/internal/hubapi/pbs"
	portainerpkg "github.com/labtether/labtether/internal/hubapi/portainer"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	schedulespkg "github.com/labtether/labtether/internal/hubapi/schedulespkg"
	sharedpkg "github.com/labtether/labtether/internal/hubapi/shared"
	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	updatespkg "github.com/labtether/labtether/internal/hubapi/updatespkg"
	webhookspkg "github.com/labtether/labtether/internal/hubapi/webhookspkg"
	whoamipkg "github.com/labtether/labtether/internal/hubapi/whoamipkg"
	workerpkg "github.com/labtether/labtether/internal/hubapi/worker"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/telemetry/bridge"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/topology"
)

// TLSState is a type alias for opspkg.TLSState. It is defined there so that
// internal/hubapi/admin can accept a *TLSState without creating an import
// cycle back to cmd/labtether.
type TLSState = opspkg.TLSState

type apiServer struct {
	db                            *persistence.PostgresStore
	terminalStore                 persistence.TerminalStore
	terminalPersistentStore       persistence.TerminalPersistentSessionStore
	terminalBookmarkStore         persistence.TerminalBookmarkStore
	terminalScrollbackStore       persistence.TerminalScrollbackStore
	terminalInMemStore            *terminal.Store
	auditStore                    persistence.AuditStore
	assetStore                    persistence.AssetStore
	groupStore                    persistence.GroupStore
	groupMaintenanceStore         persistence.GroupMaintenanceStore
	groupProfileStore             persistence.GroupProfileStore
	failoverStore                 persistence.FailoverStore
	credentialStore               persistence.CredentialStore
	canonicalStore                persistence.CanonicalModelStore
	telemetryStore                persistence.TelemetryStore
	logStore                      persistence.LogStore
	actionStore                   persistence.ActionStore
	updateStore                   persistence.UpdateStore
	alertStore                    persistence.AlertStore
	alertInstanceStore            persistence.AlertInstanceStore
	incidentStore                 persistence.IncidentStore
	retentionStore                persistence.RetentionStore
	runtimeStore                  persistence.RuntimeSettingsStore
	authStore                     persistence.AuthStore
	notificationStore             persistence.NotificationStore
	dependencyStore               persistence.DependencyStore
	syntheticStore                persistence.SyntheticStore
	incidentEventStore            persistence.IncidentEventStore
	hubCollectorStore             persistence.HubCollectorStore
	enrollmentStore               persistence.EnrollmentStore
	adminResetStore               persistence.AdminResetStore
	presenceStore                 persistence.PresenceStore
	apiKeyStore                   persistence.APIKeyStore
	scheduleStore                 persistence.ScheduleStore
	webhookStore                  persistence.WebhookStore
	savedActionStore              persistence.SavedActionStore
	edgeStore                     edges.Store
	topologyStore                 topology.Store
	linkSuggestionStore           persistence.LinkSuggestionStore
	secretsManager                *secrets.Manager
	policyState                   *policyRuntimeState
	connectorRegistry             *connectorsdk.Registry
	jobQueue                      *jobqueue.Queue
	authValidator                 *auth.TokenValidator
	oidcRef                       *authpkg.OIDCProviderRef
	agentMgr                      *agentmgr.AgentManager
	broadcaster                   *EventBroadcaster
	dockerCoordinator             *docker.Coordinator
	webServiceCoordinator         *webservice.Coordinator
	bridgeRegistry                *bridge.Registry
	notificationDispatcher        NotificationDispatcher
	collectorDispatchSem          chan struct{}
	hubIdentity                   *hubSSHIdentity
	tlsState                      TLSState
	pendingAgentCmds              sync.Map // map[jobID]pendingAgentCommand
	collectorRunState             sync.Map // map[collectorID]*atomic.Bool
	terminalBridges               sync.Map // map[sessionID]*terminalBridge
	dockerExecBridges             sync.Map // map[sessionID]*dockerExecBridge
	desktopBridges                sync.Map // map[sessionID]*desktopBridge
	webrtcBridges                 sync.Map // map[sessionID]*webrtcSignalingBridge
	displayBridges                sync.Map // map[requestID]*displayBridge
	fileBridges                   sync.Map // map[requestID]*fileBridge
	processBridges                sync.Map // map[requestID]*processBridge for process.list and process.kill
	serviceBridges                sync.Map // map[requestID]*serviceBridge
	journalBridges                sync.Map // map[requestID]*journalBridge
	diskBridges                   sync.Map // map[requestID]*diskBridge
	networkBridges                sync.Map // map[requestID]*networkBridge
	packageBridges                sync.Map // map[requestID]*packageBridge
	cronBridges                   sync.Map // map[requestID]*cronBridge
	usersBridges                  sync.Map // map[requestID]*usersBridge
	clipboardBridges              sync.Map // map[requestID]*clipboardBridge
	desktopDiagnosticWaiters      sync.Map // map[requestID]chan agentmgr.DesktopDiagnosticData
	agentSettingsState            sync.Map // map[assetID]agentSettingsRuntimeState
	rateLimiter                   RateLimiter
	streamTicketStore             StreamTicketStore
	desktopSessionMu              sync.RWMutex
	desktopSessionOpts            map[string]desktoppkg.DesktopSessionOptions // map[sessionID]DesktopSessionOptions
	desktopSPICEMu                sync.RWMutex
	desktopSPICE                  map[string]desktoppkg.DesktopSPICEProxyTarget // map[sessionID]DesktopSPICEProxyTarget
	webServiceURLGroupingCfgMu    sync.RWMutex
	webServiceURLGroupingCfg      webServiceURLGroupingConfig
	webServiceURLGroupingCfgAt    time.Time
	webServiceURLGroupingCfgTTL   time.Duration
	urlGroupingSuggestions        []webServiceGroupingSuggestion
	urlGroupingSuggestionsMu      sync.Mutex
	statusCache                   StatusCache
	linkSuggestionScanMu          sync.Mutex
	linkSuggestionScanLastStarted time.Time
	linkSuggestionScanRunning     atomic.Bool
	collectorsDepsOnce            sync.Once
	collectorsDeps                *collectorspkg.Deps
	agentsDepsOnce                sync.Once
	agentsDeps                    *agentspkg.Deps
	terminalDepsOnce              sync.Once
	terminalDeps                  *terminalpkg.Deps
	desktopDepsOnce               sync.Once
	desktopDeps                   *desktoppkg.Deps
	proxmoxDeps                   *proxmoxpkg.Deps
	pbsDepsOnce                   sync.Once
	pbsDeps                       *pbspkg.Deps
	portainerDepsOnce             sync.Once
	portainerDeps                 *portainerpkg.Deps
	truenasDepsOnce               sync.Once
	truenasDeps                   *truenaspkg.Deps
	authDepsOnce                  sync.Once
	authDeps                      *authpkg.Deps
	alertingDeps                  *alertingpkg.Deps
	dockerDepsOnce                sync.Once
	dockerDeps                    *dockerpkg.Deps
	operationsDepsOnce            sync.Once
	operationsDeps                *opspkg.ExecDeps
	actionsDepsOnce               sync.Once
	actionsDeps                   *actionspkg.Deps
	updatesDepsOnce               sync.Once
	updatesDeps                   *updatespkg.Deps
	bulkDepsOnce                  sync.Once
	bulkDeps                      *bulkpkg.Deps
	whoamiDepsOnce                sync.Once
	whoamiDeps                    *whoamipkg.Deps
	groupFeaturesDepsOnce         sync.Once
	groupFeaturesDeps             *groupfeaturespkg.Deps
	schedulesDepsOnce             sync.Once
	schedulesDeps                 *schedulespkg.Deps
	webhooksDepsOnce              sync.Once
	webhooksDeps                  *webhookspkg.Deps
	logsDepsOnce                  sync.Once
	logsDeps                      *logspkg.Deps
	homeassistantDepsOnce         sync.Once
	homeassistantDeps             *homeassistantpkg.Deps
	adminDepsOnce                 sync.Once
	adminDeps                     *adminpkg.Deps
	workerDepsOnce                sync.Once
	workerDeps                    *workerpkg.Deps
	searchDepsOnce                sync.Once
	searchDeps                    *sharedpkg.SearchDeps
	browserEventsDepsOnce         sync.Once
	browserEventsDeps             *sharedpkg.BrowserEventsDeps
	fileProtoPool                 *fileproto.Pool
	activeTransfers               sync.Map
	agentCache                    *agentspkg.AgentCache
	externalURL                   string
	dataDir                       string
	demoMode                      bool
	demoRateLimiter               *demoSessionRateLimiter
	pendingAgents                 *pendingAgents
	challengeStore                *auth.ChallengeStore
	totpEncryptionKey             []byte
	installStateStore             *installstate.Store
}

// StatusCache is now defined in internal/hubapi/statusagg. The type alias
// keeps the field name on apiServer unchanged so existing code compiles
// without modification.
type StatusCache = statusaggpkg.StatusCache

// RateLimiter bundles the sliding-window rate limit state that was previously
// scattered as three flat fields on apiServer.
type RateLimiter struct {
	Mu       sync.Mutex
	Windows  map[string]rateCounter
	PrunedAt time.Time
}

type rateCounter struct {
	Count   int
	ResetAt time.Time
}

// NotificationDispatcher bundles the notification dispatch runtime state that
// was previously scattered as three flat fields on apiServer.
type NotificationDispatcher struct {
	Adapters    map[string]notifications.Adapter
	DispatchSem chan struct{}
	DispatchWG  sync.WaitGroup
}

// StreamTicketStore bundles the one-time stream-ticket state that was
// previously scattered as two flat fields on apiServer.
type StreamTicketStore struct {
	Mu      sync.Mutex
	Tickets map[string]streamTicket
}

type streamTicket struct {
	SessionID string
	ActorID   string
	Role      string
	ExpiresAt time.Time
}

type oidcAuthState = authpkg.OIDCAuthState

type workerCounters struct {
	processed        atomic.Uint64
	processedActions atomic.Uint64
	processedUpdates atomic.Uint64
}
