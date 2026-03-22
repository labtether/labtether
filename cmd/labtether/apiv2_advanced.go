package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
)

// --- Credentials ---

func (s *apiServer) handleV2Credentials(w http.ResponseWriter, r *http.Request) {
	scope := "credentials:read"
	if r.Method == http.MethodPost {
		scope = "credentials:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/credentials/profiles"
	apiv2.WrapV1Handler(s.handleCredentialProfiles)(w, r)
}

func (s *apiServer) handleV2CredentialActions(w http.ResponseWriter, r *http.Request) {
	scope := "credentials:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "credentials:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/credentials/profiles/", "/credentials/profiles/", 1)
	apiv2.WrapV1Handler(s.handleCredentialProfileActions)(w, r)
}

// --- Terminal Sessions ---

func (s *apiServer) handleV2TerminalSessions(w http.ResponseWriter, r *http.Request) {
	scope := "terminal:read"
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		scope = "terminal:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/terminal/sessions"
	apiv2.WrapV1Handler(s.handleSessions)(w, r)
}

func (s *apiServer) handleV2TerminalHistory(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "terminal:read") {
		apiv2.WriteScopeForbidden(w, "terminal:read")
		return
	}
	r.URL.Path = "/terminal/commands/recent"
	apiv2.WrapV1Handler(s.handleRecentCommands)(w, r)
}

func (s *apiServer) handleV2TerminalSnippets(w http.ResponseWriter, r *http.Request) {
	scope := "terminal:read"
	if r.Method == http.MethodPost {
		scope = "terminal:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/terminal/snippets"
	apiv2.WrapV1Handler(s.handleTerminalSnippets)(w, r)
}

// --- Agent Lifecycle ---

func (s *apiServer) handleV2Agents(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "agents:read") {
		apiv2.WriteScopeForbidden(w, "agents:read")
		return
	}
	r.URL.Path = "/agents/connected"
	apiv2.WrapV1Handler(s.handleConnectedAgents)(w, r)
}

func (s *apiServer) handleV2AgentActions(w http.ResponseWriter, r *http.Request) {
	scope := "agents:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "agents:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/agents/", "/api/v1/agents/", 1)
	apiv2.WrapV1Handler(s.handleAgentSettingsRoutes)(w, r)
}

func (s *apiServer) handleV2AgentsPending(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "agents:read") {
		apiv2.WriteScopeForbidden(w, "agents:read")
		return
	}
	apiv2.WrapV1Handler(s.handleListPendingAgents)(w, r)
}

func (s *apiServer) handleV2AgentsPendingApprove(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "agents:write") {
		apiv2.WriteScopeForbidden(w, "agents:write")
		return
	}
	apiv2.WrapV1Handler(s.handleApproveAgent)(w, r)
}

func (s *apiServer) handleV2AgentsPendingReject(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "agents:write") {
		apiv2.WriteScopeForbidden(w, "agents:write")
		return
	}
	apiv2.WrapV1Handler(s.handleRejectAgent)(w, r)
}

// --- Hub Status ---

func (s *apiServer) handleV2HubStatus(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:read") {
		apiv2.WriteScopeForbidden(w, "hub:read")
		return
	}
	agentCount := 0
	if s.agentMgr != nil {
		agentCount = s.agentMgr.Count()
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"status":          "running",
		"agents_connected": agentCount,
	})
}

func (s *apiServer) handleV2HubAgents(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:read") {
		apiv2.WriteScopeForbidden(w, "hub:read")
		return
	}
	r.URL.Path = "/agents/presence"
	apiv2.WrapV1Handler(s.handleAgentPresence)(w, r)
}

func (s *apiServer) handleV2HubTLS(w http.ResponseWriter, r *http.Request) {
	scope := "hub:read"
	if r.Method == http.MethodPost {
		scope = "hub:admin"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/settings/tls"
	apiv2.WrapV1Handler(s.handleTLSSettings)(w, r)
}

func (s *apiServer) handleV2HubTailscale(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:read") {
		apiv2.WriteScopeForbidden(w, "hub:read")
		return
	}
	r.URL.Path = "/settings/tailscale/serve"
	apiv2.WrapV1Handler(s.handleTailscaleServeStatus)(w, r)
}

