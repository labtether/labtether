package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/dependencies"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// buildResourcesDeps constructs the resources.Deps from the apiServer's fields.
func (s *apiServer) buildResourcesDeps() *respkg.Deps {
	return &respkg.Deps{
		AgentMgr:            s.agentMgr,
		AssetStore:          s.assetStore,
		GroupStore:          s.groupStore,
		GroupProfileStore:   s.groupProfileStore,
		FailoverStore:       s.failoverStore,
		TelemetryStore:      s.telemetryStore,
		LogStore:            s.logStore,
		DependencyStore:     s.dependencyStore,
		EdgeStore:           s.edgeStore,
		SyntheticStore:      s.syntheticStore,
		LinkSuggestionStore: s.linkSuggestionStore,
		CredentialStore:     s.credentialStore,
		AuditStore:          s.auditStore,
		RetentionStore:      s.retentionStore,
		DB:                  s.db,
		FileProtoPool:       s.fileProtoPool,
		FileConnectionStore: s.db,
		FileTransferStore:   s.db,
		ActiveTransfers:     &s.activeTransfers,
		RemoteBookmarkStore: s.db,

		FileBridges:    &s.fileBridges,
		ProcessBridges: &s.processBridges,
		ServiceBridges: &s.serviceBridges,
		JournalBridges: &s.journalBridges,
		DiskBridges:    &s.diskBridges,
		NetworkBridges: &s.networkBridges,
		PackageBridges: &s.packageBridges,
		CronBridges:    &s.cronBridges,
		UsersBridges:   &s.usersBridges,

		WrapAuth: s.withAuth,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},

		EnforceRateLimit:  s.enforceRateLimit,
		PrincipalActorID:  func(ctx context.Context) string { return principalActorID(ctx) },
		UserIDFromContext: func(ctx context.Context) string { return userIDFromContext(ctx) },
		SecretsManager:    s.secretsManager,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},

		ManualDeviceDB: func() respkg.ManualDeviceExecer {
			if s.db != nil {
				return s.db.Pool()
			}
			return nil
		}(),

		// Heartbeat and delete cascade dependencies.
		EnrollmentStore:   s.enrollmentStore,
		HubCollectorStore: s.hubCollectorStore,
		RemoveDockerHost: func(assetID string) {
			if s.dockerCoordinator != nil {
				s.dockerCoordinator.RemoveHost(assetID)
			}
		},
		RemoveWebServiceHost: func(assetID string) {
			if s.webServiceCoordinator != nil {
				s.webServiceCoordinator.RemoveHost(assetID)
			}
		},
		SendSSHKeyRemoveToAsset: func(assetID string) {
			if s.hubIdentity == nil || s.agentMgr == nil {
				return
			}
			if conn, ok := s.agentMgr.Get(assetID); ok {
				s.sendSSHKeyRemove(conn)
			}
		},
		PersistCanonicalHeartbeatFn: func(assetEntry assets.Asset, req assets.HeartbeatRequest) {
			s.persistCanonicalHeartbeat(assetEntry, req)
		},
		CollectorConfigString: collectorConfigString,
		NormalizeAssetKey:     normalizeAssetKey,
		AutoDockerCollectorAssetID: func(agentAssetID string) string {
			return autoDockerCollectorAssetID(agentAssetID)
		},

		// Asset sub-handler callbacks for routes that have not yet been
		// extracted from cmd/labtether into the resources package.
		HandleDesktopCredentials:         s.handleDesktopCredentials,
		HandleRetrieveDesktopCredentials: s.handleRetrieveDesktopCredentials,
		HandleDisplayList:                s.handleDisplayList,
		HandlePushHubKey:                 s.handlePushHubKey,
		HandleTestProtocolConnection:     s.handleTestProtocolConnection,
		HandleListProtocolConfigs:        s.handleListProtocolConfigs,
		HandleCreateProtocolConfig:       s.handleCreateProtocolConfig,
		HandleUpdateProtocolConfig:       s.handleUpdateProtocolConfig,
		HandleDeleteProtocolConfig:       s.handleDeleteProtocolConfig,
	}
}

// ensureResourcesDeps returns a resources.Deps built from the current apiServer
// fields. It rebuilds on every call so that callers see the current state of
// mutable fields (e.g. groupStore set to nil in tests or after reconfiguration).
// buildResourcesDeps is cheap — it copies interface values and function pointers.
func (s *apiServer) ensureResourcesDeps() *respkg.Deps {
	return s.buildResourcesDeps()
}

