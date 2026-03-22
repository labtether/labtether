package main

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	truenasconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"

	"golang.org/x/crypto/ssh"
)

// buildCollectorsDeps constructs the collectors.Deps from the apiServer's fields.
func (s *apiServer) buildCollectorsDeps() *collectorspkg.Deps {
	return &collectorspkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,
		TelemetryStore:    s.telemetryStore,
		LogStore:          s.logStore,
		DependencyStore:   s.dependencyStore,
		ConnectorRegistry: s.connectorRegistry,
		RuntimeStore:      s.runtimeStore,
		DB:                s.db,

		AgentMgr:              s.agentMgr,
		WebServiceCoordinator: s.webServiceCoordinator,
		CollectorDispatchSem:  s.collectorDispatchSem,

		EnforceRateLimit: s.enforceRateLimit,

		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			return s.processHeartbeatRequest(req)
		},
		PersistCanonicalConnectorSnapshot: func(connectorID, collectorID, displayName, parentAssetID string, connector connectorsdk.Connector, discovered []connectorsdk.Asset) {
			s.persistCanonicalConnectorSnapshot(connectorID, collectorID, displayName, parentAssetID, connector, discovered)
		},
		DetectLinkSuggestions: func() error {
			return s.detectLinkSuggestions()
		},
		ExecuteProxmoxActionDirect: func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return s.executeProxmoxActionDirect(ctx, actionID, req)
		},

		LoadProxmoxRuntime: func(collectorID string) (*proxmoxconnector.Client, error) {
			runtime, err := s.loadProxmoxRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client(), nil
		},
		LoadTrueNASRuntime: func(collectorID string) (*truenasconnector.Client, error) {
			runtime, err := s.loadTrueNASRuntime(collectorID)
			if err != nil {
				return nil, err
			}
			return runtime.Client, nil
		},
		EnsureTrueNASSubscriptionWorker: func(ctx context.Context, collectorID string, client *truenasconnector.Client) {
			runtime, err := s.loadTrueNASRuntime(collectorID)
			if err != nil {
				return
			}
			collector, ok, loadErr := s.hubCollectorStore.GetHubCollector(collectorID)
			if loadErr != nil || !ok {
				return
			}
			s.ensureTrueNASSubscriptionWorker(ctx, collector, runtime)
		},

		BuildKnownHostsHostKeyCallback: shared.BuildKnownHostsHostKeyCallback,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensureCollectorsDeps returns the collectors deps, creating and caching on first call.
func (s *apiServer) ensureCollectorsDeps() *collectorspkg.Deps {
	if s.collectorsDeps != nil {
		return s.collectorsDeps
	}
	d := s.buildCollectorsDeps()
	s.collectorsDeps = d
	return d
}

// Forwarding methods from apiServer to collectors.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleHubCollectors(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleHubCollectors(w, r)
}

func (s *apiServer) handleHubCollectorActions(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleHubCollectorActions(w, r)
}

func (s *apiServer) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleListConnectors(w, r)
}

func (s *apiServer) handleConnectorActions(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleConnectorActions(w, r)
}

func (s *apiServer) handleWebServices(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServices(w, r)
}

func (s *apiServer) handleWebServiceCompat(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceCompat(w, r)
}

func (s *apiServer) handleWebServiceCategories(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceCategories(w, r)
}

func (s *apiServer) handleWebServiceSync(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceSync(w, r)
}

func (s *apiServer) handleWebServiceManual(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceManual(w, r)
}

func (s *apiServer) handleWebServiceManualActions(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceManualActions(w, r)
}

func (s *apiServer) handleWebServiceOverrides(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceOverrides(w, r)
}

func (s *apiServer) handleWebServiceIconLibrary(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceIconLibrary(w, r)
}

func (s *apiServer) handleWebServiceAltURLs(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceAltURLs(w, r)
}

func (s *apiServer) handleWebServiceNeverGroupRules(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceNeverGroupRules(w, r)
}

func (s *apiServer) handleWebServiceGroupingSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceGroupingSettings(w, r)
}

func (s *apiServer) handleWebServiceGroupingSuggestionResponse(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleWebServiceGroupingSuggestionResponse(w, r)
}

