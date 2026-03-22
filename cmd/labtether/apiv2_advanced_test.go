package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testV2ScopeDenied is a helper that asserts a handler returns 403 when the
// request context carries a scope that does not satisfy the handler's required
// scope.
func testV2ScopeDenied(t *testing.T, s *apiServer, method, path string, handler http.HandlerFunc) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"nonexistent:scope"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%s %s: expected 403, got %d", method, path, rec.Code)
	}
}

// TestHandleV2Advanced_ScopeDenied covers scope denial for all major domains
// added in Plan 4.
func TestHandleV2Advanced_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		// Home Assistant
		{
			name:    "HA entities read",
			method:  http.MethodGet,
			path:    "/api/v2/homeassistant/entities",
			handler: s.handleV2HAEntities,
		},
		{
			name:    "HA automations",
			method:  http.MethodGet,
			path:    "/api/v2/homeassistant/automations",
			handler: s.handleV2HAAutomations,
		},
		{
			name:    "HA scenes",
			method:  http.MethodGet,
			path:    "/api/v2/homeassistant/scenes",
			handler: s.handleV2HAScenes,
		},
		// Credentials
		{
			name:    "credentials read",
			method:  http.MethodGet,
			path:    "/api/v2/credentials/profiles",
			handler: s.handleV2Credentials,
		},
		{
			name:    "credential actions write",
			method:  http.MethodPut,
			path:    "/api/v2/credentials/profiles/abc",
			handler: s.handleV2CredentialActions,
		},
		// Terminal
		{
			name:    "terminal sessions read",
			method:  http.MethodGet,
			path:    "/api/v2/terminal/sessions",
			handler: s.handleV2TerminalSessions,
		},
		{
			name:    "terminal history",
			method:  http.MethodGet,
			path:    "/api/v2/terminal/history",
			handler: s.handleV2TerminalHistory,
		},
		{
			name:    "terminal snippets read",
			method:  http.MethodGet,
			path:    "/api/v2/terminal/snippets",
			handler: s.handleV2TerminalSnippets,
		},
		// Agents
		{
			name:    "agents list",
			method:  http.MethodGet,
			path:    "/api/v2/agents",
			handler: s.handleV2Agents,
		},
		{
			name:    "agents pending",
			method:  http.MethodGet,
			path:    "/api/v2/agents/pending",
			handler: s.handleV2AgentsPending,
		},
		{
			name:    "agents pending approve",
			method:  http.MethodPost,
			path:    "/api/v2/agents/pending/approve",
			handler: s.handleV2AgentsPendingApprove,
		},
		{
			name:    "agents pending reject",
			method:  http.MethodPost,
			path:    "/api/v2/agents/pending/reject",
			handler: s.handleV2AgentsPendingReject,
		},
		// Hub Status
		{
			name:    "hub status",
			method:  http.MethodGet,
			path:    "/api/v2/hub/status",
			handler: s.handleV2HubStatus,
		},
		{
			name:    "hub agents presence",
			method:  http.MethodGet,
			path:    "/api/v2/hub/agents",
			handler: s.handleV2HubAgents,
		},
		{
			name:    "hub tls read",
			method:  http.MethodGet,
			path:    "/api/v2/hub/tls",
			handler: s.handleV2HubTLS,
		},
		{
			name:    "hub tailscale",
			method:  http.MethodGet,
			path:    "/api/v2/hub/tailscale",
			handler: s.handleV2HubTailscale,
		},
		// Web Services
		{
			name:    "web services read",
			method:  http.MethodGet,
			path:    "/api/v2/web-services",
			handler: s.handleV2WebServices,
		},
		{
			name:    "web service sync",
			method:  http.MethodPost,
			path:    "/api/v2/web-services/sync",
			handler: s.handleV2WebServiceSync,
		},
		// Collectors
		{
			name:    "collectors read",
			method:  http.MethodGet,
			path:    "/api/v2/collectors",
			handler: s.handleV2Collectors,
		},
		// Notifications
		{
			name:    "notification channels",
			method:  http.MethodGet,
			path:    "/api/v2/notifications/channels",
			handler: s.handleV2NotificationChannels,
		},
		{
			name:    "notification history",
			method:  http.MethodGet,
			path:    "/api/v2/notifications/history",
			handler: s.handleV2NotificationHistory,
		},
		// Synthetic Checks
		{
			name:    "synthetic checks read",
			method:  http.MethodGet,
			path:    "/api/v2/synthetic-checks",
			handler: s.handleV2SyntheticChecks,
		},
		// Discovery
		{
			name:    "discovery run",
			method:  http.MethodPost,
			path:    "/api/v2/discovery/run",
			handler: s.handleV2DiscoveryRun,
		},
		{
			name:    "discovery proposals",
			method:  http.MethodGet,
			path:    "/api/v2/discovery/proposals",
			handler: s.handleV2DiscoveryProposals,
		},
		// Topology
		{
			name:    "dependencies read",
			method:  http.MethodGet,
			path:    "/api/v2/dependencies",
			handler: s.handleV2Dependencies,
		},
		{
			name:    "edges read",
			method:  http.MethodGet,
			path:    "/api/v2/edges",
			handler: s.handleV2Edges,
		},
		{
			name:    "composites read",
			method:  http.MethodGet,
			path:    "/api/v2/composites",
			handler: s.handleV2Composites,
		},
		// Failover
		{
			name:    "failover pairs read",
			method:  http.MethodGet,
			path:    "/api/v2/failover-pairs",
			handler: s.handleV2FailoverPairs,
		},
		// Dead Letters
		{
			name:    "dead letters read",
			method:  http.MethodGet,
			path:    "/api/v2/dead-letters",
			handler: s.handleV2DeadLetters,
		},
		// Audit
		{
			name:    "audit events",
			method:  http.MethodGet,
			path:    "/api/v2/audit/events",
			handler: s.handleV2AuditEvents,
		},
		// Log Views
		{
			name:    "log views read",
			method:  http.MethodGet,
			path:    "/api/v2/logs/views",
			handler: s.handleV2LogViews,
		},
		// Prometheus Settings
		{
			name:    "prometheus settings read",
			method:  http.MethodGet,
			path:    "/api/v2/settings/prometheus",
			handler: s.handleV2PrometheusSettings,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testV2ScopeDenied(t, s, tt.method, tt.path, tt.handler)
		})
	}
}

