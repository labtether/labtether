package hubdeps

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
)

// BuildAgentsDeps mirrors cmd's buildAgentsDeps field by field. Like cmd's
// test server, the Hub has no TLS (no CA certificate), no Hub SSH identity
// and no event broadcaster, so events are dropped as cmd drops them when it
// has none. Heartbeats with no agent sign-in go to the real resources Deps,
// as cmd's processHeartbeatRequest does.
//
// Stand-ins replace cmd's route auth and rate limit, and the origin check of
// cmd's agent WebSocket upgrader (see testutil/standins.go). Every field cmd
// fills with its own code is a stand-in that fails the test if reached: the
// two signed-in heartbeat writers, Docker collector auto-provisioning, the
// Hub URL resolvers, the discover caller address, the Hub SSH identity and
// the Hub SSH key pieces. Those are tested in cmd/labtether.
func (h *Hub) BuildAgentsDeps() *agents.Deps {
	enrollmentTransactions, _ := h.EnrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	deps := &agents.Deps{
		AssetStore:             h.AssetStore,
		GroupStore:             h.GroupStore,
		EnrollmentStore:        h.EnrollmentStore,
		EnrollmentTransactions: enrollmentTransactions,
		PresenceStore:          h.PresenceStore,
		RuntimeStore:           h.RuntimeStore,
		TelemetryStore:         h.TelemetryStore,
		LogStore:               h.LogStore,
		CredentialStore:        h.CredentialStore,
		SecretsManager:         h.SecretsManager,

		AgentMgr: h.AgentMgr,
		EnrollmentTokenMaxUses: enrollment.BoundedLimit(
			shared.EnvOrDefaultInt("LABTETHER_ENROLLMENT_TOKEN_MAX_USES", enrollment.DefaultTokenMaxUsesCeiling),
			enrollment.DefaultTokenMaxUsesCeiling,
			enrollment.HardTokenMaxUsesCeiling,
		),
		MaxEnrolledAgents: enrollment.BoundedLimit(
			shared.EnvOrDefaultInt("LABTETHER_MAX_ENROLLED_AGENTS", enrollment.DefaultMaxEnrolledAgents),
			enrollment.DefaultMaxEnrolledAgents,
			enrollment.HardMaxEnrolledAgents,
		),

		Broadcast: func(string, map[string]any) {},

		PendingAgents:    h.PendingAgents,
		PendingAgentCmds: &h.PendingAgentCmds,

		HubIdentity:        nil, // cmd's test server has no Hub SSH identity
		CurrentHubIdentity: hubIdentityNotTested(h.t),
		CACertPEM:          nil, // and no TLS
		AgentCache:         h.AgentCache,
		TLSEnabled:         false,

		// cmd's agentWebSocketUpgrader, with the origin check stand-in.
		AgentWebSocketUpgrader: websocket.Upgrader{CheckOrigin: testutil.CheckNoBrowserOrigin},

		EnforceRateLimit: h.RateLimiter.Enforce,
		WrapAuth:         testutil.RouteAuthNotTested(h.t),
		WrapAdmin:        testutil.RouteAuthNotTested(h.t),
		PrincipalActorID: apiv2.PrincipalActorID,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			shared.AppendAuditEventBestEffort(h.AuditStore, event, logMessage)
		},

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return h.ResourcesDeps().ProcessHeartbeatRequest(req)
		},
		ProcessAuthenticatedAgentHeartbeat: func(_ string, req assets.HeartbeatRequest) (*assets.Asset, error) {
			return agentHeartbeatWriterNotTested(h.t, "agent token", req)
		},
		ProcessExistingOwnerAgentHeartbeat: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return agentHeartbeatWriterNotTested(h.t, "owner token", req)
		},
		AutoProvisionDockerCollectorIfNeeded: dockerAutoProvisionNotTested(h.t),
		ResolveHubURL: func(*http.Request) string {
			h.t.Errorf("a request reached the Hub URL stand-in; cmd's resolver is tested in cmd/labtether")
			return ""
		},
		DiscoverClientIP: discoverClientIPNotTested(h.t),
		ResolveHubConnectionSelection: func(*http.Request) shared.HubConnectionSelection {
			h.t.Errorf("a request reached the Hub connection stand-in; cmd's resolver is tested in cmd/labtether")
			return shared.HubConnectionSelection{}
		},
		// cmd's summarizeUpdateOutput and defaultUpdateAgentTimeout are
		// aliases of these operations names.
		SummarizeUpdateOutput:     operations.SummarizeUpdateOutput,
		DefaultUpdateAgentTimeout: operations.DefaultUpdateAgentTimeout,

		GetAssetTerminalConfig: func(assetID string) (credentials.AssetTerminalConfig, bool, error) {
			return h.CredentialStore.GetAssetTerminalConfig(assetID)
		},
		SaveAssetTerminalConfig: func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
			return h.CredentialStore.SaveAssetTerminalConfig(cfg)
		},
		HubSSHKeyEnabled:        hubSSHKeyNotTested(h.t),
		HubSSHKeyMayBeInstalled: hubSSHKeyInstalledNotTested(h.t),
		SendHubSSHKeyRemoval:    hubSSHKeySendRemovalNotTested(h.t),
		RemoveHubSSHKeyIfOff: func(*agentmgr.AgentConn) {
			h.t.Errorf("an agent reply reached the Hub SSH key removal stand-in; that step is tested in cmd/labtether")
		},
	}
	if deps.RuntimeStore != nil {
		if overrides, err := deps.RuntimeStore.ListRuntimeSettingOverrides(); err == nil {
			if err := deps.MigrateSensitiveAgentSettingOverrides(overrides); err != nil {
				h.t.Logf("agent settings: failed to migrate sensitive overrides: %v", err)
			}
		}
	}
	return deps
}