// Forwarding methods from apiServer to resources.Deps so that existing
// cmd/labtether callers keep compiling.

func (s *apiServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFiles(w, r)
}

func (s *apiServer) handleProcesses(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleProcesses(w, r)
}

func (s *apiServer) handleServices(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleServices(w, r)
}

func (s *apiServer) handleDisks(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleDisks(w, r)
}

func (s *apiServer) handleNetworks(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleNetworks(w, r)
}

func (s *apiServer) handlePackages(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandlePackages(w, r)
}

func (s *apiServer) handleCrons(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleCrons(w, r)
}

func (s *apiServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleUsers(w, r)
}

func (s *apiServer) handleJournalLogs(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleJournalLogs(w, r)
}

func (s *apiServer) handleFrontendPerfTelemetry(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFrontendPerfTelemetry(w, r)
}

func (s *apiServer) handleMobileClientTelemetry(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleMobileClientTelemetry(w, r)
}

func (s *apiServer) handleRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleRemoteBookmarks(w, r)
}

// WS handler forwarding.

func (s *apiServer) processAgentFileListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentFileListed(conn, msg)
}

func (s *apiServer) processAgentFileData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentFileData(conn, msg)
}

func (s *apiServer) processAgentFileWritten(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentFileWritten(conn, msg)
}

func (s *apiServer) processAgentFileResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentFileResult(conn, msg)
}

func (s *apiServer) processAgentProcessListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentProcessListed(conn, msg)
}

func (s *apiServer) processAgentProcessKillResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentProcessKillResult(conn, msg)
}

func (s *apiServer) processAgentServiceListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentServiceListed(conn, msg)
}

func (s *apiServer) processAgentServiceResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentServiceResult(conn, msg)
}

func (s *apiServer) processAgentDiskListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentDiskListed(conn, msg)
}

func (s *apiServer) processAgentNetworkListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentNetworkListed(conn, msg)
}

func (s *apiServer) processAgentNetworkResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentNetworkResult(conn, msg)
}

func (s *apiServer) processAgentPackageListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentPackageListed(conn, msg)
}

func (s *apiServer) processAgentPackageResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentPackageResult(conn, msg)
}

func (s *apiServer) processAgentCronListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentCronListed(conn, msg)
}

func (s *apiServer) processAgentUsersListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentUsersListed(conn, msg)
}

func (s *apiServer) processAgentJournalEntries(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentJournalEntries(conn, msg)
}

// Type aliases for bridge types used in cmd/labtether/.
type uploadRelaySendError = respkg.UploadRelaySendError
type fileBridge = respkg.FileBridge
type processBridge = respkg.ProcessBridge
type serviceBridge = respkg.ServiceBridge
type journalBridge = respkg.JournalBridge
type diskBridge = respkg.DiskBridge
type networkBridge = respkg.NetworkBridge
type packageBridge = respkg.PackageBridge
type cronBridge = respkg.CronBridge
type usersBridge = respkg.UsersBridge

func newFileBridge(buffer int, expectedAssetID string) *fileBridge {
	return respkg.NewFileBridge(buffer, expectedAssetID)
}

func generateRequestID() string { return respkg.GenerateRequestID() }

// Function aliases for test accessibility.
func parseProcessListLimit(raw string) int             { return respkg.ParseProcessListLimit(raw) }
func normalizeProcessSortBy(raw string) string         { return respkg.NormalizeProcessSortBy(raw) }
func normalizeProcessSignal(raw string) (string, bool) { return respkg.NormalizeProcessSignal(raw) }
func parseProcessCommandOutput(output string) []agentmgr.ProcessInfo {
	return respkg.ParseProcessCommandOutput(output)
}
func normalizeFrontendPerfMetadata(input map[string]any) map[string]string {
	return respkg.NormalizeFrontendPerfMetadata(input)
}
func sanitizeMobileTelemetryKey(value string) string { return respkg.SanitizeMobileTelemetryKey(value) }

// Constant aliases.
const (
	defaultProcessListLimit = respkg.DefaultProcessListLimit
	maxProcessListLimit     = respkg.MaxProcessListLimit
)

