package agents

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

// Deps holds all dependencies required by the agents handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	AssetStore             persistence.AssetStore
	EnrollmentStore        persistence.EnrollmentStore
	EnrollmentTransactions persistence.AgentEnrollmentTransactionStore
	PresenceStore          persistence.PresenceStore
	RuntimeStore           persistence.RuntimeSettingsStore
	TelemetryStore         persistence.TelemetryStore
	LogStore               persistence.LogStore
	CredentialStore        persistence.CredentialStore
	SecretsManager         *secrets.Manager

	// Agent manager
	AgentMgr *agentmgr.AgentManager

	// Enrollment safety ceilings (hard-bounded by the server at startup).
	EnrollmentTokenMaxUses int
	MaxEnrolledAgents      int

	// Broadcaster for SSE events.
	Broadcast func(eventType string, data map[string]any)

	// Pending agents registry (owned by this package).
	PendingAgents *PendingAgents

	// Agent settings state (owned by this package).
	AgentSettingsState sync.Map // map[assetID]AgentSettingsRuntimeState

	// Pending agent commands (shared with cmd/labtether).
	PendingAgentCmds *sync.Map // map[jobID]shared.PendingAgentCommand

	// Pending typed Docker endpoint probes. The expected connection and asset
	// are retained with each request so an authenticated peer cannot satisfy a
	// probe routed to another agent.
	DockerEndpointTestBridges sync.Map // map[requestID]pendingDockerEndpointTest
	DockerEndpointTestTimeout time.Duration

	// Hub identity for SSH key provisioning.
	HubIdentity *shared.HubSSHIdentity
	// CurrentHubIdentity returns an immutable snapshot of the active identity.
	CurrentHubIdentity func() *shared.HubSSHIdentity

	// CA certificate PEM for enrollment.
	CACertPEM []byte

	// Agent cache for manifest-driven binary distribution.
	AgentCache *AgentCache

	// TLS state.
	TLSEnabled bool

	// WebSocket upgrader for pending enrollment.
	AgentWebSocketUpgrader websocket.Upgrader

	// Auth middleware injected from cmd/labtether.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
	WrapAuth         func(http.HandlerFunc) http.HandlerFunc
	WrapAdmin        func(http.HandlerFunc) http.HandlerFunc

	// Cross-cutting methods injected from cmd/labtether.
	ProcessHeartbeatRequest              func(req assets.HeartbeatRequest) (*assets.Asset, error)
	ProcessAuthenticatedAgentHeartbeat   func(agentTokenID string, req assets.HeartbeatRequest) (*assets.Asset, error)
	ProcessExistingOwnerAgentHeartbeat   func(req assets.HeartbeatRequest) (*assets.Asset, error)
	AutoProvisionDockerCollectorIfNeeded func(agentAssetID string, connectors []agentmgr.ConnectorInfo)
	ResolveHubURL                        func(r *http.Request) string
	ResolveHubConnectionSelection        func(r *http.Request) shared.HubConnectionSelection
	SummarizeUpdateOutput                func(output string) string
	DefaultUpdateAgentTimeout            time.Duration

	// Credential store method helpers.
	GetAssetTerminalConfig  func(assetID string) (credentials.AssetTerminalConfig, bool, error)
	SaveAssetTerminalConfig func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error)
}

// broadcastEvent calls the Broadcast function if non-nil.
func (d *Deps) broadcastEvent(eventType string, data map[string]any) {
	if d.Broadcast != nil {
		d.Broadcast(eventType, data)
	}
}

func (d *Deps) currentHubIdentity() *shared.HubSSHIdentity {
	if d.CurrentHubIdentity != nil {
		return d.CurrentHubIdentity()
	}
	if d.HubIdentity == nil {
		return nil
	}
	identity := *d.HubIdentity
	return &identity
}

// RegisterRoutes registers all agent-related HTTP routes on the given handler map.
func RegisterRoutes(handlers map[string]http.HandlerFunc, d *Deps) {
	handlers["/api/v1/enroll"] = d.HandleEnroll
	handlers["/api/v1/discover"] = d.HandleDiscover
	handlers["/api/v1/agent/binary"] = d.HandleAgentBinary
	handlers["/api/v1/agent/releases/latest"] = d.HandleAgentReleaseLatest
	handlers["/api/v1/agent/install.sh"] = d.HandleAgentInstallScript
	handlers["/api/v1/agent/bootstrap.sh"] = d.HandleAgentBootstrapScript
	handlers["/install.sh"] = d.HandleAgentInstallScript
	handlers["/settings/enrollment"] = d.WrapAdmin(d.HandleEnrollmentTokens)
	handlers["/settings/enrollment/"] = d.WrapAdmin(d.HandleEnrollmentTokenActions)
	handlers["/settings/agent-tokens"] = d.WrapAdmin(d.HandleAgentTokens)
	handlers["/settings/agent-tokens/"] = d.WrapAdmin(d.HandleAgentTokenActions)
	handlers["/settings/tokens/cleanup"] = d.WrapAdmin(d.HandleTokenCleanup)
	handlers["/agents/connected"] = d.WrapAuth(d.HandleConnectedAgents)
	handlers["/agents/presence"] = d.WrapAuth(d.HandleAgentPresence)
	handlers["/api/v1/agents/"] = d.WrapAuth(d.HandleAgentSettingsRoutes)
	handlers["/api/v1/agents/pending"] = d.WrapAuth(d.HandleListPendingAgents)
	handlers["/api/v1/agents/approve"] = d.WrapAuth(d.HandleApproveAgent)
	handlers["/api/v1/agents/reject"] = d.WrapAuth(d.HandleRejectAgent)
	handlers["/api/v1/agent/manifest"] = d.WrapAuth(d.HandleAgentManifest)
	handlers["/api/v1/agent/cache/refresh"] = d.WrapAdmin(d.HandleAgentCacheRefresh)
}

// RegisterWSHandlers registers WebSocket message handlers for agent-related
// agent messages into the shared router.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgHeartbeat] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentHeartbeat(conn, msg)
	}
	router[agentmgr.MsgTelemetry] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentTelemetry(conn, msg)
	}
	router[agentmgr.MsgCommandResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentCommandResult(conn, msg)
	}
	router[agentmgr.MsgLogStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentLogStream(conn, msg)
	}
	router[agentmgr.MsgLogBatch] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentLogBatch(conn, msg)
	}
	router[agentmgr.MsgUpdateProgress] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentUpdateProgress(conn, msg)
	}
	router[agentmgr.MsgUpdateResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentUpdateResult(conn, msg)
	}
	router[agentmgr.MsgSSHKeyInstalled] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentSSHKeyInstalled(conn, msg)
	}
	router[agentmgr.MsgSSHKeyRemoved] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentSSHKeyRemoved(conn, msg)
	}
	router[agentmgr.MsgConfigApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentConfigApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentSettingsApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentSettingsState(conn, msg)
	}
	router[agentmgr.MsgDockerEndpointTestResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerEndpointTestResult(conn, msg)
	}
}
