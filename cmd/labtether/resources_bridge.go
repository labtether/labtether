package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// buildResourcesDeps constructs the resources.Deps from the apiServer's fields.
func (s *apiServer) buildResourcesDeps() *respkg.Deps {
	enrollmentTransactions, _ := s.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	fileTransferStore := s.fileTransferStore
	if fileTransferStore == nil {
		fileTransferStore = s.db
	}
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
		FileTransferStore:   fileTransferStore,
		ActiveTransfers:     &s.activeTransfers,
		SpoolDir:            fileSpoolDir(s.dataDir),
		RemoteBookmarkStore: s.db,

		FileBridges:    &s.fileBridges,
		ProcessReplies: &s.processReplies,
		ServiceReplies: &s.serviceReplies,
		JournalReplies: &s.journalReplies,
		DiskReplies:    &s.diskReplies,
		NetworkReplies: &s.networkReplies,
		WoLPending:     &s.wolPending,
		PackageReplies: &s.packageReplies,
		CronReplies:    &s.cronReplies,
		UsersReplies:   &s.usersReplies,

		WrapAuth: s.withAuth,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.ensureAgentsDeps().ExecuteViaAgent(job)
		},

		EnforceRateLimit:        s.enforceRateLimit,
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
		PrincipalActorID:        func(ctx context.Context) string { return apiv2.PrincipalActorID(ctx) },
		UserIDFromContext:       func(ctx context.Context) string { return apiv2.UserIDFromContext(ctx) },
		SecretsManager:          s.secretsManager,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},
		OnRelayPushDeviceRegistered: s.ensureDefaultRelayPushRoute,

		ManualDeviceDB: func() respkg.ManualDeviceExecer {
			if s.db != nil {
				return s.db.Pool()
			}
			return nil
		}(),

		// Heartbeat and delete cascade dependencies.
		EnrollmentStore:        s.enrollmentStore,
		EnrollmentTransactions: enrollmentTransactions,
		HubCollectorStore:      s.hubCollectorStore,
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
		RemoveSavedTerminalSessions: s.removeSavedTerminalSessionsForRemovedDevice,
		SendSSHKeyRemoveToAsset: func(assetID string) {
			if s.currentHubSSHIdentity() == nil || s.agentMgr == nil {
				return
			}
			if conn, ok := s.agentMgr.Get(assetID); ok {
				_ = s.sendSSHKeyRemove(conn)
			}
		},
		PersistCanonicalHeartbeatFn: func(assetEntry assets.Asset, req assets.HeartbeatRequest) {
			s.persistCanonicalHeartbeat(assetEntry, req)
		},
		CollectorConfigString: collectorspkg.CollectorConfigString,
		NormalizeAssetKey:     collectorspkg.NormalizeAssetKey,
		AutoDockerCollectorAssetID: func(agentAssetID string) string {
			return autoDockerCollectorAssetID(agentAssetID)
		},

		// Asset sub-handler callbacks for routes that have not yet been
		// extracted from cmd/labtether into the resources package.
		HandleDesktopCredentials:         s.handleDesktopCredentials,
		HandleRetrieveDesktopCredentials: s.handleRetrieveDesktopCredentials,
		HandleDisplayList: func(w http.ResponseWriter, r *http.Request, assetID string) {
			s.ensureDesktopDeps().HandleDisplayListDirect(w, r, assetID)
		},
		HandlePushHubKey:             s.handlePushHubKey,
		HandleTestProtocolConnection: s.handleTestProtocolConnection,
		HandleListProtocolConfigs:    s.handleListProtocolConfigs,
		HandleCreateProtocolConfig:   s.handleCreateProtocolConfig,
		HandleUpdateProtocolConfig:   s.handleUpdateProtocolConfig,
		HandleDeleteProtocolConfig:   s.handleDeleteProtocolConfig,
		HandleHubSSHKey:              s.handleAssetHubSSHKey,
		HubSSHKeyEnabled:             s.hubSSHKeyEnabled,
	}
}

// fileSpoolDir keeps transfer staging on the data volume. An unset data
// directory, as in tests, keeps the system temp directory.
func fileSpoolDir(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, "spool")
}

// ensureResourcesDeps returns a resources.Deps built from the current apiServer
// fields. It rebuilds on every call so that callers see the current state of
// mutable fields (e.g. groupStore set to nil in tests or after reconfiguration).
// buildResourcesDeps is cheap — it copies interface values and function pointers.
func (s *apiServer) ensureResourcesDeps() *respkg.Deps {
	return s.buildResourcesDeps()
}

// The two handlers below stay in cmd/labtether because each runs cmd code
// of its own. Every other caller calls the resources package through
// ensureResourcesDeps(): route entries inside a closure, so the Deps are
// still built for each request and never once when the route table is built.

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

// handleDiscoveryRun runs cmd's link detection (detectLinkSuggestions).
func (s *apiServer) handleDiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobalAPI(w, r, "discovery") {
		return
	}
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