// TestHandleV2HAEntities_ScopeWriteDenied verifies that a POST with only read
// scope is correctly rejected.
func TestHandleV2HAEntities_ScopeWriteDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/homeassistant/entities", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"homeassistant:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2HAEntities(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for write without write scope, got %d", rec.Code)
	}
}

// TestHandleV2HAEntities_ScopeAllowed verifies that a GET with read scope
// passes the scope gate (returns 501 because HA is not implemented, not 403).
func TestHandleV2HAEntities_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/homeassistant/entities", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"homeassistant:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2HAEntities(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed for homeassistant:read GET")
	}
}

// TestHandleV2HubStatus_ScopeAllowed verifies the hub status endpoint returns
// 200 when hub:read scope is present.
func TestHandleV2HubStatus_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/hub/status", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"hub:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2HubStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// TestHandleV2Credentials_ScopeAllowed verifies that credentials:read scope
// passes the scope gate on GET.
func TestHandleV2Credentials_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/credentials/profiles", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"credentials:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2Credentials(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed for credentials:read GET")
	}
}

// TestHandleV2DeadLetters_WriteScope verifies that a DELETE without write
// scope is rejected.
func TestHandleV2DeadLetters_WriteScope(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/dead-letters", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"dead-letters:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2DeadLetters(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for DELETE without write scope, got %d", rec.Code)
	}
}

// TestHandleV2PrometheusSettings_WriteScope verifies that a PATCH without
// write scope is rejected.
func TestHandleV2PrometheusSettings_WriteScope(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v2/settings/prometheus", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"settings:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2PrometheusSettings(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for PATCH without settings:write scope, got %d", rec.Code)
	}
}