// AgentsDeps mirrors cmd's ensureAgentsDeps: built on first use and kept,
// so the pending agents, settings state and update tracking it holds last
// for the test. ResetAgentsDeps makes the next call build again.
func (h *Hub) AgentsDeps() *agents.Deps {
	h.agentsOnce.Do(func() { h.agentsDeps = h.BuildAgentsDeps() })
	return h.agentsDeps
}

// ResetAgentsDeps does what cmd tests do by hand to apiServer's agentsDeps
// and agentsDepsOnce.
func (h *Hub) ResetAgentsDeps() {
	h.agentsDeps = nil
	h.agentsOnce = sync.Once{}
}

// errHeartbeatWriterNotTested is what the heartbeat writer stand-ins return,
// so the heartbeat is refused and the agent's connection closed.
var errHeartbeatWriterNotTested = errors.New("signed-in agent heartbeats are not wired in package tests")

// agentHeartbeatWriterNotTested stands in for the two closures in cmd's
// buildAgentsDeps that commit a signed-in agent's heartbeat. They are cmd
// code: they also write the canonical record (cmd's persistCanonicalHeartbeat)
// and the heartbeat's metrics. It fails the test if reached and refuses.
func agentHeartbeatWriterNotTested(t *testing.T, mode string, req assets.HeartbeatRequest) (*assets.Asset, error) {
	t.Errorf("a %s heartbeat for %s reached the heartbeat writer stand-in; it is tested in cmd/labtether", mode, req.AssetID)
	return nil, errHeartbeatWriterNotTested
}

// dockerAutoProvisionNotTested stands in for cmd's
// autoProvisionDockerCollectorIfNeeded, which adds a Docker collector for an
// agent that reports Docker. It fails the test if reached and adds nothing.
func dockerAutoProvisionNotTested(t *testing.T) func(string, []agentmgr.ConnectorInfo) {
	return func(assetID string, _ []agentmgr.ConnectorInfo) {
		t.Errorf("a heartbeat from %s reached the Docker auto-provision stand-in; it is tested in cmd/labtether", assetID)
	}
}

// discoverClientIPNotTested stands in for cmd's discoverClientIP, which
// decides which caller of /api/v1/discover counts as on the LAN. It fails the
// test if reached and reports no known caller, which is never treated as a
// LAN caller.
func discoverClientIPNotTested(t *testing.T) func(*http.Request) string {
	return func(*http.Request) string {
		t.Errorf("a request reached the discover caller stand-in; cmd's rule is tested in cmd/labtether")
		return ""
	}
}

// hubIdentityNotTested stands in for cmd's currentHubSSHIdentity. It fails
// the test if reached and reports no identity, as cmd's test server has none.
func hubIdentityNotTested(t *testing.T) func() *shared.HubSSHIdentity {
	return func() *shared.HubSSHIdentity {
		t.Errorf("a request reached the Hub SSH identity stand-in; it is tested in cmd/labtether")
		return nil
	}
}

// hubSSHKeyInstalledNotTested stands in for cmd's hubSSHKeyMayBeInstalled.
// It fails the test if reached and answers that the key may be on the
// device, the agents package's own answer when it has no check.
func hubSSHKeyInstalledNotTested(t *testing.T) func(string) bool {
	return func(string) bool {
		t.Errorf("a request reached the Hub SSH key state stand-in; that check is tested in cmd/labtether")
		return true
	}
}

// hubSSHKeySendRemovalNotTested stands in for cmd's sendSSHKeyRemove, which
// sends the removal under the Hub's identity lock. It fails the test if
// reached and reports that nothing was sent.
func hubSSHKeySendRemovalNotTested(t *testing.T) func(*agentmgr.AgentConn) error {
	return func(*agentmgr.AgentConn) error {
		t.Errorf("a request reached the Hub SSH key removal stand-in; it is tested in cmd/labtether")
		return errors.New("the Hub SSH key removal is not wired in package tests")
	}
}
