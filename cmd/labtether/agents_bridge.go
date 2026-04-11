package main

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/credentials"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// buildAgentsDeps constructs the agents.Deps from the apiServer's fields.
func (s *apiServer) buildAgentsDeps() *agentspkg.Deps {
	return &agentspkg.Deps{
		AssetStore:      s.assetStore,
		EnrollmentStore: s.enrollmentStore,
		PresenceStore:   s.presenceStore,
		RuntimeStore:    s.runtimeStore,
		TelemetryStore:  s.telemetryStore,
		LogStore:        s.logStore,
		CredentialStore: s.credentialStore,

		AgentMgr: s.agentMgr,

		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},

		PendingAgents:    s.pendingAgents,
		PendingAgentCmds: &s.pendingAgentCmds,

		HubIdentity: s.hubIdentity,
		CACertPEM:   s.tlsState.CACertPEM,
		AgentCache:  s.agentCache,
		TLSEnabled:  s.tlsState.Enabled,

		AgentWebSocketUpgrader: agentWebSocketUpgrader,

		EnforceRateLimit: s.enforceRateLimit,
		WrapAuth:         s.withAuth,
		WrapAdmin:        s.withAdminAuth,

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return s.processHeartbeatRequest(req)
		},
		AutoProvisionDockerCollectorIfNeeded: func(agentAssetID string, connectors []agentmgr.ConnectorInfo) {
			s.autoProvisionDockerCollectorIfNeeded(agentAssetID, connectors)
		},
		ResolveHubURL: func(r *http.Request) string {
			return s.resolveHubURL(r)
		},
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
	}
}

// ensureAgentsDeps returns the agents deps, creating and caching on first call.
func (s *apiServer) ensureAgentsDeps() *agentspkg.Deps {
	if s.agentsDeps != nil {
		return s.agentsDeps
	}
	d := s.buildAgentsDeps()
	s.agentsDeps = d
	return d
}

// Forwarding methods from apiServer to agents.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) processAgentHeartbeat(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentHeartbeat(conn, msg)
}

func (s *apiServer) processAgentTelemetry(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentTelemetry(conn, msg)
}

func (s *apiServer) processAgentCommandResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentCommandResult(conn, msg)
}

func (s *apiServer) processAgentLogStream(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentLogStream(conn, msg)
}

func (s *apiServer) processAgentLogBatch(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentLogBatch(conn, msg)
}

func (s *apiServer) processAgentUpdateProgress(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentUpdateProgress(conn, msg)
}

func (s *apiServer) processAgentUpdateResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentUpdateResult(conn, msg)
}

func (s *apiServer) processAgentSSHKeyInstalled(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentSSHKeyInstalled(conn, msg)
}

func (s *apiServer) processAgentSSHKeyRemoved(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentSSHKeyRemoved(conn, msg)
}

func (s *apiServer) processAgentConfigApplied(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentConfigApplied(conn, msg)
}

func (s *apiServer) processAgentSettingsApplied(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentSettingsApplied(conn, msg)
}

func (s *apiServer) processAgentSettingsState(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureAgentsDeps().ProcessAgentSettingsState(conn, msg)
}

func (s *apiServer) sendSSHKeyInstall(conn *agentmgr.AgentConn) {
	s.ensureAgentsDeps().SendSSHKeyInstall(conn)
}

func (s *apiServer) sendSSHKeyRemove(conn *agentmgr.AgentConn) {
	s.ensureAgentsDeps().SendSSHKeyRemove(conn)
}

func (s *apiServer) sendConfigUpdate(conn *agentmgr.AgentConn) {
	s.ensureAgentsDeps().SendConfigUpdate(conn)
}

func (s *apiServer) sendAgentSettingsApply(conn *agentmgr.AgentConn) {
	s.ensureAgentsDeps().SendAgentSettingsApply(conn)
}

func (s *apiServer) executeViaAgent(cmdJob terminal.CommandJob) terminal.CommandResult {
	return s.ensureAgentsDeps().ExecuteViaAgent(cmdJob)
}

func (s *apiServer) executeUpdateViaAgent(jobID string, target string, mode string, packages []string, timeout time.Duration, force bool) agentmgr.CommandResultData {
	return s.ensureAgentsDeps().ExecuteUpdateViaAgent(jobID, target, mode, packages, timeout, force)
}

func (s *apiServer) deliverPendingAgentResult(conn *agentmgr.AgentConn, data agentmgr.CommandResultData) {
	s.ensureAgentsDeps().DeliverPendingAgentResult(conn, data)
}