func (s *apiServer) runHubCollectorLoop(ctx context.Context) {
	s.ensureCollectorsDeps().RunHubCollectorLoop(ctx)
}

func (s *apiServer) runWebServiceCleanup(ctx context.Context) {
	s.ensureCollectorsDeps().RunWebServiceCleanup(ctx)
}

func (s *apiServer) runHubCollectorNow(collectorID string) error {
	return s.ensureCollectorsDeps().RunHubCollectorNow(collectorID)
}

func (s *apiServer) invalidateWebServiceURLGroupingConfigCache() {
	s.ensureCollectorsDeps().InvalidateWebServiceURLGroupingConfigCache()
}

func (s *apiServer) processAgentWebServiceReport(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureCollectorsDeps().ProcessAgentWebServiceReport(conn, msg)
}

func (s *apiServer) appendConnectorLogEvent(assetID, source, level, message string, fields map[string]string, at time.Time) {
	s.ensureCollectorsDeps().AppendConnectorLogEvent(assetID, source, level, message, fields, at)
}

func (s *apiServer) appendConnectorLogEventWithID(eventID, assetID, source, level, message string, fields map[string]string, at time.Time) {
	s.ensureCollectorsDeps().AppendConnectorLogEventWithID(eventID, assetID, source, level, message, fields, at)
}

// Connector test forwarding methods used by tests in cmd/labtether/.
func (s *apiServer) handleProxmoxConnectorTest(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleProxmoxConnectorTest(w, r)
}
func (s *apiServer) handlePBSConnectorTest(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandlePBSConnectorTest(w, r)
}
func (s *apiServer) handleTrueNASConnectorTest(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleTrueNASConnectorTest(w, r)
}
func (s *apiServer) handlePortainerConnectorTest(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandlePortainerConnectorTest(w, r)
}
func (s *apiServer) handleHomeAssistantConnectorTest(w http.ResponseWriter, r *http.Request) {
	s.ensureCollectorsDeps().HandleHomeAssistantConnectorTest(w, r)
}

// Collector and status methods used by tests and docker_auto_collector.
func (s *apiServer) executeCollector(ctx context.Context, collector hubcollector.Collector) {
	s.ensureCollectorsDeps().ExecuteCollector(ctx, collector)
}
func (s *apiServer) updateCollectorStatus(collectorID, status, errMsg string) {
	s.ensureCollectorsDeps().UpdateCollectorStatus(collectorID, status, errMsg)
}

// Type aliases for collector types used in cmd/labtether/ test files.
type webServiceGroupingSuggestion = collectorspkg.WebServiceGroupingSuggestion
type webServiceURLGroupingConfig = collectorspkg.WebServiceURLGroupingConfig

// Utility function aliases delegating to the collectors package.
func collectorConfigString(config map[string]any, key string) string {
	return collectorspkg.CollectorConfigString(config, key)
}
func normalizeAssetKey(value string) string { return collectorspkg.NormalizeAssetKey(value) }
func sanitizeConnectorErrorMessage(message string, secrets ...string) string {
	return collectorspkg.SanitizeConnectorErrorMessage(message, secrets...)
}
func sanitizeUpstreamError(msg string) string { return collectorspkg.SanitizeUpstreamError(msg) }
func withCanonicalResourceMetadata(source, assetType string, metadata map[string]string) (string, map[string]string) {
	return collectorspkg.WithCanonicalResourceMetadata(source, assetType, metadata)
}
func collectCollectorIdentity(asset assets.Asset) collectorspkg.CollectorIdentity {
	return collectorspkg.CollectCollectorIdentity(asset)
}
func overlapIdentity(left, right map[string]struct{}) int {
	return collectorspkg.OverlapIdentity(left, right)
}

type collectorIdentity = collectorspkg.CollectorIdentity

const redactedConnectorSecret = collectorspkg.RedactedConnectorSecret

type collectorParentAssetRefreshOptions = collectorspkg.CollectorParentAssetRefreshOptions
type collectorsDeps = collectorspkg.Deps

func connectorSnapshotAssetFromHeartbeat(req assets.HeartbeatRequest, kind string) connectorsdk.Asset {
	return collectorspkg.ConnectorSnapshotAssetFromHeartbeat(req, kind)
}

