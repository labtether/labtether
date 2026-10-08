package main

import (
	"github.com/labtether/labtether/internal/apiv2"
	"net/http"
	"strings"
)

// --- Credentials ---

func (s *apiServer) handleV2Credentials(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobalAPI(w, r, "credential profiles") {
		return
	}
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
	if denyAssetRestrictedGlobalAPI(w, r, "credential profiles") {
		return
	}
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

func (s *apiServer) handleV2TerminalHistoryActions(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "terminal:write") {
		apiv2.WriteScopeForbidden(w, "terminal:write")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/terminal/history/", "/terminal/commands/", 1)
	apiv2.WrapV1Handler(s.handleTerminalCommandActions)(w, r)
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

func (s *apiServer) handleV2TerminalSnippetActions(w http.ResponseWriter, r *http.Request) {
	scope := "terminal:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "terminal:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/terminal/snippets/", "/terminal/snippets/", 1)
	apiv2.WrapV1Handler(s.handleTerminalSnippetActions)(w, r)
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

// --- Web Services ---

func (s *apiServer) handleV2WebServices(w http.ResponseWriter, r *http.Request) {
	var (
		scope   string
		path    string
		handler http.HandlerFunc
	)
	switch r.Method {
	case http.MethodGet:
		scope = "web-services:read"
		path = "/api/v1/services/web"
		handler = s.handleWebServices
	case http.MethodPost:
		scope = "web-services:write"
		path = "/api/v1/services/web/manual"
		handler = s.handleWebServiceManual
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = path
	apiv2.WrapV1Handler(handler)(w, r)
}

func (s *apiServer) handleV2WebServiceSync(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "web-services:write") {
		apiv2.WriteScopeForbidden(w, "web-services:write")
		return
	}
	r.URL.Path = "/api/v1/services/web/sync"
	apiv2.WrapV1Handler(s.handleWebServiceSync)(w, r)
}

func (s *apiServer) handleV2WebServiceActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PATCH, PUT, or DELETE required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "web-services:write") {
		apiv2.WriteScopeForbidden(w, "web-services:write")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/web-services/", "/api/v1/services/web/manual/", 1)
	apiv2.WrapV1Handler(s.handleWebServiceManualActions)(w, r)
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
	scope := "notifications:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "notifications:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
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

// --- Dead Letters ---

func (s *apiServer) handleV2DeadLetters(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobalAPI(w, r, "dead-letter jobs") {
		return
	}
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
	if !s.requireAdminAuth(w, r) {
		return
	}
	if denyAssetRestrictedGlobalAPI(w, r, "Prometheus settings") {
		return
	}
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
//	GET  /api/v2/file-transfers  – list the authenticated actor's transfers
//	                              (scope: files:read)
func (s *apiServer) handleV2FileTransfers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:read") {
			apiv2.WriteScopeForbidden(w, "files:read")
			return
		}
		r.URL.Path = "/api/v1/file-transfers"
		apiv2.WrapV1Handler(s.handleFileTransfers)(w, r)
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
	if !s.requireAdminAuth(w, r) {
		return
	}
	if denyAssetRestrictedGlobalAPI(w, r, "Prometheus connection tests") {
		return
	}
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "settings:write") {
		apiv2.WriteScopeForbidden(w, "settings:write")
		return
	}

	r.URL.Path = "/settings/prometheus/test-connection"
	apiv2.WrapV1Handler(s.handlePrometheusTestConnection)(w, r)
}