// --- Web Services ---

func (s *apiServer) handleV2WebServices(w http.ResponseWriter, r *http.Request) {
	scope := "web-services:read"
	if r.Method == http.MethodPost {
		scope = "web-services:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/api/v1/services/web"
	apiv2.WrapV1Handler(s.handleWebServices)(w, r)
}

func (s *apiServer) handleV2WebServiceSync(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "web-services:write") {
		apiv2.WriteScopeForbidden(w, "web-services:write")
		return
	}
	r.URL.Path = "/api/v1/services/web/sync"
	apiv2.WrapV1Handler(s.handleWebServiceSync)(w, r)
}

// --- Hub Collectors ---

func (s *apiServer) handleV2Collectors(w http.ResponseWriter, r *http.Request) {
	scope := "collectors:read"
	if r.Method == http.MethodPost {
		scope = "collectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/hub-collectors"
	apiv2.WrapV1Handler(s.handleHubCollectors)(w, r)
}

func (s *apiServer) handleV2CollectorActions(w http.ResponseWriter, r *http.Request) {
	scope := "collectors:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "collectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/collectors/", "/hub-collectors/", 1)
	apiv2.WrapV1Handler(s.handleHubCollectorActions)(w, r)
}

// --- Notifications ---

func (s *apiServer) handleV2NotificationChannels(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "notifications:read") {
		apiv2.WriteScopeForbidden(w, "notifications:read")
		return
	}
	r.URL.Path = "/notifications/channels"
	apiv2.WrapV1Handler(s.handleNotificationChannels)(w, r)
}

func (s *apiServer) handleV2NotificationHistory(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "notifications:read") {
		apiv2.WriteScopeForbidden(w, "notifications:read")
		return
	}
	r.URL.Path = "/notifications/history"
	apiv2.WrapV1Handler(s.handleNotificationHistory)(w, r)
}

// --- Synthetic Checks ---

func (s *apiServer) handleV2SyntheticChecks(w http.ResponseWriter, r *http.Request) {
	scope := "assets:read" // synthetic checks are part of asset monitoring
	if r.Method == http.MethodPost {
		scope = "assets:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/synthetic-checks"
	apiv2.WrapV1Handler(s.handleSyntheticChecks)(w, r)
}

func (s *apiServer) handleV2SyntheticCheckActions(w http.ResponseWriter, r *http.Request) {
	scope := "assets:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "assets:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/synthetic-checks/", "/synthetic-checks/", 1)
	apiv2.WrapV1Handler(s.handleSyntheticCheckActions)(w, r)
}

// --- Discovery ---

func (s *apiServer) handleV2DiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "discovery:write") {
		apiv2.WriteScopeForbidden(w, "discovery:write")
		return
	}
	r.URL.Path = "/discovery/run"
	apiv2.WrapV1Handler(s.handleDiscoveryRun)(w, r)
}

func (s *apiServer) handleV2DiscoveryProposals(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "discovery:read") {
		apiv2.WriteScopeForbidden(w, "discovery:read")
		return
	}
	r.URL.Path = "/discovery/proposals"
	apiv2.WrapV1Handler(s.handleProposals)(w, r)
}

func (s *apiServer) handleV2DiscoveryProposalActions(w http.ResponseWriter, r *http.Request) {
	scope := "discovery:read"
	if r.Method == http.MethodPost {
		scope = "discovery:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/discovery/proposals/", "/discovery/proposals/", 1)
	apiv2.WrapV1Handler(s.handleProposalActions)(w, r)
}

// --- Topology ---

func (s *apiServer) handleV2Dependencies(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/dependencies"
	apiv2.WrapV1Handler(s.handleDependencies)(w, r)
}

