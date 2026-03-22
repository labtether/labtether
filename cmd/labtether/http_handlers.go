package main

import (
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/telemetry/promexport"
)

func (s *apiServer) buildHTTPHandlers(
	workerState *workerRuntimeState,
	retentionTracker *retentionState,
	counters *workerCounters,
) map[string]http.HandlerFunc {
	if counters == nil {
		counters = &workerCounters{}
	}

	handlers := map[string]http.HandlerFunc{
		"/":                              s.handleHubRoot,
		"/status/aggregate":              s.withAuth(s.handleStatusAggregate(retentionTracker, &counters.processed)),
		"/status/aggregate/live":         s.withAuth(s.handleStatusAggregateLive()),
		"/terminal/preferences":          s.withAuth(s.handleTerminalPreferences),
		"/terminal/snippets":             s.withAuth(s.handleTerminalSnippets),
		"/terminal/snippets/":            s.withAuth(s.handleTerminalSnippetActions),
		"/terminal/workspace/tabs":       s.withAuth(s.handleWorkspaceTabs),
		"/terminal/workspace/tabs/":      s.withAuth(s.handleWorkspaceTabActions),
		"/terminal/persistent-sessions":  s.withAuth(s.handlePersistentSessions),
		"/terminal/persistent-sessions/": s.withAuth(s.handlePersistentSessionActions),
		"/terminal/sessions":             s.withAuth(s.handleSessions),
		"/terminal/sessions/":            s.withAuth(s.handleSessionActions),
		"/terminal/bookmarks":            s.withAuth(s.handleBookmarks),
		"/terminal/bookmarks/":           s.withAuth(s.handleBookmarkActions),
		"/terminal/commands/recent":      s.withAuth(s.handleRecentCommands),
		"/group-profiles":                s.withAuth(s.handleGroupProfiles),
		"/group-profiles/":               s.withAuth(s.handleGroupProfileActions),
		"/group-failover-pairs":          s.withAuth(s.handleFailoverPairs),
		"/group-failover-pairs/":         s.withAuth(s.handleFailoverPairActions),
		"/groups":                        s.withAuth(s.handleGroups),
		"/groups/":                       s.withAuth(s.handleGroupActions),
		"/assets":                        s.withAuth(s.handleAssets),
		"/assets/manual":                 s.withAuth(s.handleManualDeviceRoutes),
		"/assets/":                       s.withAuth(s.handleAssetActions),
		"/credentials/profiles":          s.withAuth(s.handleCredentialProfiles),
		"/credentials/profiles/":         s.withAuth(s.handleCredentialProfileActions),
		// /metrics is an exact-match path (no trailing slash) so it does not
		// conflict with /metrics/overview or /metrics/assets/ above.
		// The endpoint is intentionally unauthenticated for Prometheus scraping;
		// a settings toggle (Task 10) will gate it when that feature ships.
		"/metrics":                                   s.handlePrometheusMetrics(),
		"/metrics/overview":                          s.withAuth(s.handleMetricsOverview),
		"/metrics/assets/":                           s.withAuth(s.handleAssetMetrics),
		"/logs/query":                                s.withAuth(s.handleLogsQuery),
		"/logs/journal/":                             s.withAuth(s.handleJournalLogs),
		"/logs/sources":                              s.withAuth(s.handleLogSources),
		"/logs/views":                                s.withAuth(s.handleLogViews),
		"/logs/views/":                               s.withAuth(s.handleLogViewActions),
		"/telemetry/frontend/perf":                   s.withAuth(s.handleFrontendPerfTelemetry),
		"/telemetry/mobile/client":                   s.withAuth(s.handleMobileClientTelemetry),
		"/queue/dead-letters":                        s.withAuth(s.handleDeadLetters),
		"/connectors":                                s.withAuth(s.handleListConnectors),
		"/connectors/":                               s.withAuth(s.handleConnectorActions),
		"/proxmox/assets/":                           s.withAuth(s.handleProxmoxAssets),
		"/proxmox/tasks/":                            s.withAuth(s.handleProxmoxTaskRoutes),
		"/proxmox/cluster/status":                    s.withAuth(s.handleProxmoxClusterStatus),
		"/proxmox/cluster/resources":                 s.withAuth(s.handleProxmoxClusterResources),
		"/proxmox/nodes/":                            s.withAuth(s.handleProxmoxNodeRoutes),
		"/proxmox/ceph/":                             s.withAuth(s.ensureProxmoxDeps().HandleProxmoxCeph),
		"/proxmox/ceph/status":                       s.withAuth(s.handleProxmoxCephStatus),
		"/pbs/assets/":                               s.withAuth(s.handlePBSAssets),
		"/pbs/tasks/":                                s.withAuth(s.handlePBSTaskRoutes),
		"/truenas/assets/":                           s.withAuth(s.handleTrueNASAssets),
		"/portainer/assets/":                         s.withAuth(s.handlePortainerAssets),
		"/api/v1/services/web":                       s.withAuth(s.handleWebServices),
		"/api/v1/services/web/compat":                s.withAuth(s.handleWebServiceCompat),
		"/api/v1/services/web/categories":            s.withAuth(s.handleWebServiceCategories),
		"/api/v1/services/web/sync":                  s.withAuth(s.handleWebServiceSync),
		"/api/v1/services/web/manual":                s.withAuth(s.handleWebServiceManual),
		"/api/v1/services/web/manual/":               s.withAuth(s.handleWebServiceManualActions),
		"/api/v1/services/web/overrides":             s.withAuth(s.handleWebServiceOverrides),
		"/api/v1/services/web/icon-library":          s.withAuth(s.handleWebServiceIconLibrary),
		"/api/v1/services/web/alt-urls/":             s.withAuth(s.handleWebServiceAltURLs),
		"/api/v1/services/web/never-group-rules":     s.withAuth(s.handleWebServiceNeverGroupRules),
		"/api/v1/services/web/grouping-settings":     s.withAuth(s.handleWebServiceGroupingSettings),
		"/api/v1/services/web/grouping-suggestions/": s.withAuth(s.handleWebServiceGroupingSuggestionResponse),
		"/api/v1/docker/hosts":                       s.withAuth(s.handleDockerHosts),
		"/api/v1/docker/hosts/":                      s.withAuth(s.handleDockerHostActions),
		"/api/v1/docker/containers/":                 s.withAuth(s.handleDockerContainerActions),
		"/api/v1/docker/stacks/":                     s.withAuth(s.handleDockerStackActions),
		"/api/v2/docker/hosts":                       s.withAuth(s.handleV2DockerHosts),
		"/api/v2/docker/hosts/":                      s.withAuth(s.handleV2DockerHostActions),
		"/api/v2/docker/containers/":                 s.withAuth(s.handleV2DockerContainerActions),
		"/api/v2/docker/stacks/":                     s.withAuth(s.handleV2DockerStackActions),
		"/api/v2/metrics/overview":                   s.withAuth(s.handleV2MetricsOverview),
		"/api/v2/metrics/query":                      s.withAuth(s.handleV2MetricsQuery),
		// Updates
		"/api/v2/updates/plans":  s.withAuth(s.handleV2UpdatePlans),
		"/api/v2/updates/plans/": s.withAuth(s.handleV2UpdatePlanActions),
		"/api/v2/updates/runs":   s.withAuth(s.handleV2UpdateRuns),
		"/api/v2/updates/runs/":  s.withAuth(s.handleV2UpdateRunActions),
		// Groups
		"/api/v2/groups":  s.withAuth(s.handleV2Groups),
		"/api/v2/groups/": s.withAuth(s.handleV2GroupActions),
		// Alerts & Incidents
		"/api/v2/alerts":       s.withAuth(s.handleV2Alerts),
		"/api/v2/alerts/":      s.withAuth(s.handleV2AlertActions),
		"/api/v2/alerts/rules": s.withAuth(s.handleV2AlertRules),
		"/api/v2/incidents":    s.withAuth(s.handleV2Incidents),
		"/api/v2/incidents/":   s.withAuth(s.handleV2IncidentActions),
		// Connectors (generic)
		"/api/v2/connectors":  s.withAuth(s.handleV2Connectors),
		"/api/v2/connectors/": s.withAuth(s.handleV2ConnectorActions),
		// Proxmox
		"/api/v2/proxmox/cluster/status":    s.withAuth(s.handleV2ProxmoxClusterStatus),
		"/api/v2/proxmox/cluster/resources": s.withAuth(s.handleV2ProxmoxClusterResources),
		"/api/v2/proxmox/assets/":           s.withAuth(s.handleV2ProxmoxAssets),
		"/api/v2/proxmox/nodes/":            s.withAuth(s.handleV2ProxmoxNodeRoutes),
		"/api/v2/proxmox/ceph/status":       s.withAuth(s.handleV2ProxmoxCephStatus),
		"/api/v2/proxmox/tasks/":            s.withAuth(s.handleV2ProxmoxTasks),
		// TrueNAS
		"/api/v2/truenas/assets/": s.withAuth(s.handleV2TrueNASAssets),
		// PBS
		"/api/v2/pbs/assets/": s.withAuth(s.handleV2PBSAssets),
		"/api/v2/pbs/tasks/":  s.withAuth(s.handleV2PBSTasks),
		// Portainer
		"/api/v2/portainer/assets/": s.withAuth(s.handleV2PortainerAssets),
		// Home Assistant
		"/api/v2/homeassistant/entities":    s.withAuth(s.handleV2HAEntities),
		"/api/v2/homeassistant/entities/":   s.withAuth(s.handleV2HAEntityActions),
		"/api/v2/homeassistant/automations": s.withAuth(s.handleV2HAAutomations),
		"/api/v2/homeassistant/scenes":      s.withAuth(s.handleV2HAScenes),
		// Credentials
		"/api/v2/credentials/profiles":  s.withAuth(s.handleV2Credentials),
		"/api/v2/credentials/profiles/": s.withAuth(s.handleV2CredentialActions),
		// Terminal
		"/api/v2/terminal/sessions": s.withAuth(s.handleV2TerminalSessions),
		"/api/v2/terminal/history":  s.withAuth(s.handleV2TerminalHistory),
		"/api/v2/terminal/snippets": s.withAuth(s.handleV2TerminalSnippets),
		// Agents
		"/api/v2/agents":                 s.withAuth(s.handleV2Agents),
		"/api/v2/agents/":                s.withAuth(s.handleV2AgentActions),
		"/api/v2/agents/pending":         s.withAuth(s.handleV2AgentsPending),
		"/api/v2/agents/pending/approve": s.withAuth(s.handleV2AgentsPendingApprove),
		"/api/v2/agents/pending/reject":  s.withAuth(s.handleV2AgentsPendingReject),
		// Hub
		"/api/v2/hub/status":    s.withAuth(s.handleV2HubStatus),
		"/api/v2/hub/agents":    s.withAuth(s.handleV2HubAgents),
		"/api/v2/hub/tls":       s.withAuth(s.handleV2HubTLS),
		"/api/v2/hub/tls/renew": s.withAuth(s.handleV2HubTLSRenew),
		"/api/v2/hub/tailscale": s.withAuth(s.handleV2HubTailscale),
		// Web Services
		"/api/v2/web-services":      s.withAuth(s.handleV2WebServices),
		"/api/v2/web-services/sync": s.withAuth(s.handleV2WebServiceSync),
		// Collectors
		"/api/v2/collectors":  s.withAuth(s.handleV2Collectors),
		"/api/v2/collectors/": s.withAuth(s.handleV2CollectorActions),
		// Notifications
		"/api/v2/notifications/channels": s.withAuth(s.handleV2NotificationChannels),
		"/api/v2/notifications/history":  s.withAuth(s.handleV2NotificationHistory),
		// Synthetic Checks
		"/api/v2/synthetic-checks":  s.withAuth(s.handleV2SyntheticChecks),
		"/api/v2/synthetic-checks/": s.withAuth(s.handleV2SyntheticCheckActions),
		// Discovery
		"/api/v2/discovery/run":        s.withAuth(s.handleV2DiscoveryRun),
		"/api/v2/discovery/proposals":  s.withAuth(s.handleV2DiscoveryProposals),
		"/api/v2/discovery/proposals/": s.withAuth(s.handleV2DiscoveryProposalActions),
		// Topology
		"/api/v2/dependencies":  s.withAuth(s.handleV2Dependencies),
		"/api/v2/dependencies/": s.withAuth(s.handleV2DependencyActions),
		"/api/v2/edges":         s.withAuth(s.handleV2Edges),
		"/api/v2/composites":    s.withAuth(s.handleV2Composites),
		"/api/v2/composites/":   s.withAuth(s.handleV2CompositeActions),
		// Topology Canvas
		"/api/v2/topology":              s.withAuth(s.handleV2Topology),
		"/api/v2/topology/zones":        s.withAuth(s.handleV2TopologyZones),
		"/api/v2/topology/zones/":       s.withAuth(s.handleV2TopologyZoneActions),
		"/api/v2/topology/connections":  s.withAuth(s.handleV2TopologyConnections),
		"/api/v2/topology/connections/": s.withAuth(s.handleV2TopologyConnection),
		"/api/v2/topology/viewport":     s.withAuth(s.handleV2TopologyViewport),
		"/api/v2/topology/unsorted":     s.withAuth(s.handleV2TopologyUnsorted),
		"/api/v2/topology/auto-place":   s.withAuth(s.handleV2TopologyAutoPlace),
		"/api/v2/topology/reset":        s.withAuth(s.handleV2TopologyReset),
		"/api/v2/topology/dismiss":      s.withAuth(s.handleV2TopologyDismiss),
		"/api/v2/topology/dismiss/":     s.withAuth(s.handleV2TopologyUndismiss),
		// Failover
		"/api/v2/failover-pairs":  s.withAuth(s.handleV2FailoverPairs),
		"/api/v2/failover-pairs/": s.withAuth(s.handleV2FailoverPairActions),
		// Dead Letters
		"/api/v2/dead-letters":  s.withAuth(s.handleV2DeadLetters),
		"/api/v2/dead-letters/": s.withAuth(s.handleV2DeadLetters),
		// Audit
		"/api/v2/audit/events": s.withAuth(s.handleV2AuditEvents),
		// Log Views
		"/api/v2/logs/views":  s.withAuth(s.handleV2LogViews),
		"/api/v2/logs/views/": s.withAuth(s.handleV2LogViewActions),
		// Prometheus Settings
		"/api/v2/settings/prometheus":      s.withAuth(s.handleV2PrometheusSettings),
		"/api/v2/settings/prometheus/test": s.withAuth(s.handleV2PrometheusTest),
		// Events
		"/api/v2/events/stream":     s.withAuth(s.handleV2EventStream),
		"/api/v1/file-connections":  s.withAuth(s.handleFileConnections),
		"/api/v1/file-connections/": s.withAuth(s.handleFileConnections),
		"/api/v1/remote-bookmarks":  s.withAuth(s.handleRemoteBookmarks),
		"/api/v1/remote-bookmarks/": s.withAuth(s.handleRemoteBookmarks),
		"/api/v1/file-transfers":    s.withAuth(s.handleFileTransfers),
		"/api/v1/file-transfers/":   s.withAuth(s.handleFileTransfers),
		// File Transfers (v2)
		"/api/v2/file-transfers":  s.withAuth(s.handleV2FileTransfers),
		"/api/v2/file-transfers/": s.withAuth(s.handleV2FileTransferActions),
		// OpenAPI spec (unauthenticated — public documentation endpoint)
		"/api/v2/openapi.json":                 s.handleV2OpenAPI,
		"/actions/execute":                     s.withAuth(s.handleActionExecute),
		"/actions/runs":                        s.withAuth(s.handleActionRuns),
		"/actions/runs/":                       s.withAuth(s.handleActionRunActions),
		"/updates/plans":                       s.withAuth(s.handleUpdatePlans),
		"/updates/plans/":                      s.withAuth(s.handleUpdatePlanActions),
		"/updates/runs":                        s.withAuth(s.handleUpdateRuns),
		"/updates/runs/":                       s.withAuth(s.handleUpdateRunActions),
		"/auth/login":                          s.handleAuthLogin,
		"/auth/login/2fa":                      s.handleLogin2FA,
		"/auth/bootstrap/status":               s.handleAuthBootstrapStatus,
		"/auth/bootstrap":                      s.handleAuthBootstrapSetup,
		"/auth/logout":                         s.handleAuthLogout,
		"/auth/me":                             s.withAuth(s.handleAuthMe),
		"/auth/2fa/setup":                      s.withAuth(s.handle2FASetup),
		"/auth/2fa/verify":                     s.withAuth(s.handle2FAVerify),
		"/auth/2fa":                            s.withAuth(s.handle2FADisable),
		"/auth/2fa/recovery-codes":             s.withAuth(s.handle2FARecoveryCodes),
		"/auth/providers":                      s.handleAuthProviders,
		"/auth/oidc/start":                     s.handleAuthOIDCStart,
		"/auth/oidc/callback":                  s.handleAuthOIDCCallback,
		"/auth/users":                          s.withAdminAuth(s.handleAuthUsers),
		"/auth/users/":                         s.withAdminAuth(s.handleAuthUserActions),
		"/alerts/rules":                        s.withAuth(s.handleAlertRules),
		"/alerts/rules/":                       s.withAuth(s.handleAlertRuleActions),
		"/alerts/templates":                    s.withAuth(s.handleAlertTemplates),
		"/alerts/templates/":                   s.withAuth(s.handleAlertTemplateActions),
		"/alerts/instances":                    s.withAuth(s.handleAlertInstances),
		"/alerts/instances/":                   s.withAuth(s.handleAlertInstanceActions),
		"/alerts/silences":                     s.withAuth(s.handleAlertSilences),
		"/alerts/silences/":                    s.withAuth(s.handleAlertSilenceActions),
		"/alerts/routes":                       s.withAuth(s.handleAlertRoutes),
		"/alerts/routes/":                      s.withAuth(s.handleAlertRouteActions),
		"/notifications/channels":              s.withAuth(s.handleNotificationChannels),
		"/notifications/channels/":             s.withAuth(s.handleNotificationChannelActions),
		"/notifications/history":               s.withAuth(s.handleNotificationHistory),
		"/incidents":                           s.withAuth(s.handleIncidents),
		"/incidents/":                          s.withAuth(s.handleIncidentActions),
		"/settings/retention":                  s.withAuth(s.handleRetentionSettings),
		"/settings/oidc":                       s.withAdminAuth(s.handleOIDCSettings),
		"/settings/oidc/apply":                 s.withAdminAuth(s.handleOIDCSettingsApply),
		"/settings/managed-database":           s.withAdminAuth(s.handleManagedDatabaseSettings),
		"/settings/managed-database/reveal":    s.withAdminAuth(s.handleManagedDatabasePasswordReveal),
		"/settings/runtime":                    s.withAdminAuth(s.handleRuntimeSettings),
		"/settings/runtime/reset":              s.withAdminAuth(s.handleRuntimeSettingsReset),
		"/settings/restart":                    s.withAdminAuth(s.handleRestartSettings),
		"/settings/tls":                        s.withAdminAuth(s.handleTLSSettings),
		"/settings/prometheus/test-connection": s.withAdminAuth(s.handlePrometheusTestConnection),
		"/settings/tailscale/serve":            s.withAuth(s.handleTailscaleServeStatus),
		"/audit/events":                        s.withAuth(s.handleAuditEvents),
		"/policy/check":                        s.withAuth(s.handlePolicyCheck),
		"/api/v1/enroll":                       s.handleEnroll,
		"/api/v1/discover":                     s.handleDiscover,
		"/api/v1/ca.crt":                       s.handleCACert,
		"/api/v1/tls/info":                     s.handleTLSInfo,
		"/api/v1/agent/binary":                 s.handleAgentBinary,
		"/api/v1/agent/releases/latest":        s.handleAgentReleaseLatest,
		"/api/v1/agent/install.sh":             s.handleAgentInstallScript,
		"/api/v1/agent/bootstrap.sh":           s.handleAgentBootstrapScript,
		"/install.sh":                          s.handleAgentInstallScript,
		"/settings/enrollment":                 s.withAdminAuth(s.handleEnrollmentTokens),
		"/settings/enrollment/":                s.withAdminAuth(s.handleEnrollmentTokenActions),
		"/settings/agent-tokens":               s.withAdminAuth(s.handleAgentTokens),
		"/settings/agent-tokens/":              s.withAdminAuth(s.handleAgentTokenActions),
		"/settings/tokens/cleanup":             s.withAdminAuth(s.handleTokenCleanup),
		"/hub/ssh-public-key":                  s.withAuth(s.handleHubSSHPublicKey),
		"/settings/ssh-hub-key/rotate":         s.withAdminAuth(s.handleSSHHubKeyRotate),
		"/desktop/sessions":                    s.withAuth(s.handleDesktopSessions),
		"/desktop/sessions/":                   s.withAuth(s.handleDesktopSessionActions),
		"/desktop/diagnose/":                   s.withAuth(s.handleDesktopDiagnoseRequest),
		"/recordings":                          s.withAuth(s.handleRecordings),
		"/recordings/":                         s.withAuth(s.handleRecordingActions),
		"/files/":                              s.withAuth(s.handleFiles),
		"/processes/":                          s.withAuth(s.handleProcesses),
		"/disks/":                              s.withAuth(s.handleDisks),
		"/network/":                            s.withAuth(s.handleNetworks),
		"/packages/":                           s.withAuth(s.handlePackages),
		"/cron/":                               s.withAuth(s.handleCrons),
		"/users/":                              s.withAuth(s.handleUsers),
		"/services/":                           s.withAuth(s.handleServices),
		"/api/v1/nodes/":                       s.withAuth(s.handleNodeSubRoutes),
		"/ws/agent":                            s.handleAgentWebSocket,
		"/ws/events":                           s.handleBrowserEvents,
		"/ws/events/ticket":                    s.withAuth(s.handleEventTicket),
		"/agents/connected":                    s.withAuth(s.handleConnectedAgents),
		"/agents/presence":                     s.withAuth(s.handleAgentPresence),
		"/api/v1/agents/":                      s.withAuth(s.handleAgentSettingsRoutes),
		"/api/v1/agents/pending":               s.withAuth(s.handleListPendingAgents),
		"/api/v1/agents/approve":               s.withAuth(s.handleApproveAgent),
		"/api/v1/agents/reject":                s.withAuth(s.handleRejectAgent),
		"/dependencies":                        s.withAuth(s.handleDependencies),
		"/dependencies/":                       s.withAuth(s.handleDependencyActions),
		"/links/suggestions":                   s.withAuth(s.handleLinkSuggestions),
		"/links/suggestions/":                  s.withAuth(s.handleLinkSuggestionActions),
		"/links/manual":                        s.withAuth(s.handleManualLink),
		"/edges":                               s.withAuth(s.handleEdges),
		"/edges/":                              s.withAuth(s.handleEdgeByID),
		"/composites":                          s.withAuth(s.handleComposites),
		"/composites/":                         s.withAuth(s.handleCompositeActions),
		"/discovery/run":                       s.withAuth(s.handleDiscoveryRun),
		"/discovery/proposals":                 s.withAuth(s.handleProposals),
		"/discovery/proposals/":                s.withAuth(s.handleProposalActions),
		"/assets/bulk/move":                    s.withAuth(s.handleAssetBulkMove),
		"/synthetic-checks":                    s.withAuth(s.handleSyntheticChecks),
		"/synthetic-checks/":                   s.withAuth(s.handleSyntheticCheckActions),
		"/hub-collectors":                      s.withAuth(s.handleHubCollectors),
		"/hub-collectors/":                     s.withAuth(s.handleHubCollectorActions),
		"/api/v1/devices/register":             s.withAuth(s.handleDeviceRegister),
		"/admin/reset":                         s.withAdminAuth(s.handleAdminReset),
		"/api/v2/keys":                         s.withAdminAuth(s.handleAPIKeys),
		"/api/v2/keys/":                        s.withAdminAuth(s.handleAPIKeyActions),
		"/api/v2/schedules":                    s.withAuth(s.handleV2Schedules),
		"/api/v2/schedules/":                   s.withAuth(s.handleV2ScheduleActions),
		"/api/v2/webhooks":                     s.withAuth(s.handleV2Webhooks),
		"/api/v2/webhooks/":                    s.withAuth(s.handleV2WebhookActions),
		"/api/v2/whoami":                       s.withAuth(s.handleV2Whoami),
		"/api/v2/assets":                       s.withAuth(s.handleV2Assets),
		"/api/v2/assets/":                      s.withAuth(s.handleV2AssetActions),
		"/api/v2/exec":                         s.withAuth(s.handleV2ExecMulti),
		"/api/v2/actions":                      s.withAuth(s.handleV2SavedActions),
		"/api/v2/actions/":                     s.withAuth(s.handleV2SavedActionActions),
		"/api/v2/search":                       s.withAuth(s.handleV2Search),
		"/api/v2/bulk/service-action":          s.withAuth(s.handleV2BulkServiceAction),
		"/api/v2/bulk/file-push":               s.withAuth(s.handleV2BulkFilePush),
		"/worker/stats":                        s.withAuth(s.workerStatsHandler(workerState, retentionTracker, &counters.processed, &counters.processedActions, &counters.processedUpdates)),
		// MCP (Model Context Protocol) endpoint for AI agent tool discovery
		"/mcp": s.withAuth(s.handleMCP()),
	}

	if envOrDefaultBool("DEV_MODE", false) {
		log.Printf("labtether: DEV_MODE enabled — pprof endpoints registered at /debug/pprof/")
		handlers["/debug/pprof/"] = s.withAuth(pprof.Index)
		handlers["/debug/pprof/cmdline"] = s.withAuth(pprof.Cmdline)
		handlers["/debug/pprof/profile"] = s.withAuth(pprof.Profile)
		handlers["/debug/pprof/symbol"] = s.withAuth(pprof.Symbol)
		handlers["/debug/pprof/trace"] = s.withAuth(pprof.Trace)
	}

	return handlers
}

// handlePrometheusMetrics returns an http.HandlerFunc that serves the
// Prometheus metrics exposition format at the exact path /metrics.
//
// The endpoint is unauthenticated by design so that external Prometheus
// servers can scrape it without credentials. It is gated by the
// prometheus.scrape_enabled runtime setting (default: false). When disabled
// it returns 404 with a plain-text explanation.
func (s *apiServer) handlePrometheusMetrics() http.HandlerFunc {
	src := newPrometheusSnapshotSource(s)
	h := promexport.NewHandler(src)
	return func(w http.ResponseWriter, r *http.Request) {
		if !prometheusSettingsEnabled(s.runtimeStore, runtimesettings.KeyPrometheusScrapeEnabled) {
			http.Error(w, "prometheus scrape endpoint is disabled", http.StatusNotFound)
			return
		}
		h.ServeHTTP(w, r)
	}
}
