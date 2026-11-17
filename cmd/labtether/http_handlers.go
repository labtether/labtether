package main

import (
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/labtether/labtether/internal/hubapi/shared"
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
		"/":                      s.handleHubRoot,
		"/status/aggregate":      s.withAuth(requireScope("hub:read", s.handleStatusAggregate(retentionTracker, &counters.processed))),
		"/status/aggregate/live": s.withAuth(requireScope("hub:read", s.handleStatusAggregateLive())),
		"/terminal/preferences":  s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleTerminalPreferences(w, r) })),
		"/terminal/snippets":     s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleTerminalSnippets(w, r) })),
		"/terminal/snippets/": s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureTerminalDeps().HandleTerminalSnippetActions(w, r)
		})),
		"/terminal/workspace/tabs":      s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleWorkspaceTabs(w, r) })),
		"/terminal/workspace/tabs/":     s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleWorkspaceTabActions(w, r) })),
		"/terminal/persistent-sessions": s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandlePersistentSessions(w, r) })),
		"/terminal/persistent-sessions/": s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureTerminalDeps().HandlePersistentSessionActions(w, r)
		})),
		"/terminal/sessions":        s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleSessions(w, r) })),
		"/terminal/sessions/":       s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleSessionActions(w, r) })),
		"/terminal/quick-session":   s.withAuth(requireScope("terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleQuickSession(w, r) })),
		"/terminal/host-keys/":      s.withAuth(requireScope("terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleSSHHostKeys(w, r) })),
		"/terminal/bookmarks":       s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleBookmarks(w, r) })),
		"/terminal/bookmarks/":      s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleBookmarkActions(w, r) })),
		"/terminal/commands/recent": s.withAuth(requireScope("terminal:read", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleRecentCommands(w, r) })),
		"/terminal/commands/":       s.withAuth(requireScope("terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureTerminalDeps().HandleCommandActions(w, r) })),
		"/group-profiles":           s.withAuth(requireReadWriteScopes("groups:read", "groups:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleGroupProfiles(w, r) })),
		"/group-profiles/":          s.withAuth(requireReadWriteScopes("groups:read", "groups:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleGroupProfileActions(w, r) })),
		"/group-failover-pairs":     s.withAuth(requireReadWriteScopes("failover:read", "failover:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFailoverPairs(w, r) })),
		"/group-failover-pairs/":    s.withAuth(requireReadWriteScopes("failover:read", "failover:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFailoverPairActions(w, r) })),
		"/groups":                   s.withAuth(requireReadWriteScopes("groups:read", "groups:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleGroups(w, r) })),
		"/groups/":                  s.withAuth(requireReadWriteScopes("groups:read", "groups:write", s.handleGroupActions)),
		"/assets":                   s.withAuth(requireScope("assets:read", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAssets(w, r) })),
		"/assets/manual":            s.withAuth(requireScope("assets:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleManualDeviceRoutes(w, r) })),
		"/assets/heartbeat":         s.withAgentHeartbeatAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAssetActions(w, r) }),
		"/assets/":                  s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAssetActions(w, r) }),
		"/credentials/profiles":     s.withAuth(requireReadWriteScopes("credentials:read", "credentials:write", s.handleCredentialProfiles)),
		"/credentials/profiles/":    s.withAuth(requireReadWriteScopes("credentials:read", "credentials:write", s.handleCredentialProfileActions)),
		// /metrics is an exact-match path (no trailing slash) so it does not
		// conflict with /metrics/overview or /metrics/assets/ above. Scrapers
		// authenticate with an API key carrying metrics:read.
		"/metrics":          s.withAuth(requireScope("metrics:read", guardAssetRestrictedGlobalAPI("Prometheus metrics", s.handlePrometheusMetrics()))),
		"/metrics/overview": s.withAuth(requireScope("metrics:read", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleMetricsOverview(w, r) })),
		"/metrics/assets/":  s.withAuth(requireScope("metrics:read", requireAssetFromPath("/metrics/assets/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAssetMetrics(w, r) }))),
		"/logs/query":       s.withAuth(requireScope("logs:read", s.handleLogsQuery)),
		"/logs/journal/":    s.withAuth(requireScope("logs:read", requireAssetFromPath("/logs/journal/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleJournalLogs(w, r) }))),
		"/logs/sources":     s.withAuth(requireScope("logs:read", s.handleLogSources)),
		"/logs/views":       s.withAuth(requireReadWriteScopes("logs:read", "logs:write", s.handleLogViews)),
		"/logs/views/":      s.withAuth(requireReadWriteScopes("logs:read", "logs:write", s.handleLogViewActions)),
		"/telemetry/frontend/perf": s.withAuth(requireScope("logs:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureResourcesDeps().HandleFrontendPerfTelemetry(w, r)
		})),
		"/telemetry/mobile/client": s.withAuth(requireScope("logs:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureResourcesDeps().HandleMobileClientTelemetry(w, r)
		})),
		"/queue/dead-letters":     s.withAuth(requireReadWriteScopes("dead-letters:read", "dead-letters:write", s.handleDeadLetters)),
		"/connectors":             s.withAuth(requireScope("connectors:read", s.handleListConnectors)),
		"/connectors/":            s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleConnectorActions)),
		"/proxmox/assets/":        s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxAssets)),
		"/proxmox/tasks/":         s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxTaskRoutes)),
		"/proxmox/cluster/status": s.withAuth(requireScope("connectors:read", func(w http.ResponseWriter, r *http.Request) { s.ensureProxmoxDeps().HandleProxmoxClusterStatus(w, r) })),
		"/proxmox/cluster/resources": s.withAuth(requireScope("connectors:read", func(w http.ResponseWriter, r *http.Request) {
			s.ensureProxmoxDeps().HandleProxmoxClusterResources(w, r)
		})),
		"/proxmox/nodes/":                            s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleProxmoxNodeRoutes)),
		"/proxmox/ceph/":                             s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.guardIntegrationActions(s.ensureProxmoxDeps().HandleProxmoxCeph))),
		"/proxmox/ceph/status":                       s.withAuth(requireScope("connectors:read", func(w http.ResponseWriter, r *http.Request) { s.ensureProxmoxDeps().HandleProxmoxCephStatus(w, r) })),
		"/pbs/assets/":                               s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handlePBSAssets)),
		"/pbs/tasks/":                                s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handlePBSTaskRoutes)),
		"/truenas/assets/":                           s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handleTrueNASAssets)),
		"/portainer/endpoints":                       s.withAuth(requireScope("connectors:read", func(w http.ResponseWriter, r *http.Request) { s.ensurePortainerDeps().HandlePortainerEndpoints(w, r) })),
		"/portainer/assets/":                         s.withAuth(requireReadWriteScopes("connectors:read", "connectors:write", s.handlePortainerAssets)),
		"/api/v1/services/web":                       s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServices)),
		"/api/v1/services/web/compat":                s.withAuth(requireScope("web-services:read", s.handleWebServiceCompat)),
		"/api/v1/services/web/categories":            s.withAuth(requireScope("web-services:read", s.handleWebServiceCategories)),
		"/api/v1/services/web/sync":                  s.withAuth(requireScope("web-services:write", s.handleWebServiceSync)),
		"/api/v1/services/web/manual":                s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceManual)),
		"/api/v1/services/web/manual/":               s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceManualActions)),
		"/api/v1/services/web/overrides":             s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceOverrides)),
		"/api/v1/services/web/icon-library":          s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceIconLibrary)),
		"/api/v1/services/web/alt-urls/":             s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceAltURLs)),
		"/api/v1/services/web/never-group-rules":     s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceNeverGroupRules)),
		"/api/v1/services/web/grouping-settings":     s.withAuth(requireReadWriteScopes("web-services:read", "web-services:write", s.handleWebServiceGroupingSettings)),
		"/api/v1/services/web/grouping-suggestions/": s.withAuth(requireScope("web-services:write", s.handleWebServiceGroupingSuggestionResponse)),
		"/api/v1/docker/hosts":                       s.withAuth(requireReadWriteScopes("docker:read", "docker:write", func(w http.ResponseWriter, r *http.Request) { s.ensureDockerDeps().HandleDockerHosts(w, r) })),
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
		"/api/v2/terminal/sessions":  s.withAuth(s.handleV2TerminalSessions),
		"/api/v2/terminal/history":   s.withAuth(s.handleV2TerminalHistory),
		"/api/v2/terminal/history/":  s.withAuth(s.handleV2TerminalHistoryActions),
		"/api/v2/terminal/snippets":  s.withAuth(s.handleV2TerminalSnippets),
		"/api/v2/terminal/snippets/": s.withAuth(s.handleV2TerminalSnippetActions),
		// Agents
		"/api/v2/agents":                 s.withAuth(s.handleV2Agents),
		"/api/v2/agents/":                s.withAuth(s.handleV2AgentActions),
		"/api/v2/agents/pending":         s.withAuth(s.handleV2AgentsPending),
		"/api/v2/agents/pending/approve": s.withAdminAuth(s.handleV2AgentsPendingApprove),
		"/api/v2/agents/pending/reject":  s.withAuth(s.handleV2AgentsPendingReject),
		// Hub
		"/api/v2/hub/status":    s.withAuth(s.handleV2HubStatus),
		"/api/v2/hub/agents":    s.withAuth(s.handleV2HubAgents),
		"/api/v2/hub/tls":       s.withAdminAuth(s.handleV2HubTLS),
		"/api/v2/hub/tls/renew": s.withAdminAuth(s.handleV2HubTLSRenew),
		"/api/v2/hub/tailscale": s.withAuth(s.handleV2HubTailscale),
		// Web Services
		"/api/v2/web-services":      s.withAuth(s.handleV2WebServices),
		"/api/v2/web-services/":     s.withAuth(s.handleV2WebServiceActions),
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
		"/api/v2/edges/":        s.withAuth(s.handleV2EdgeActions),
		"/api/v2/composites":    s.withAuth(s.handleV2Composites),
		"/api/v2/composites/":   s.withAuth(s.handleV2CompositeActions),
		// Topology Canvas
		"/api/v2/topology":              s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2Topology)),
		"/api/v2/topology/zones":        s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyZones)),
		"/api/v2/topology/zones/":       s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyZoneActions)),
		"/api/v2/topology/connections":  s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyConnections)),
		"/api/v2/topology/connections/": s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyConnection)),
		"/api/v2/topology/viewport":     s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyViewport)),
		"/api/v2/topology/unsorted":     s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyUnsorted)),
		"/api/v2/topology/auto-place":   s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyAutoPlace)),
		"/api/v2/topology/reset":        s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyReset)),
		"/api/v2/topology/dismiss":      s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyDismiss)),
		"/api/v2/topology/dismiss/":     s.withAuth(guardAssetRestrictedGlobalAPI("topology canvas", s.handleV2TopologyUndismiss)),
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
		"/api/v2/settings/prometheus":      s.withAdminAuth(s.handleV2PrometheusSettings),
		"/api/v2/settings/prometheus/test": s.withAdminAuth(s.handleV2PrometheusTest),
		// Events
		"/api/v2/events/stream":     s.withAuth(s.handleV2EventStream),
		"/api/v1/file-connections":  s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFileConnections(w, r) })),
		"/api/v1/file-connections/": s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFileConnections(w, r) })),
		"/api/v1/remote-bookmarks":  s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleRemoteBookmarks(w, r) })),
		"/api/v1/remote-bookmarks/": s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleRemoteBookmarks(w, r) })),
		"/api/v1/file-transfers":    s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFileTransfers(w, r) })),
		"/api/v1/file-transfers/":   s.withAuth(requireReadWriteScopes("files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFileTransfers(w, r) })),
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
		"/auth/me/password":                    s.withSelfServiceAuth(s.handleChangePassword),
		"/auth/account":                        s.withSelfServiceAuth(s.handleDeleteOwnAccount),
		"/auth/2fa/setup":                      s.withSelfServiceAuth(s.handle2FASetup),
		"/auth/2fa/verify":                     s.withSelfServiceAuth(s.handle2FAVerify),
		"/auth/2fa":                            s.withSelfServiceAuth(s.handle2FADisable),
		"/auth/2fa/recovery-codes":             s.withSelfServiceAuth(s.handle2FARecoveryCodes),
		"/auth/providers":                      s.handleAuthProviders,
		"/auth/oidc/start":                     s.handleAuthOIDCStart,
		"/auth/oidc/callback":                  s.handleAuthOIDCCallback,
		"/auth/oidc/mobile/start":              s.handleAuthOIDCMobileStart,
		"/auth/oidc/mobile/callback":           s.handleAuthOIDCMobileCallback,
		"/auth/users":                          s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("user administration", s.handleAuthUsers))),
		"/auth/users/":                         s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("user administration", s.handleAuthUserActions))),
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
		"/live-activities/incidents/":          s.withAuth(requireScope("alerts:read", s.handleIncidentLiveActivityTokens)),
		"/settings/retention":                  s.withAuth(requireAdminForMutation(requireReadWriteScopes("settings:read", "settings:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleRetentionSettings(w, r) }))),
		"/settings/oidc":                       s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("OIDC settings", s.handleOIDCSettings))),
		"/settings/oidc/apply":                 s.withAdminAuth(requireScope("settings:write", guardAssetRestrictedGlobalAPI("OIDC settings", s.handleOIDCSettingsApply))),
		"/settings/managed-database":           s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("managed database settings", s.handleManagedDatabaseSettings))),
		"/settings/managed-database/reveal":    s.withAdminAuth(requireScope("settings:read", requireScope("credentials:use", guardAssetRestrictedGlobalAPI("managed database credentials", s.handleManagedDatabasePasswordReveal)))),
		"/settings/runtime":                    s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("runtime settings", s.handleRuntimeSettings))),
		"/settings/runtime/reset":              s.withAdminAuth(requireScope("settings:write", guardAssetRestrictedGlobalAPI("runtime settings", s.handleRuntimeSettingsReset))),
		"/settings/restart":                    s.withAdminAuth(requireScope("settings:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleRestartSettings(w, r) })),
		"/settings/tls":                        s.withAdminAuth(requireReadWriteScopes("hub:read", "hub:admin", guardAssetRestrictedGlobalAPI("TLS settings", s.handleTLSSettings))),
		"/settings/prometheus/test-connection": s.withAdminAuth(requireScope("settings:read", guardAssetRestrictedGlobalAPI("Prometheus settings", s.handlePrometheusTestConnection))),
		"/settings/tailscale/serve":            s.withAuth(requireReadWriteScopes("hub:read", "hub:admin", guardAssetRestrictedGlobalAPI("Tailscale Serve settings", s.handleTailscaleServeStatus))),
		"/audit/events":                        s.withAuth(requireScope("audit:read", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAuditEvents(w, r) })),
		"/policy/check":                        s.withAuth(requireScope("settings:read", s.handlePolicyCheck)),
		"/api/v1/enroll":                       func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleEnroll(w, r) },
		"/api/v1/discover":                     func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleDiscover(w, r) },
		"/api/v1/ca.crt":                       s.handleCACert,
		"/api/v1/tls/info":                     s.handleTLSInfo,
		"/api/v1/agent/binary":                 func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentBinary(w, r) },
		"/api/v1/agent/releases/latest":        func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentReleaseLatest(w, r) },
		"/api/v1/agent/manifest":               s.withAuth(requireScope("agents:read", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentManifest(w, r) })),
		"/api/v1/agent/cache/refresh":          s.withAdminAuth(requireScope("agents:write", guardAssetRestrictedGlobalAPI("agent cache refresh", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentCacheRefresh(w, r) }))),
		"/api/v1/agent/install.sh":             func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentInstallScript(w, r) },
		"/api/v1/agent/bootstrap.sh":           func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentBootstrapScript(w, r) },
		"/install.sh":                          func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentInstallScript(w, r) },
		"/settings/enrollment":                 s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", guardAssetRestrictedGlobalAPI("enrollment tokens", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleEnrollmentTokens(w, r) }))),
		"/settings/enrollment/":                s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", guardAssetRestrictedGlobalAPI("enrollment tokens", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleEnrollmentTokenActions(w, r) }))),
		"/settings/agent-tokens":               s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", guardAssetRestrictedGlobalAPI("agent tokens", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentTokens(w, r) }))),
		"/settings/agent-tokens/":              s.withAdminAuth(requireReadWriteScopes("agents:read", "agents:write", guardAssetRestrictedGlobalAPI("agent tokens", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentTokenActions(w, r) }))),
		"/settings/tokens/cleanup":             s.withAdminAuth(requireScope("agents:write", guardAssetRestrictedGlobalAPI("agent token cleanup", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleTokenCleanup(w, r) }))),
		"/hub/ssh-public-key":                  s.withAuth(requireScope("hub:read", guardAssetRestrictedGlobalAPI("hub SSH public key", s.handleHubSSHPublicKey))),
		"/settings/ssh-hub-key":                s.withAdminAuth(requireScope("hub:read", guardAssetRestrictedGlobalAPI("hub SSH key settings", s.handleHubSSHPublicKey))),
		"/settings/ssh-hub-key/rotate":         s.withAdminAuth(requireScope("hub:admin", guardAssetRestrictedGlobalAPI("hub SSH key rotation", s.handleSSHHubKeyRotate))),
		"/desktop/sessions":                    s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleDesktopSessions(w, r) })),
		"/desktop/sessions/":                   s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleDesktopSessionActions(w, r) })),
		"/desktop/diagnose/":                   s.withAuth(requireScope("terminal:read", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleDesktopDiagnoseRequest(w, r) })),
		"/recordings":                          s.withAuth(requireScope("terminal:read", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleRecordings(w, r) })),
		"/recordings/":                         s.withAuth(requireReadWriteScopes("terminal:read", "terminal:write", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleRecordingActions(w, r) })),
		"/files/":                              s.withAuth(guardV1AssetRoute("/files/", "files:read", "files:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleFiles(w, r) })),
		"/processes/":                          s.withAuth(requireScopeByMethod(map[string]string{http.MethodGet: "processes:read", http.MethodHead: "processes:read", http.MethodOptions: "processes:read"}, "processes:kill", requireAssetFromPath("/processes/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleProcesses(w, r) }))),
		"/disks/":                              s.withAuth(requireScope("disks:read", requireAssetFromPath("/disks/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleDisks(w, r) }))),
		"/network/":                            s.withAuth(guardV1AssetRoute("/network/", "network:read", "network:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleNetworks(w, r) })),
		"/packages/":                           s.withAuth(guardV1AssetRoute("/packages/", "packages:read", "packages:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandlePackages(w, r) })),
		"/cron/":                               s.withAuth(requireScope("cron:read", requireAssetFromPath("/cron/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleCrons(w, r) }))),
		"/users/":                              s.withAuth(requireScope("users:read", requireAssetFromPath("/users/", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleUsers(w, r) }))),
		"/services/":                           s.withAuth(guardV1AssetRoute("/services/", "services:read", "services:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleServices(w, r) })),
		"/api/v1/nodes/":                       s.withAuth(requireReadWriteScopes("assets:read", "assets:write", requireAssetFromPath("/api/v1/nodes/", func(w http.ResponseWriter, r *http.Request) { s.ensureDesktopDeps().HandleNodeSubRoutes(w, r) }))),
		"/ws/agent":                            s.handleAgentWebSocket,
		"/ws/events":                           s.handleBrowserEvents,
		"/ws/events/ticket":                    s.withReadCapabilityAuth(requireScope("events:subscribe", s.handleEventTicket)),
		"/agents/connected":                    s.withAuth(requireScope("agents:read", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleConnectedAgents(w, r) })),
		"/agents/presence":                     s.withAuth(requireScope("agents:read", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentPresence(w, r) })),
		"/api/v1/agents/":                      s.withAuth(requireReadWriteScopes("agents:read", "agents:write", requireAssetFromPath("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleAgentSettingsRoutes(w, r) }))),
		"/api/v1/agents/pending":               s.withAuth(requireScope("agents:read", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleListPendingAgents(w, r) })),
		"/api/v1/agents/approve":               s.withAdminAuth(requireScope("agents:write", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleApproveAgent(w, r) })),
		"/api/v1/agents/reject":                s.withAuth(requireScope("agents:write", func(w http.ResponseWriter, r *http.Request) { s.ensureAgentsDeps().HandleRejectAgent(w, r) })),
		"/dependencies":                        s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleDependencies(w, r) }),
		"/dependencies/":                       s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleDependencyActions(w, r) }),
		"/links/suggestions":                   s.withAuth(requireScope("topology:read", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleLinkSuggestions(w, r) })),
		"/links/suggestions/": s.withAuth(requireReadWriteScopes("topology:read", "topology:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureResourcesDeps().HandleLinkSuggestionActions(w, r)
		})),
		"/links/manual":         s.withAuth(requireScope("topology:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleManualLink(w, r) })),
		"/edges":                s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleEdges(w, r) }),
		"/edges/":               s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleEdgeByID(w, r) }),
		"/composites":           s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleComposites(w, r) }),
		"/composites/":          s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleCompositeActions(w, r) }),
		"/discovery/run":        s.withAuth(requireScope("discovery:write", s.handleDiscoveryRun)),
		"/discovery/proposals":  s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleProposals(w, r) }),
		"/discovery/proposals/": s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleProposalActions(w, r) }),
		"/assets/bulk/move":     s.withAuth(requireScope("assets:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleAssetBulkMove(w, r) })),
		"/synthetic-checks":     s.withAuth(requireReadWriteScopes("assets:read", "assets:write", func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleSyntheticChecks(w, r) })),
		"/synthetic-checks/": s.withAuth(requireReadWriteScopes("assets:read", "assets:write", func(w http.ResponseWriter, r *http.Request) {
			s.ensureResourcesDeps().HandleSyntheticCheckActions(w, r)
		})),
		"/hub-collectors":  s.withAuth(requireReadWriteScopes("collectors:read", "collectors:write", s.handleHubCollectors)),
		"/hub-collectors/": s.withAuth(requireReadWriteScopes("collectors:read", "collectors:write", s.handleHubCollectorActions)),
		// Push registration is self-scoped to the authenticated user's device;
		// viewer accounts must be able to opt in/out without asset mutation rights.
		"/api/v1/devices/register":    s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleDeviceRegister(w, r) }),
		"/api/v1/devices/deregister":  s.withAuth(func(w http.ResponseWriter, r *http.Request) { s.ensureResourcesDeps().HandleDeviceDeregister(w, r) }),
		"/admin/reset":                s.withAdminAuth(requireScope("settings:write", guardAssetRestrictedGlobalAPI("administrative reset", s.handleAdminReset))),
		"/api/v2/keys":                s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("API key administration", s.handleAPIKeys))),
		"/api/v2/keys/":               s.withAdminAuth(requireReadWriteScopes("settings:read", "settings:write", guardAssetRestrictedGlobalAPI("API key administration", s.handleAPIKeyActions))),
		"/api/v2/schedules":           s.withAuth(s.handleV2Schedules),
		"/api/v2/schedules/":          s.withAuth(s.handleV2ScheduleActions),
		"/api/v2/webhooks":            s.withAuth(s.handleV2Webhooks),
		"/api/v2/webhooks/":           s.withAuth(s.handleV2WebhookActions),
		"/api/v2/whoami":              s.withAuth(s.handleV2Whoami),
		"/api/v2/assets":              s.withAuth(s.handleV2Assets),
		"/api/v2/assets/":             s.withAuth(s.handleV2AssetActions),
		"/api/v2/exec":                s.withAuth(s.handleV2ExecMulti),
		"/api/v2/actions":             s.withAuth(s.handleV2SavedActions),
		"/api/v2/actions/":            s.withAuth(s.handleV2SavedActionActions),
		"/api/v2/search":              s.withAuth(s.handleV2Search),
		"/api/v2/bulk/service-action": s.withAuth(s.handleV2BulkServiceAction),
		"/api/v2/bulk/file-push":      s.withAuth(s.handleV2BulkFilePush),
		"/worker/stats":               s.withAuth(requireScope("hub:read", guardAssetRestrictedGlobalAPI("worker statistics", s.workerStatsHandler(workerState, retentionTracker, &counters.processed, &counters.processedActions, &counters.processedUpdates, &counters.processedSchedules)))),
		// MCP (Model Context Protocol) endpoint for AI agent tool discovery
		// MCP applies scopes and asset allow-lists per tool/resource. Keeping a
		// global-resource guard here would reject every asset-restricted key
		// before those narrower checks can run.
		// MCP encodes both reads and mutations as POST requests. Authenticate the
		// transport here, then let per-tool scopes, asset allowlists, and the MCP
		// mutation policy enforce the operation-specific authorization boundary.
		"/mcp": s.withReadCapabilityAuth(s.handleMCP()),
	}

	if shared.EnvOrDefaultBool("DEV_MODE", false) {
		log.Printf("labtether: DEV_MODE enabled — pprof endpoints registered at /debug/pprof/")
		handlers["/debug/pprof/"] = s.withAdminAuth(guardAssetRestrictedGlobalAPI("debug profiles", pprof.Index))
		handlers["/debug/pprof/cmdline"] = s.withAdminAuth(guardAssetRestrictedGlobalAPI("debug profiles", pprof.Cmdline))
		handlers["/debug/pprof/profile"] = s.withAdminAuth(guardAssetRestrictedGlobalAPI("debug profiles", pprof.Profile))
		handlers["/debug/pprof/symbol"] = s.withAdminAuth(guardAssetRestrictedGlobalAPI("debug profiles", pprof.Symbol))
		handlers["/debug/pprof/trace"] = s.withAdminAuth(guardAssetRestrictedGlobalAPI("debug profiles", pprof.Trace))
	}

	return handlers
}

// handlePrometheusMetrics returns an http.HandlerFunc that serves the
// Prometheus metrics exposition format at the exact path /metrics.
//
// The endpoint requires a bearer credential carrying metrics:read and is also
// gated by the prometheus.scrape_enabled runtime setting (default: false).
// When disabled it returns 404 with a plain-text explanation.
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