func (s *apiServer) handleV2DependencyActions(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/dependencies/", "/dependencies/", 1)
	apiv2.WrapV1Handler(s.handleDependencyActions)(w, r)
}

func (s *apiServer) handleV2Edges(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/edges"
	apiv2.WrapV1Handler(s.handleEdges)(w, r)
}

func (s *apiServer) handleV2Composites(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/composites"
	apiv2.WrapV1Handler(s.handleComposites)(w, r)
}

func (s *apiServer) handleV2CompositeActions(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/composites/", "/composites/", 1)
	apiv2.WrapV1Handler(s.handleCompositeActions)(w, r)
}

// --- Failover ---

func (s *apiServer) handleV2FailoverPairs(w http.ResponseWriter, r *http.Request) {
	scope := "failover:read"
	if r.Method == http.MethodPost {
		scope = "failover:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/group-failover-pairs"
	apiv2.WrapV1Handler(s.handleFailoverPairs)(w, r)
}

func (s *apiServer) handleV2FailoverPairActions(w http.ResponseWriter, r *http.Request) {
	scope := "failover:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "failover:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/failover-pairs/", "/group-failover-pairs/", 1)
	apiv2.WrapV1Handler(s.handleFailoverPairActions)(w, r)
}

// --- Dead Letters ---

func (s *apiServer) handleV2DeadLetters(w http.ResponseWriter, r *http.Request) {
	scope := "dead-letters:read"
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		scope = "dead-letters:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/queue/dead-letters"
	apiv2.WrapV1Handler(s.handleDeadLetters)(w, r)
}

// --- Audit ---

func (s *apiServer) handleV2AuditEvents(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "audit:read") {
		apiv2.WriteScopeForbidden(w, "audit:read")
		return
	}
	r.URL.Path = "/audit/events"
	apiv2.WrapV1Handler(s.handleAuditEvents)(w, r)
}

// --- Log Views ---

func (s *apiServer) handleV2LogViews(w http.ResponseWriter, r *http.Request) {
	scope := "logs:read"
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		scope = "logs:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/logs/views"
	apiv2.WrapV1Handler(s.handleLogViews)(w, r)
}

func (s *apiServer) handleV2LogViewActions(w http.ResponseWriter, r *http.Request) {
	scope := "logs:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "logs:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/logs/views/", "/logs/views/", 1)
	apiv2.WrapV1Handler(s.handleLogViewActions)(w, r)
}

// --- Prometheus Settings ---

func (s *apiServer) handleV2PrometheusSettings(w http.ResponseWriter, r *http.Request) {
	scope := "settings:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "settings:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/settings/runtime"
	apiv2.WrapV1Handler(s.handleRuntimeSettings)(w, r)
}

// --- File Transfers ---

// handleV2FileTransfers handles collection-level requests:
//
//	POST /api/v2/file-transfers  – start a new transfer (scope: files:write)
//	GET  /api/v2/file-transfers  – list transfers (scope: files:read; returns
//	                              501 since the v1 layer has no list endpoint)
func (s *apiServer) handleV2FileTransfers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:read") {
			apiv2.WriteScopeForbidden(w, "files:read")
			return
		}
		apiv2.WriteError(w, http.StatusNotImplemented, "not_implemented",
			"listing all file transfers is not yet supported; query individual transfers by ID")
	case http.MethodPost:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:write") {
			apiv2.WriteScopeForbidden(w, "files:write")
			return
		}
		r.URL.Path = "/api/v1/file-transfers"
		apiv2.WrapV1Handler(s.handleFileTransfers)(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// handleV2FileTransferActions handles /api/v2/file-transfers/{id}:
//
//	GET    – get transfer status (scope: files:read)
//	DELETE – cancel a transfer  (scope: files:write)
func (s *apiServer) handleV2FileTransferActions(w http.ResponseWriter, r *http.Request) {
	transferID := strings.TrimPrefix(r.URL.Path, "/api/v2/file-transfers/")
	transferID = strings.TrimRight(transferID, "/")
	if transferID == "" || strings.Contains(transferID, "/") {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "transfer id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:read") {
			apiv2.WriteScopeForbidden(w, "files:read")
			return
		}
	case http.MethodDelete:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:write") {
			apiv2.WriteScopeForbidden(w, "files:write")
			return
		}
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	// Rewrite to the v1 path so the existing handler can parse the ID.
	r.URL.Path = "/api/v1/file-transfers/" + transferID
	apiv2.WrapV1Handler(s.handleFileTransfers)(w, r)
}

