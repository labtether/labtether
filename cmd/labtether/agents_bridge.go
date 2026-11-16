package main

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/enrollment"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/telemetry"
)

const agentHeartbeatTelemetryStoreTimeout = 5 * time.Second

func (s *apiServer) appendAgentHeartbeatTelemetry(assetID string, samples []telemetry.MetricSample, source string) {
	if len(samples) == 0 || s.telemetryStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), agentHeartbeatTelemetryStoreTimeout)
	defer cancel()
	if err := s.telemetryStore.AppendSamples(ctx, samples); err != nil {
		securityruntime.Logf("agentws: failed to append %s heartbeat samples for %s: %v", source, assetID, err)
	}
}

// buildAgentsDeps constructs the agents.Deps from the apiServer's fields.
func (s *apiServer) buildAgentsDeps() *agentspkg.Deps {
	enrollmentTransactions, _ := s.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	deps := &agentspkg.Deps{
		AssetStore:             s.assetStore,
		GroupStore:             s.groupStore,
		EnrollmentStore:        s.enrollmentStore,
		EnrollmentTransactions: enrollmentTransactions,
		PresenceStore:          s.presenceStore,
		RuntimeStore:           s.runtimeStore,
		TelemetryStore:         s.telemetryStore,
		LogStore:               s.logStore,
		CredentialStore:        s.credentialStore,
		SecretsManager:         s.secretsManager,

		AgentMgr: s.agentMgr,
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

		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},

		PendingAgents:    s.pendingAgents,
		PendingAgentCmds: &s.pendingAgentCmds,

		HubIdentity: s.currentHubSSHIdentity(),
		CurrentHubIdentity: func() *shared.HubSSHIdentity {
			return s.currentHubSSHIdentity()
		},
		CACertPEM:  s.tlsState.CACertPEM,
		AgentCache: s.agentCache,
		TLSEnabled: s.tlsState.Enabled,

		AgentWebSocketUpgrader: agentWebSocketUpgrader,

		EnforceRateLimit:           s.enforceClientRateLimit,
		WrapAuth:                   s.withAuth,
		WrapAdmin:                  s.withAdminAuth,
		PrincipalActorID:           apiv2.PrincipalActorID,
		AppendAuditEventBestEffort: s.appendAuditEventBestEffort,

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return s.processHeartbeatRequest(req)
		},
		ProcessAuthenticatedAgentHeartbeat: func(agentTokenID string, req assets.HeartbeatRequest) (*assets.Asset, error) {
			if enrollmentTransactions == nil {
				return nil, persistence.ErrAgentEnrollmentTransactionsUnavailable
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			assetEntry, err := enrollmentTransactions.CommitAuthenticatedAgentHeartbeat(ctx, agentTokenID, shared.AgentTokenTTL(), req)
			if err != nil {
				return nil, err
			}
			s.persistCanonicalHeartbeat(assetEntry, req)
			s.appendAgentHeartbeatTelemetry(
				assetEntry.ID,
				telemetry.SamplesFromHeartbeatMetadata(assetEntry.ID, assetEntry.LastSeenAt, req.Metadata),
				"authenticated",
			)
			return &assetEntry, nil
		},
		ProcessExistingOwnerAgentHeartbeat: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			if enrollmentTransactions == nil {
				return nil, persistence.ErrAgentEnrollmentTransactionsUnavailable
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			assetEntry, err := enrollmentTransactions.CommitExistingOwnerAgentHeartbeat(ctx, req)
			if err != nil {
				return nil, err
			}
			s.persistCanonicalHeartbeat(assetEntry, req)
			s.appendAgentHeartbeatTelemetry(
				assetEntry.ID,
				telemetry.SamplesFromHeartbeatMetadata(assetEntry.ID, assetEntry.LastSeenAt, req.Metadata),
				"owner",
			)
			return &assetEntry, nil
		},
		AutoProvisionDockerCollectorIfNeeded: func(agentAssetID string, connectors []agentmgr.ConnectorInfo) {
			s.autoProvisionDockerCollectorIfNeeded(agentAssetID, connectors)
		},
		ResolveHubURL: func(r *http.Request) string {
			return s.resolveHubURL(r)
		},
		DiscoverClientIP: discoverClientIP,
		ResolveHubConnectionSelection: func(r *http.Request) shared.HubConnectionSelection {
			return shared.HubConnectionSelection(s.resolveHubConnectionSelection(r))
		},
		SummarizeUpdateOutput: func(output string) string {
			return summarizeUpdateOutput(output)
		},
		DefaultUpdateAgentTimeout: defaultUpdateAgentTimeout,

		GetAssetTerminalConfig: func(assetID string) (credentials.AssetTerminalConfig, bool, error) {
			return s.credentialStore.GetAssetTerminalConfig(assetID)
		},
		SaveAssetTerminalConfig: func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
			return s.credentialStore.SaveAssetTerminalConfig(cfg)
		},
		HubSSHKeyEnabled:        s.hubSSHKeyEnabled,
		HubSSHKeyMayBeInstalled: s.hubSSHKeyMayBeInstalled,
		SendHubSSHKeyRemoval:    s.sendSSHKeyRemove,
		RemoveHubSSHKeyIfOff:    s.removeHubSSHKeyIfOff,
	}
	if deps.RuntimeStore != nil {
		if overrides, err := deps.RuntimeStore.ListRuntimeSettingOverrides(); err == nil {
			if err := deps.MigrateSensitiveAgentSettingOverrides(overrides); err != nil {
				securityruntime.Logf("agent settings: failed to migrate sensitive overrides: %v", err)
			}
		}
	}
	return deps
}