func buildCollectorSSHHostKeyCallback(config map[string]any) (ssh.HostKeyCallback, bool, error) {
	return collectorspkg.BuildCollectorSSHHostKeyCallback(config)
}

func bestRunsOnIdentityTarget(source assets.Asset, targets []assets.Asset, identities map[string]collectorspkg.CollectorIdentity) (string, string, bool) {
	return collectorspkg.BestRunsOnIdentityTarget(source, targets, identities)
}

func bestRunsOnIdentityTargetWithPriority(source assets.Asset, targets []assets.Asset, identities map[string]collectorspkg.CollectorIdentity, priority func(candidate assets.Asset) int) (string, string, bool) {
	return collectorspkg.BestRunsOnIdentityTargetWithPriority(source, targets, identities, priority)
}

func dockerInfraTargetPriority(candidate assets.Asset) int {
	return collectorspkg.DockerInfraTargetPriority(candidate)
}

func proxmoxResourceHeartbeat(resource proxmoxconnector.Resource, collectorID string, latestBackups map[string]time.Time, guestIdentity map[string]string, collectedAt time.Time) (assets.HeartbeatRequest, bool) {
	return collectorspkg.ProxmoxResourceHeartbeat(resource, collectorID, latestBackups, guestIdentity, collectedAt)
}

func (s *apiServer) autoLinkDockerHostsToInfra() error {
	return s.ensureCollectorsDeps().AutoLinkDockerHostsToInfra()
}

func (s *apiServer) autoLinkDockerContainersToHosts() error {
	return s.ensureCollectorsDeps().AutoLinkDockerContainersToHosts()
}

func proxmoxGuestIdentityMetadataFromConfig(config map[string]any) map[string]string {
	return collectorspkg.ProxmoxGuestIdentityMetadataFromConfig(config)
}

func (s *apiServer) tryBeginCollectorRun(collectorID string) bool {
	return s.ensureCollectorsDeps().TryBeginCollectorRun(collectorID)
}

func (s *apiServer) finishCollectorRun(collectorID string) {
	s.ensureCollectorsDeps().FinishCollectorRun(collectorID)
}

func (s *apiServer) executeDockerCollector(ctx context.Context, collector hubcollector.Collector) {
	s.ensureCollectorsDeps().ExecuteDockerCollector(ctx, collector)
}

var collectorExecutorRegistry = collectorspkg.CollectorExecutorRegistry

func (s *apiServer) runPendingCollectors(ctx context.Context) int {
	return s.ensureCollectorsDeps().RunPendingCollectors(ctx)
}

func (s *apiServer) ingestTrueNASAlertLogs(ctx context.Context, client *truenasconnector.Client, fallbackAssetID string) (int, error) {
	return s.ensureCollectorsDeps().IngestTrueNASAlertLogs(ctx, client, fallbackAssetID)
}

func (s *apiServer) executeTrueNASCollector(ctx context.Context, collector hubcollector.Collector) {
	s.ensureCollectorsDeps().ExecuteTrueNASCollector(ctx, collector)
}

func (s *apiServer) refreshCollectorParentAsset(opts collectorspkg.CollectorParentAssetRefreshOptions) (connectorsdk.Asset, bool) {
	return s.ensureCollectorsDeps().RefreshCollectorParentAsset(opts)
}

func (s *apiServer) keepConnectorClusterAssetAlive(collector hubcollector.Collector, source string, discovered int, logPrefix string) (connectorsdk.Asset, bool) {
	return s.ensureCollectorsDeps().KeepConnectorClusterAssetAlive(collector, source, discovered, logPrefix)
}

func newCollectorLifecycle(s *apiServer, collector hubcollector.Collector, source, collectorType string) collectorspkg.CollectorLifecycle {
	return collectorspkg.NewCollectorLifecycle(s.ensureCollectorsDeps(), collector, source, collectorType)
}

func (s *apiServer) executePortainerCollector(ctx context.Context, collector hubcollector.Collector) {
	s.ensureCollectorsDeps().ExecutePortainerCollector(ctx, collector)
}
func firstNonEmptyString(values ...string) string {
	return collectorspkg.FirstNonEmptyString(values...)
}