// --- Prometheus Test ---

// handleV2PrometheusTest handles POST /api/v2/settings/prometheus/test.
// It delegates to the same connection-test logic used by the v1 endpoint
// (/settings/prometheus/test-connection), wrapped in the v2 response envelope.
func (s *apiServer) handleV2PrometheusTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "settings:write") {
		apiv2.WriteScopeForbidden(w, "settings:write")
		return
	}

	if !s.enforceRateLimit(
		w, r,
		prometheusTestConnectionRateLimitKey,
		prometheusTestConnectionRateLimitCount,
		prometheusTestConnectionRateLimitWindow,
	) {
		return
	}

	var req prometheusTestConnectionRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		apiv2.WriteJSON(w, http.StatusOK, prometheusTestConnectionResponse{
			Success: false,
			Error:   errPrometheusURLRequired,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), prometheusTestConnectionTimeout)
	defer cancel()

	result := testPrometheusRemoteWriteConnection(ctx, targetURL, req.Username, req.Password)
	apiv2.WriteJSON(w, http.StatusOK, result)
}

// --- TLS Renew ---

// handleV2HubTLSRenew handles POST /api/v2/hub/tls/renew.
//
// Behaviour by TLS source:
//   - built-in (self-signed):  forces immediate renewal via certmgr.CertReloader.
//   - tailscale:               re-runs `tailscale cert` via the tailscale reloader.
//   - uploaded / external:     returns 422 — those certs cannot be renewed
//     by the hub automatically.
//   - TLS disabled:            returns 422.
func (s *apiServer) handleV2HubTLSRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "settings:write") {
		apiv2.WriteScopeForbidden(w, "settings:write")
		return
	}

	switch s.tlsState.Source {
	case tlsSourceBuiltIn:
		if s.tlsState.CertReloader == nil {
			apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "built-in cert reloader is not initialised")
			return
		}
		if err := s.tlsState.CertReloader.ForceRenew(); err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "renewal_failed", "failed to renew built-in certificate: "+err.Error())
			return
		}
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "hub.tls.renewed",
			ActorID:   principalActorID(r.Context()),
			Details:   map[string]any{"tls_source": tlsSourceBuiltIn},
			Timestamp: time.Now().UTC(),
		}, "v2 hub tls renew (built-in)")
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "renewed", "tls_source": tlsSourceBuiltIn})

	case tlsSourceTailscale:
		tsrl, tsrlOK := s.tlsState.TailscaleCertReloader.(*tailscaleCertReloader)
		if !tsrlOK || tsrl == nil {
			apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "tailscale cert reloader is not initialised")
			return
		}
		if err := tsrl.renew(); err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "renewal_failed", "failed to renew tailscale certificate: "+err.Error())
			return
		}
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "hub.tls.renewed",
			ActorID:   principalActorID(r.Context()),
			Details:   map[string]any{"tls_source": tlsSourceTailscale},
			Timestamp: time.Now().UTC(),
		}, "v2 hub tls renew (tailscale)")
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "renewed", "tls_source": tlsSourceTailscale})

	case tlsSourceUIUploaded, tlsSourceDeploymentExternal:
		apiv2.WriteError(w, http.StatusUnprocessableEntity, "unsupported_source",
			"certificate renewal is not supported for source '"+s.tlsState.Source+"'; upload a new certificate via POST /api/v2/hub/tls")

	default:
		apiv2.WriteError(w, http.StatusUnprocessableEntity, "tls_disabled",
			"TLS is not enabled on this hub; renewal is not applicable")
	}
}