// ensureAgentsDeps returns the agents deps, creating and caching on first call.
func (s *apiServer) ensureAgentsDeps() *agentspkg.Deps {
	s.agentsDepsOnce.Do(func() {
		s.agentsDeps = s.buildAgentsDeps()
	})
	return s.agentsDeps
}

// Routes, the agent router and other cmd code call the agents package
// through ensureAgentsDeps. The methods below stay because they do more:
// the two Hub key sends hold cmd's Hub identity lock, and hubSchemes reads
// cmd's TLS state.

// sendSSHKeyInstall sends the Hub key without checking the device's Hub SSH
// key setting. Agent connects use syncHubSSHKeyOnConnect, which does.
func (s *apiServer) sendSSHKeyInstall(conn *agentmgr.AgentConn) {
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	s.ensureAgentsDeps().SendSSHKeyInstall(conn)
}

func (s *apiServer) sendSSHKeyRemove(conn *agentmgr.AgentConn) error {
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	return s.ensureAgentsDeps().SendSSHKeyRemove(conn)
}

func (s *apiServer) hubSchemes() (string, string) {
	if s.tlsState.Enabled {
		return "https", "wss"
	}
	return "http", "ws"
}

func resolveApprovedAssetID(agent *agentspkg.PendingAgent, pendingAssetID string) string {
	return agentspkg.ResolveApprovedAssetID(agentspkg.PendingAgentInfo{Hostname: agent.Hostname}, pendingAssetID)
}

// Test-seam: cmd/labtether tests write to agentspkg.PendingEnrollmentAfterFunc directly.

const (
	maxPendingHostnameIDLen           = 64               // mirrors agents.maxPendingHostnameIDLen
	maxPendingEnrollmentAgents        = 200              // mirrors agents.maxPendingEnrollmentAgents
	maxPendingEnrollmentPerIP         = 5                // mirrors agents.maxPendingEnrollmentPerIP
	maxPendingEnrollmentTimeout       = 10 * time.Minute // mirrors agents.maxPendingEnrollmentTimeout
	maxPendingEnrollmentProofMessages = 4                // mirrors agents.maxPendingEnrollmentProofMessages
)
