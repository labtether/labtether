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
		"/status/aggregate":              s.withAuth(requireScope("hub:read", s.handleStatusAggregate(retentionTracker, &counters.processed))),
		"/status/aggregate/live":         s.withAuth(requireScope("hub:read", s.handleStatusAggregateLive())),
		"/terminal/preferences":          s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleTerminalPreferences)),
		"/terminal/snippets":             s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleTerminalSnippets)),
		"/terminal/snippets/":            s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleTerminalSnippetActions)),
		"/terminal/workspace/tabs":       s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleWorkspaceTabs)),
		"/terminal/workspace/tabs/":      s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleWorkspaceTabActions)),
		"/terminal/persistent-sessions":  s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handlePersistentSessions)),
		"/terminal/persistent-sessions/": s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handlePersistentSessionActions)),
		"/terminal/sessions":             s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleSessions)),
		"/terminal/sessions/":            s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleSessionActions)),
		"/terminal/bookmarks":            s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleBookmarks)),
		"/terminal/bookmarks/":           s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleBookmarkActions)),
		"/terminal/commands/recent":      s.withAuth(requireScope("terminal:read", s.handleRecentCommands)),
		"/group-profiles":                s.withAuth(requireReadWriteScopes("groups:read", "groups:write", s.handleGroupProfiles)),
		"/group-profiles/":               s.withAuth(requireReadWriteScopes("groups:read", "groups:write", s.handleGroupProfileActions)),
		"/group-failover-pairs":          s.withAuth(requireReadWriteScopes("failover:read", "failover:write", s.handleFailoverPairs)),
		"/group-failover-pairs/":         s.withAuth(requireReadWriteScopes("failover:read", "failover:write", s.handleFailoverPairActions)),
		"/groups":                        s.withAuth(requireReadWriteScopes("groups:read", "groups:write", s.handleGroups)),
		"/groups/":                       s.withAuth(requireReadWriteScopes("groups:read", "groups:write", s.handleGroupActions)),
		"/assets":                        s.withAuth(requireScope("assets:read", s.handleAssets)),
		"/assets/manual":                 s.withAuth(requireScope("assets:write", s.handleManualDeviceRoutes)),
		"/assets/":                       s.withAuth(s.handleAssetActions),
		"/credentials/profiles":          s.withAuth(requireReadWriteScopes("credentials:read", "credentials:write", s.handleCredentialProfiles)),
		"/credentials/profiles/":         s.withAuth(requireReadWriteScopes("credentials:read", "credentials:write", s.handleCredentialProfileActions)),
		// /metrics is an exact-match path (no trailing slash) so it does not
		// conflict with /metrics/overview or /metrics/assets/ above.
		// The endpoint is intentionally unauthenticated for Prometheus scraping;
		// a settings toggle (Task 10) will gate it when that feature ships.
		"/metrics":                                   s.handlePrometheusMetrics(),
		"/metrics/overview":                          s.withAuth(requireScope("metrics:read", s.handleMetricsOverview)),
		"/metrics/assets/":                           s.withAuth(requireScope("metrics:read", requireAssetFromPath("/metrics/assets/", s.handleAssetMetrics))),
		"/logs/query":                                s.withAuth(requireScope("logs:read", s.handleLogsQuery)),
		"/logs/journal/":                             s.withAuth(requireScope("logs:read", requireAssetFromPath("/logs/journal/", s.handleJournalLogs))),
		"/logs/sources":                              s.withAuth(requireScope("logs:read", s.handleLogSources)),
		"/logs/views":                                s.withAuth(requireReadWriteScopes("logs:read", "logs:write", s.handleLogViews)),
		"/logs/views/":                               s.withAuth(requireReadWriteScopes("logs:read", "logs:write", s.handleLogViewActions)),
		"/telemetry/frontend/perf":                   s.withAuth(s.handleFrontendPerfTelemetry),
		"/telemetry/mobile/client":                   s.withAuth(s.handleMobileClientTelemetry),
		"/queue/dead-letters":                        s.withAuth(requireReadWriteScopes("dead-letters:read", "dead-letters:write", s.handleDeadLetters)),
		"/connectors":                                s.withAuth(requireScope("connectors:read", s.handleListConnectors)),
		"/connectors/":                               s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleConnectorActions)),
		"/proxmox/assets/":                           s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxAssets)),
		"/proxmox/tasks/":                            s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxTaskRoutes)),
		"/proxmox/cluster/status":                    s.withAuth(requireScope("connectors:read", s.handleProxmoxClusterStatus)),
		"/proxmox/cluster/resources":                 s.withAuth(requireScope("connectors:read", s.handleProxmoxClusterResources)),
		"/proxmox/nodes/":                            s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxNodeRoutes)),
		"/proxmox/ceph/":                             s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.ensureProxmoxDeps().HandleProxmoxCeph)),
		"/proxmox/ceph/status":                       s.withAuth(requireScope("connectors:read", s.handleProxmoxCephStatus)),
		"/pbs/assets/":                               s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handlePBSAssets)),
		"/pbs/tasks/":                                s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handlePBSTaskRoutes)),
		"/truenas/assets/":                           s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleTrueNASAssets)),
		"/portainer/assets/":                         s.withAuth(requireScope("connectors:read", s.handlePortainerAssets)),
		"/api/v1/services/web":                       s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServices)),
		"/api/v1/services/web/compat":                s.withAuth(requireScope("web-services:read", s.handleWebServiceCompat)),
		"/api/v1/services/web/categories":            s.withAuth(requireScope("web-services:read", s.handleWebServiceCategories)),
		"/api/v1/services/web/sync":                  s.withAuth(requireScope("web-services:write", s.handleWebServiceSync)),
		"/api/v1/services/web/manual":                s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceManual)),
		"/api/v1/services/web/manual/":               s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceManualActions)),
		"/api/v1/services/web/overrides":             s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceOverrides)),
		"/api/v1/services/web/icon-library":          s.withAuth(requireScope("web-services:read", s.handleWebServiceIconLibrary)),
		"/api/v1/services/web/alt-urls/":             s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceAltURLs)),
		"/api/v1/services/web/never-group-rules":     s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceNeverGroupRules)),
		"/api/v1/services/web/grouping-settings":     s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceGroupingSettings)),
		"/api/v1/services/web/grouping-suggestions/": s.withAuth(requireScope("web-services:write", s.handleWebServiceGroupingSuggestionResponse)),
		"/api/v1/docker/hosts":                       s.withAuth(requireReadWriteScopes("docker:read", "docker:write", s.handleDockerHosts)),
		"/api/v1/docker/hosts/":                      s.withAuth(requireReadWriteScopes("docker:read", "docker:write", s.handleDockerHostActions)),
		"/api/v1/docker/containers/":                 s.withAuth(requireReadWriteScopes("docker:read", "docker:write", s.handleDockerContainerActions)),
		"/api/v1/docker/stacks/":                     s.withAuth(requireReadWriteScopes("docker:read", "docker:write", s.handleDockerStackActions)),
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
		"/api/v1/file-connections":  s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleFileConnections)),
		"/api/v1/file-connections/": s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleFileConnections)),
		"/api/v1/remote-bookmarks":  s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleRemoteBookmarks)),
		"/api/v1/remote-bookmarks/": s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleRemoteBookmarks)),
		"/api/v1/file-transfers":    s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleFileTransfers)),
		"/api/v1/file-transfers/":   s.withAuth(requireReadWriteScopes("files:read", "files:write", s.handleFileTransfers)),
		// File Transfers (v2)
		"/api/v2/file-transfers":  s.withAuth(s.handleV2FileTransfers),
		"/api/v2/file-transfers/": s.withAuth(s.handleV2FileTransferActions),
		// OpenAPI spec (unauthenticated — public documentation endpoint)
		"/api/v2/openapi.json":                 s.handleV2OpenAPI,
		"/actions/execute":                     s.withAuth(requireScope("actions:exec", s.handleActionExecute)),
		"/actions/runs":                        s.withAuth(requireScope("actions:read", s.handleActionRuns)),
		"/actions/runs/":                       s.withAuth(requireReadWriteScopes("actions:read", "actions:write", s.handleActionRunActions)),
		"/updates/plans":                       s.withAuth(requireReadWriteScopes("updates:read", "updates:write", s.handleUpdatePlans)),
		"/updates/plans/":                      s.withAuth(requireReadWriteScopes("updates:read", "updates:write", s.handleUpdatePlanActions)),
		"/updates/runs":                        s.withAuth(requireScope("updates:read", s.handleUpdateRuns)),
		"/updates/runs/":                       s.withAuth(requireReadWriteScopes("updates:read", "updates:write", s.handleUpdateRunActions)),
		"/api/demo/session":                    s.handleDemoSession,
		"/auth/login":                          s.handleAuthLogin,
		"/auth/login/2fa":                      s.handleLogin2FA,
		"/auth/bootstrap/status":               s.handleAuthBootstrapStatus,
		"/auth/bootstrap":                      s.handleAuthBootstrapSetup,
		"/auth/logout":                         s.handleAuthLogout,
		"/auth/me":                             s.withAuth(s.handleAuthMe),
		"/auth/me/password":                    s.withAuth(s.handleChangePassword),
		"/auth/account":                        s.withAuth(s.handleDeleteOwnAccount),
		"/auth/2fa/setup":                      s.withAuth(s.handle2FASetup),
		"/auth/2fa/verify":                     s.withAuth(s.handle2FAVerify),
		"/auth/2fa":                            s.withAuth(s.handle2FADisable),
		"/auth/2fa/recovery-codes":             s.withAuth(s.handle2FARecoveryCodes),
		"/auth/providers":                      s.handleAuthProviders,
		"/auth/oidc/start":                     s.handleAuthOIDCStart,
		"/auth/oidc/callback":                  s.handleAuthOIDCCallback,
		"/auth/users":                          s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleAuthUsers)),
		"/auth/users/":                         s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleAuthUserActions)),
		"/alerts/rules":                        s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertRules)),
		"/alerts/rules/":                       s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertRuleActions)),
		"/alerts/templates":                    s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertTemplates)),
		"/alerts/templates/":                   s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertTemplateActions)),
		"/alerts/instances":                    s.withAuth(requireScope("alerts:read", s.handleAlertInstances)),
		"/alerts/instances/":                   s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertInstanceActions)),
		"/alerts/silences":                     s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertSilences)),
		"/alerts/silences/":                    s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertSilenceActions)),
		"/alerts/routes":                       s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertRoutes)),
		"/alerts/routes/":                      s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleAlertRouteActions)),
		"/notifications/channels":              s.withAuth(requireReadWriteScopes("notifications:read", "notifications:write", s.handleNotificationChannels)),
		"/notifications/channels/":             s.withAuth(requireReadWriteScopes("notifications:read", "notifications:write", s.handleNotificationChannelActions)),
		"/notifications/history":               s.withAuth(requireScope("notifications:read", s.handleNotificationHistory)),
		"/incidents":                           s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleIncidents)),
		"/incidents/":                          s.withAuth(requireReadWriteScopes("alerts:read", "alerts:write", s.handleIncidentActions)),
		"/settings/retention":                  s.withAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleRetentionSettings)),
		"/settings/oidc":                       s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleOIDCSettings)),
		"/settings/oidc/apply":                 s.withAdminAuth(requireScope("settings:write", s.handleOIDCSettingsApply)),
		"/settings/managed-database":           s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleManagedDatabaseSettings)),
		"/settings/managed-database/reveal":    s.withAdminAuth(requireScope("settings:read", s.handleManagedDatabasePasswordReveal)),
		"/settings/runtime":                    s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleRuntimeSettings)),
		"/settings/runtime/reset":              s.withAdminAuth(requireScope("settings:write", s.handleRuntimeSettingsReset)),
		"/settings/restart":                    s.withAdminAuth(requireScope("settings:write", s.handleRestartSettings)),
		"/settings/tls":                        s.withAdminAuth(requireReadWriteScopes("hub:read", "hub:admin", s.handleTLSSettings)),
		"/settings/prometheus/test-connection": s.withAdminAuth(requireScope("settings:read", s.handlePrometheusTestConnection)),
		"/settings/tailscale/serve":            s.withAuth(requireReadWriteScopes("hub:read", "hub:admin", s.handleTailscaleServeStatus)),
		"/audit/events":                        s.withAuth(requireScope("audit:read", s.handleAuditEvents)),
		"/policy/check":                        s.withAuth(requireScope("settings:read", s.handlePolicyCheck)),
		"/api/v1/enroll":                       s.handleEnroll,
		"/api/v1/discover":                     s.handleDiscover,
		"/api/v1/ca.crt":                       s.handleCACert,
		"/api/v1/tls/info":                     s.handleTLSInfo,
		"/api/v1/agent/binary":                 s.handleAgentBinary,
		"/api/v1/agent/releases/latest":        s.handleAgentReleaseLatest,
		"/api/v1/agent/install.sh":             s.handleAgentInstallScript,
		"/api/v1/agent/bootstrap.sh":           s.handleAgentBootstrapScript,
		"/install.sh":                          s.handleAgentInstallScript,
		"/settings/enrollment":                 s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", s.handleEnrollmentTokens)),
		"/settings/enrollment/":                s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", s.handleEnrollmentTokenActions)),
		"/settings/agent-tokens":               s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", s.handleAgentTokens)),
		"/settings/agent-tokens/":              s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", s.handleAgentTokenActions)),
		"/settings/tokens/cleanup":             s.withAdminAuth(requireScope("agents:write", s.handleTokenCleanup)),
		"/hub/ssh-public-key":                  s.withAuth(requireScope("hub:read", s.handleHubSSHPublicKey)),
		"/settings/ssh-hub-key/rotate":         s.withAdminAuth(requireScope("hub:admin", s.handleSSHHubKeyRotate)),
		"/desktop/sessions":                    s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleDesktopSessions)),
		"/desktop/sessions/":                   s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleDesktopSessionActions)),
		"/desktop/diagnose/":                   s.withAuth(requireScope("terminal:read", s.handleDesktopDiagnoseRequest)),
		"/recordings":                          s.withAuth(requireScope("terminal:read", s.handleRecordings)),
		"/recordings/":                         s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", s.handleRecordingActions)),
		"/files/":                              s.withAuth(guardV1AssetRoute("/files/", "files:read", "files:write", s.handleFiles)),
		"/processes/":                          s.withAuth(requireScopeByMethod(map[string]string{http.MethodGet: "processes:read", http.MethodHead: "processes:read", http.MethodOptions: "processes:read"}, "processes:kill", requireAssetFromPath("/processes/", s.handleProcesses))),
		"/disks/":                              s.withAuth(requireScope("disks:read", requireAssetFromPath("/disks/", s.handleDisks))),
		"/network/":                            s.withAuth(guardV1AssetRoute("/network/", "network:read", "network:write", s.handleNetworks)),
		"/packages/":                           s.withAuth(guardV1AssetRoute("/packages/", "packages:read", "packages:write", s.handlePackages)),
		"/cron/":                               s.withAuth(requireScope("cron:read", requireAssetFromPath("/cron/", s.handleCrons))),
		"/users/":                              s.withAuth(requireScope("users:read", requireAssetFromPath("/users/", s.handleUsers))),
		"/services/":                           s.withAuth(guardV1AssetRoute("/services/", "services:read", "services:write", s.handleServices)),
		"/api/v1/nodes/":                       s.withAuth(requireReadWriteScopes("assets:read", "assets:write", requireAssetFromPath("/api/v1/nodes/", s.handleNodeSubRoutes))),
		"/ws/agent":                            s.handleAgentWebSocket,
		"/ws/events":                           s.handleBrowserEvents,
		"/ws/events/ticket":                    s.withAuth(requireScope("events:subscribe", s.handleEventTicket)),
		"/agents/connected":                    s.withAuth(requireScope("agents:read", s.handleConnectedAgents)),
		"/agents/presence":                     s.withAuth(requireScope("agents:read", s.handleAgentPresence)),
		"/api/v1/agents/":                      s.withAuth(requireReadWriteScopes("agents:read", "agents:write", requireAssetFromPath("/api/v1/agents/", s.handleAgentSettingsRoutes))),
		"/api/v1/agents/pending":               s.withAuth(requireScope("agents:read", s.handleListPendingAgents)),
		"/api/v1/agents/approve":               s.withAuth(requireScope("agents:write", s.handleApproveAgent)),
		"/api/v1/agents/reject":                s.withAuth(requireScope("agents:write", s.handleRejectAgent)),
		"/dependencies":                        s.withAuth(s.handleDependencies),
		"/dependencies/":                       s.withAuth(s.handleDependencyActions),
		"/links/suggestions":                   s.withAuth(requireScope("topology:read", s.handleLinkSuggestions)),
		"/links/suggestions/":                  s.withAuth(requireReadWriteScopes("topology:read", "topology:write", s.handleLinkSuggestionActions)),
		"/links/manual":                        s.withAuth(requireScope("topology:write", s.handleManualLink)),
		"/edges":                               s.withAuth(s.handleEdges),
		"/edges/":                              s.withAuth(s.handleEdgeByID),
		"/composites":                          s.withAuth(s.handleComposites),
		"/composites/":                         s.withAuth(s.handleCompositeActions),
		"/discovery/run":                       s.withAuth(requireScope("discovery:write", s.handleDiscoveryRun)),
		"/discovery/proposals":                 s.withAuth(s.handleProposals),
		"/discovery/proposals/":                s.withAuth(s.handleProposalActions),
		"/assets/bulk/move":                    s.withAuth(requireScope("assets:write", s.handleAssetBulkMove)),
		"/synthetic-checks":                    s.withAuth(requireReadWriteScopes("assets:read", "assets:write", s.handleSyntheticChecks)),
		"/synthetic-checks/":                   s.withAuth(requireReadWriteScopes("assets:read", "assets:write", s.handleSyntheticCheckActions)),
		"/hub-collectors":                      s.withAuth(requireReadWriteScopes("collectors:read", "collectors:write", s.handleHubCollectors)),
		"/hub-collectors/":                     s.withAuth(requireReadWriteScopes("collectors:read", "collectors:write", s.handleHubCollectorActions)),
		"/api/v1/devices/register":             s.withAuth(requireScope("assets:write", s.handleDeviceRegister)),
		"/admin/reset":                         s.withAdminAuth(requireScope("settings:write", s.handleAdminReset)),
		"/api/v2/keys":                         s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleAPIKeys)),
		"/api/v2/keys/":                        s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", s.handleAPIKeyActions)),
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
		"/worker/stats":                        s.withAuth(requireScope("hub:read", s.workerStatsHandler(workerState, retentionTracker, &counters.processed, &counters.processedActions, &counters.processedUpdates))),
		// MCP (Model Context Protocol) endpoint for AI agent tool discovery
		"/mcp": s.withAuth(s.handleMCP()),
	}

	if envOrDefaultBool("DEV_MODE", false) {
		log.Printf("labtether: DEV_MODE enabled — pprof endpoints registered at /debug/pprof/")
		handlers["/debug/pprof/"] = s.withAdminAuth(pprof.Index)
		handlers["/debug/pprof/cmdline"] = s.withAdminAuth(pprof.Cmdline)
		handlers["/debug/pprof/profile"] = s.withAdminAuth(pprof.Profile)
		handlers["/debug/pprof/symbol"] = s.withAdminAuth(pprof.Symbol)
		handlers["/debug/pprof/trace"] = s.withAdminAuth(pprof.Trace)
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