// Constant aliases.
const (
	webServiceHealthLogSource      = collectorspkg.WebServiceHealthLogSource
	webServiceStatusTransitionKind = collectorspkg.WebServiceStatusTransitionKind
	webServiceUptimeDropKind       = collectorspkg.WebServiceUptimeDropKind
	webServiceUptimeDropThreshold  = collectorspkg.WebServiceUptimeDropThreshold
)

func (s *apiServer) autoLinkPortainerHostsToTrueNASHosts() error {
	return s.ensureCollectorsDeps().AutoLinkPortainerHostsToTrueNASHosts()
}
func (s *apiServer) autoLinkTrueNASHostsToProxmoxGuests() error {
	return s.ensureCollectorsDeps().AutoLinkTrueNASHostsToProxmoxGuests()
}
func normalizeTrueNASStatus(metadata map[string]string) string {
	return collectorspkg.NormalizeTrueNASStatus(metadata)
}
func normalizePortainerStatus(metadata map[string]string) string {
	return collectorspkg.NormalizePortainerStatus(metadata)
}
func normalizeDockerStatus(metadata map[string]string) string {
	return collectorspkg.NormalizeDockerStatus(metadata)
}
func stableConnectorLogID(prefix, key string) string {
	return collectorspkg.StableConnectorLogID(prefix, key)
}
func proxmoxTaskAssetID(task proxmoxconnector.Task, fallback string) string {
	return collectorspkg.ProxmoxTaskAssetID(task, fallback)
}
func proxmoxTaskLevel(task proxmoxconnector.Task) string { return collectorspkg.ProxmoxTaskLevel(task) }
func trueNASAlertMessage(alert map[string]any) string {
	return collectorspkg.TrueNASAlertMessage(alert)
}
func collectorAnyTime(value any) time.Time { return collectorspkg.CollectorAnyTime(value) }
func collectorEndpointIdentity(rawBaseURL string) (string, string) {
	return collectorspkg.CollectorEndpointIdentity(rawBaseURL)
}
func executeWinRMCommand(ctx context.Context, endpoint, user, password, command string, useHTTPS, skipVerify bool, caPEM string) (string, error) {
	return collectorspkg.ExecuteWinRMCommand(ctx, endpoint, user, password, command, useHTTPS, skipVerify, caPEM)
}

const maxWinRMSOAPResponseBytes = collectorspkg.MaxWinRMSOAPResponseBytes

func winrmSOAPRequest(ctx context.Context, client *http.Client, endpoint, user, password, body string) (string, error) {
	return collectorspkg.WinRMSOAPRequest(ctx, client, endpoint, user, password, body)
}
func collectorAnyString(value any) string { return collectorspkg.CollectorAnyString(value) }

type webServiceIconLibraryEntry = collectorspkg.WebServiceIconLibraryEntry

const webServiceURLGroupingModeBalanced = collectorspkg.WebServiceURLGroupingModeBalanced

func parseWebServiceAliasRules(raw string) []collectorspkg.WebServiceAliasRule {
	return collectorspkg.ParseWebServiceAliasRules(raw)
}
func parseWebServicePairRules(raw string) map[string]struct{} {
	return collectorspkg.ParseWebServicePairRules(raw)
}

const webServiceURLGroupingModeConservative = collectorspkg.WebServiceURLGroupingModeConservative

// Getter/setter helpers for webServiceCleanup test vars (direct references to package vars).
func getWebServiceCleanupInterval() time.Duration           { return collectorspkg.WebServiceCleanupInterval }
func setWebServiceCleanupInterval(d time.Duration)          { collectorspkg.WebServiceCleanupInterval = d }
func getWebServiceCleanupStep() func(*collectorspkg.Deps)   { return collectorspkg.WebServiceCleanupStep }
func setWebServiceCleanupStep(fn func(*collectorspkg.Deps)) { collectorspkg.WebServiceCleanupStep = fn }
func (s *apiServer) resolveWebServiceURLGroupingConfig() collectorspkg.WebServiceURLGroupingConfig {
	return s.ensureCollectorsDeps().ResolveWebServiceURLGroupingConfig()
}