func decodeFileDownloadChunk(payload agentmgr.FileDataPayload) ([]byte, error) {
	return respkg.DecodeFileDownloadChunk(payload)
}

func relayFileUploadChunks(body io.Reader, requestID, filePath string, chunkSize int, send func(agentmgr.FileWriteData) error) (int64, error) {
	return respkg.RelayFileUploadChunks(body, requestID, filePath, chunkSize, send)
}

// Forwarding for test-referenced internal methods.
func (s *apiServer) handleFileList(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileList(w, r, assetID)
}
func (s *apiServer) handleFileDownload(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileDownload(w, r, assetID)
}
func (s *apiServer) handleFileDownloadWithTimeout(w http.ResponseWriter, r *http.Request, assetID string, timeout time.Duration) {
	s.ensureResourcesDeps().HandleFileDownloadWithTimeout(w, r, assetID, timeout)
}
func (s *apiServer) handleFileUpload(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileUpload(w, r, assetID)
}
func (s *apiServer) handleFileMkdir(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileMkdir(w, r, assetID)
}
func (s *apiServer) handleFileDelete(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileDelete(w, r, assetID)
}
func (s *apiServer) handleFileRename(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileRename(w, r, assetID)
}
func (s *apiServer) handleFileCopy(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleFileCopy(w, r, assetID)
}

func (s *apiServer) deliverFileResponse(requestID string, msg agentmgr.Message) {
	s.ensureResourcesDeps().DeliverFileResponse(requestID, msg)
}

// --- Forwarding for extracted handler methods ---

func (s *apiServer) handleMetricsOverview(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleMetricsOverview(w, r)
}
func (s *apiServer) handleAssetMetrics(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleAssetMetrics(w, r)
}
func (s *apiServer) handleDependencies(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleDependencies(w, r)
}
func (s *apiServer) handleDependencyActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleDependencyActions(w, r)
}
func (s *apiServer) handleAssetDependencies(w http.ResponseWriter, r *http.Request, assetID string, subParts []string) {
	s.ensureResourcesDeps().HandleAssetDependencies(w, r, assetID, subParts)
}
func (s *apiServer) handleAssetBlastRadius(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleAssetBlastRadius(w, r, assetID)
}
func (s *apiServer) handleAssetUpstream(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleAssetUpstream(w, r, assetID)
}
func (s *apiServer) handleWakeOnLAN(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureResourcesDeps().HandleWakeOnLAN(w, r, assetID)
}
func (s *apiServer) processAgentWoLResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureResourcesDeps().ProcessAgentWoLResult(conn, msg)
}
func (s *apiServer) handleGroups(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleGroups(w, r)
}

// handleGroupActions routes group sub-paths. Simple CRUD and move/reorder
// delegate to the resources package; timeline, reliability, and maintenance
// windows stay in cmd/labtether because they need heavier deps.
func (s *apiServer) handleGroupActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/groups/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group path not found")
		return
	}

	parts := strings.Split(path, "/")
	groupID := strings.TrimSpace(parts[0])
	if groupID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group path not found")
		return
	}
	// Intercept "reliability" at root level before it reaches the resources handler.
	if groupID == "reliability" {
		if len(parts) != 1 {
			servicehttp.WriteError(w, http.StatusNotFound, "invalid group reliability path")
			return
		}
		s.handleGroupReliabilityCollection(w, r)
		return
	}
	// Sub-paths that need the heavier local deps.
	if len(parts) == 2 {
		switch parts[1] {
		case "timeline":
			if r.Method != http.MethodGet {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.handleGroupTimeline(w, r, groupID)
			return
		case "reliability":
			if r.Method != http.MethodGet {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.handleGroupReliabilityByID(w, r, groupID)
			return
		case "maintenance-windows":
			s.handleGroupMaintenanceWindowsCollection(w, r, groupID)
			return
		}
	}
	if len(parts) == 3 && parts[1] == "maintenance-windows" {
		windowID := strings.TrimSpace(parts[2])
		if windowID == "" {
			servicehttp.WriteError(w, http.StatusNotFound, "maintenance window path not found")
			return
		}
		s.handleGroupMaintenanceWindowActions(w, r, groupID, windowID)
		return
	}
	// Everything else (CRUD, move, reorder) goes to resources.
	s.ensureResourcesDeps().HandleGroupActions(w, r)
}
func (s *apiServer) handleGroupProfiles(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleGroupProfiles(w, r)
}
func (s *apiServer) handleGroupProfileActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleGroupProfileActions(w, r)
}
func (s *apiServer) handleFailoverPairs(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFailoverPairs(w, r)
}
func (s *apiServer) handleFailoverPairActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFailoverPairActions(w, r)
}
func (s *apiServer) handleSyntheticChecks(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleSyntheticChecks(w, r)
}
func (s *apiServer) handleSyntheticCheckActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleSyntheticCheckActions(w, r)
}
func (s *apiServer) handleLinkSuggestions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleLinkSuggestions(w, r)
}
func (s *apiServer) handleLinkSuggestionActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleLinkSuggestionActions(w, r)
}
func (s *apiServer) handleManualLink(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleManualLink(w, r)
}