func (s *apiServer) handlePendingEnrollment(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandlePendingEnrollment(w, r)
}

func (s *apiServer) handleListPendingAgents(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleListPendingAgents(w, r)
}

func (s *apiServer) handleApproveAgent(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleApproveAgent(w, r)
}

func (s *apiServer) handleRejectAgent(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleRejectAgent(w, r)
}

func (s *apiServer) handleEnroll(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleEnroll(w, r)
}

func (s *apiServer) handleDiscover(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleDiscover(w, r)
}

func (s *apiServer) handleEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleEnrollmentTokens(w, r)
}

func (s *apiServer) handleEnrollmentTokenActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleEnrollmentTokenActions(w, r)
}

func (s *apiServer) handleAgentTokens(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentTokens(w, r)
}

func (s *apiServer) handleAgentTokenActions(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentTokenActions(w, r)
}

func (s *apiServer) handleTokenCleanup(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleTokenCleanup(w, r)
}

func (s *apiServer) handleAgentBinary(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentBinary(w, r)
}

func (s *apiServer) handleAgentReleaseLatest(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentReleaseLatest(w, r)
}

func (s *apiServer) handleAgentInstallScript(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentInstallScript(w, r)
}

func (s *apiServer) handleAgentBootstrapScript(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentBootstrapScript(w, r)
}

func (s *apiServer) handleAgentSettingsRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentSettingsRoutes(w, r)
}

func (s *apiServer) handleConnectedAgents(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleConnectedAgents(w, r)
}

func (s *apiServer) handleAgentPresence(w http.ResponseWriter, r *http.Request) {
	s.ensureAgentsDeps().HandleAgentPresence(w, r)
}

func (s *apiServer) sendShutdownToAgents() {
	s.ensureAgentsDeps().SendShutdownToAgents()
}

func (s *apiServer) hubSchemes() (string, string) {
	if s.tlsState.Enabled {
		return "https", "wss"
	}
	return "http", "ws"
}

func (s *apiServer) resolvePublicHubHost(r *http.Request) string {
	return s.ensureAgentsDeps().ResolvePublicHubHost(r)
}

func (s *apiServer) verifyPendingEnrollmentProof(agent *pendingAgent, msg agentmgr.Message) error {
	return s.ensureAgentsDeps().VerifyPendingEnrollmentProof(agent, msg)
}

func (s *apiServer) sendPendingEnrollmentChallenge(agent *pendingAgent) error {
	return s.ensureAgentsDeps().SendPendingEnrollmentChallenge(agent)
}

func (s *apiServer) pushAgentSettingsApply(assetID string, values map[string]string) {
	s.ensureAgentsDeps().PushAgentSettingsApply(assetID, values)
}

func (s *apiServer) collectEffectiveAgentSettingValues(assetID string) (map[string]string, error) {
	return s.ensureAgentsDeps().CollectEffectiveAgentSettingValues(assetID)
}

func (s *apiServer) getAgentSettingsRuntimeState(assetID string) (agentspkg.AgentSettingsRuntimeState, bool) {
	return s.ensureAgentsDeps().GetAgentSettingsRuntimeState(assetID)
}

func (s *apiServer) setAgentSettingsRuntimeState(assetID string, state agentspkg.AgentSettingsRuntimeState) {
	s.ensureAgentsDeps().SetAgentSettingsRuntimeState(assetID, state)
}

func (s *apiServer) buildAgentSettingsPayload(assetID string) (agentspkg.AgentSettingsPayload, error) {
	return s.ensureAgentsDeps().BuildAgentSettingsPayload(assetID)
}

func (s *apiServer) handleAgentSettingsHistory(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAgentsDeps().HandleAgentSettingsHistory(w, r, assetID)
}

func (s *apiServer) handleAgentSettingsGet(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAgentsDeps().HandleAgentSettingsGet(w, r, assetID)
}

func (s *apiServer) handleAgentSettingsPatch(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAgentsDeps().HandleAgentSettingsPatch(w, r, assetID)
}

func (s *apiServer) handleAgentSettingsReset(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAgentsDeps().HandleAgentSettingsReset(w, r, assetID)
}

// Type aliases for types used in cmd/labtether/ files.
type pendingAgents = agentspkg.PendingAgents
type pendingAgent = agentspkg.PendingAgent
type pendingAgentInfo = agentspkg.PendingAgentInfo
type agentSettingsRuntimeState = agentspkg.AgentSettingsRuntimeState

func newPendingAgents() *agentspkg.PendingAgents {
	return agentspkg.NewPendingAgents()
}