// Edge handlers (graph model)
func (s *apiServer) handleEdges(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleEdges(w, r)
}
func (s *apiServer) handleEdgeByID(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleEdgeByID(w, r)
}

// Composite handlers
func (s *apiServer) handleComposites(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleComposites(w, r)
}
func (s *apiServer) handleCompositeActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleCompositeActions(w, r)
}

// Discovery run handler
func (s *apiServer) handleDiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.detectLinkSuggestions(); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "discovery run failed")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "completed"})
}

// Discovery proposal handlers
func (s *apiServer) handleProposals(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleProposals(w, r)
}
func (s *apiServer) handleProposalActions(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleProposalActions(w, r)
}

func (s *apiServer) handleAssetBulkMove(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleAssetBulkMove(w, r)
}
func (s *apiServer) handleDeviceRegister(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleDeviceRegister(w, r)
}
func (s *apiServer) handleRestartSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleRestartSettings(w, r)
}
func (s *apiServer) handleRetentionSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleRetentionSettings(w, r)
}
func (s *apiServer) handleAssetTerminalConfig(w http.ResponseWriter, r *http.Request, assetEntry assets.Asset) {
	s.ensureResourcesDeps().HandleAssetTerminalConfig(w, r, assetEntry)
}
func (s *apiServer) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleAuditEvents(w, r)
}

// hubRestartSelf and restartSettingsRoute aliases for test compatibility.
var hubRestartSelf = respkg.HubRestartSelf

const restartSettingsRoute = respkg.RestartSettingsRoute

type restartSettingsResponse = respkg.RestartSettingsResponse

// sendWakeOnLAN is an alias for test compatibility. Tests that override this
// must also set respkg.SendWakeOnLAN to propagate to the extracted handler.
var sendWakeOnLAN = respkg.SendWakeOnLAN

// Function aliases for test accessibility.
func findAssetMAC(metadata map[string]string) string { return respkg.FindAssetMAC(metadata) }

type wolRelayCandidate = respkg.WoLRelayCandidate

func wolRelayPlatformPriority(platform string) int { return respkg.WoLRelayPlatformPriority(platform) }

func (s *apiServer) eligibleWoLRelays(targetAssetID string, target assets.Asset) []wolRelayCandidate {
	return s.ensureResourcesDeps().EligibleWoLRelays(targetAssetID, target)
}

func (s *apiServer) findAssetMAC(metadata map[string]string) string {
	return respkg.FindAssetMAC(metadata)
}

type dependencyBatchLister = respkg.DependencyBatchLister
type dependencySingleLister = respkg.DependencySingleLister

func parseDependencyAssetIDs(csv string, singular []string) []string {
	return respkg.ParseDependencyAssetIDs(csv, singular)
}
func listAssetDependenciesBatch(store dependencySingleLister, assetIDs []string, limit int) ([]dependencies.Dependency, error) {
	return respkg.ListAssetDependenciesBatch(store, assetIDs, limit)
}
func validateDependencyRequest(req dependencies.CreateDependencyRequest) error {
	return respkg.ValidateDependencyRequest(req)
}

// File connection CRUD handlers
func (s *apiServer) handleFileConnections(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFileConnections(w, r)
}

// File transfer handlers
func (s *apiServer) handleFileTransfers(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleFileTransfers(w, r)
}

// Manual device handler.
func (s *apiServer) handleManualDeviceRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleManualDeviceRoutes(w, r)
}