type agentsDeps = agentspkg.Deps
type agentSettingEntry = agentspkg.AgentSettingEntry
type agentSettingsPayload = agentspkg.AgentSettingsPayload
type agentSettingsViewState = agentspkg.AgentSettingsViewState

type pendingAgentCommand = shared.PendingAgentCommand

func configuredAgentTokenTTLHours() int           { return agentspkg.ConfiguredAgentTokenTTLHours() }
func newAgentTokenExpiry(now time.Time) time.Time { return agentspkg.NewAgentTokenExpiry(now) }
func normalizeHostnameForAssetID(hostname string) string {
	return agentspkg.NormalizeHostnameForAssetID(hostname)
}
func buildPendingEnrollmentAssetID(hostname string) string {
	return agentspkg.BuildPendingEnrollmentAssetID(hostname)
}
func sanitizePendingIdentityHeader(raw string, maxLen int) string {
	return agentspkg.SanitizePendingIdentityHeader(raw, maxLen)
}
func decodePendingEnrollmentAssetID(w http.ResponseWriter, r *http.Request) (string, bool) {
	return agentspkg.DecodePendingEnrollmentAssetID(w, r)
}
func resolveApprovedAssetID(agent *pendingAgent, pendingAssetID string) string {
	return agentspkg.ResolveApprovedAssetID(agent, pendingAssetID)
}
func sendPendingEnrollmentDecision(agent *pendingAgent, msgType string, data any, closeReason string) error {
	return agentspkg.SendPendingEnrollmentDecision(agent, msgType, data, closeReason)
}
func normalizeAgentSettingValues(values map[string]string, forHubApply bool) (map[string]string, error) {
	return agentspkg.NormalizeAgentSettingValues(values, forHubApply)
}
func parseIntSafe(s string) int { return agentspkg.ParseIntSafe(s) }
func mergeAgentSettingsReportState(previous, reported agentSettingsRuntimeState) agentSettingsRuntimeState {
	return agentspkg.MergeAgentSettingsReportState(previous, reported)
}
func sameAgentSettingsRevision(left, right string) bool {
	return agentspkg.SameAgentSettingsRevision(left, right)
}
func preservesAgentSettingsApplyStatus(status string) bool {
	return agentspkg.PreservesAgentSettingsApplyStatus(status)
}
func dockerConnectivityTestCommand(endpoint string) string {
	return agentspkg.DockerConnectivityTestCommand(endpoint)
}
func trimUnixEndpointScheme(endpoint string) (string, bool) {
	return agentspkg.TrimUnixEndpointScheme(endpoint)
}
func sanitizeSHA256Hex(raw string) (string, bool) { return agentspkg.SanitizeSHA256Hex(raw) }
func generateInstallScript(hubURL, wsURL string) string {
	return agentspkg.GenerateInstallScript(hubURL, wsURL)
}
func determineAgentVersionStatus(currentVersion, latestVersion string) string {
	return agentspkg.DetermineAgentVersionStatus(currentVersion, latestVersion)
}
func normalizeAgentReleaseOS(raw string) string   { return agentspkg.NormalizeAgentReleaseOS(raw) }
func normalizeAgentReleaseArch(raw string) string { return agentspkg.NormalizeAgentReleaseArch(raw) }
func agentVersionFromBuildInfo(mainVersion string, settings []debug.BuildSetting) string {
	return agentspkg.AgentVersionFromBuildInfo(mainVersion, settings)
}
func agentSettingGlobalDefaultKey(key string) (string, bool) {
	return agentspkg.AgentSettingGlobalDefaultKey(key)
}
func cloneAgentSettingValues(values map[string]string) map[string]string {
	return agentspkg.CloneAgentSettingValues(values)
}
func zeroTimeToRFC3339(t time.Time) string { return agentspkg.ZeroTimeToRFC3339(t) }
func agentSettingStoreKey(assetID, key string) string {
	return agentspkg.AgentSettingStoreKey(assetID, key)
}

// Test-seam: cmd/labtether tests write to agentspkg.PendingEnrollmentAfterFunc directly.

const (
	maxPendingHostnameIDLen     = 64               // mirrors agents.maxPendingHostnameIDLen
	maxPendingEnrollmentAgents  = 200              // mirrors agents.maxPendingEnrollmentAgents
	maxPendingEnrollmentPerIP   = 5                // mirrors agents.maxPendingEnrollmentPerIP
	maxPendingEnrollmentTimeout = 10 * time.Minute // mirrors agents.maxPendingEnrollmentTimeout
)
