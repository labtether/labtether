package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/server"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/runtimesettings"
)

func TestLegacyRoutesEnforceAPIKeyScopes(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct {
		name       string
		handlerKey string
		method     string
		path       string
		body       string
	}{
		{
			name:       "groups missing scope",
			handlerKey: "/groups",
			method:     http.MethodGet,
			path:       "/groups",
		},
		{
			name:       "connectors missing scope",
			handlerKey: "/connectors",
			method:     http.MethodGet,
			path:       "/connectors",
		},
		{
			name:       "legacy action execute missing scope",
			handlerKey: "/actions/execute",
			method:     http.MethodPost,
			path:       "/actions/execute",
			body:       `{"target":"srv1","command":"uptime"}`,
		},
		{
			name:       "status aggregate missing hub scope",
			handlerKey: "/status/aggregate",
			method:     http.MethodGet,
			path:       "/status/aggregate",
		},
		{
			name:       "legacy discovery run missing write scope",
			handlerKey: "/discovery/run",
			method:     http.MethodPost,
			path:       "/discovery/run",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], tc.method, tc.path, key, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLegacyPackageRoutesUseExactReadWriteScopesAndAssetBinding(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	readKey := createLegacyRouteAPIKey(t, sut, []string{"packages:read"}, []string{"srv1"})
	writeKey := createLegacyRouteAPIKey(t, sut, []string{"packages:write"}, []string{"srv1"})

	if rec := invokeLegacyRoute(t, handlers["/packages/"], http.MethodGet, "/packages/srv1/upgradable", readKey, ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("read scope should reach upgradable handler, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := invokeLegacyRoute(t, handlers["/packages/"], http.MethodPost, "/packages/srv1/update", readKey, `{"packages":["curl"]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("read scope performed package mutation, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := invokeLegacyRoute(t, handlers["/packages/"], http.MethodPost, "/packages/srv1/update", writeKey, `{"packages":["curl"]}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("write scope should reach update handler, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := invokeLegacyRoute(t, handlers["/packages/"], http.MethodGet, "/packages/srv1/upgradable", writeKey, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("write-only scope read package inventory, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := invokeLegacyRoute(t, handlers["/packages/"], http.MethodGet, "/packages/srv2/upgradable", readKey, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("asset-restricted key read srv2 inventory, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLegacyAssetRoutesEnforceAllowedAssets(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, id := range []string{"srv1", "srv2"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id,
			Name:    strings.ToUpper(id),
			Source:  "agent",
			Type:    "host",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed asset %s: %v", id, err)
		}
	}

	listRec := invokeLegacyRoute(t, handlers["/assets"], http.MethodGet, "/assets", key, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing allowed assets, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp struct {
		Assets []assets.Asset `json:"assets"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode assets response: %v", err)
	}
	if len(listResp.Assets) != 1 || listResp.Assets[0].ID != "srv1" {
		t.Fatalf("expected only srv1 in legacy asset list, got %#v", listResp.Assets)
	}

	deniedRec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodGet, "/assets/srv2", key, "")
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed asset, got %d: %s", deniedRec.Code, deniedRec.Body.String())
	}

	allowedRec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodGet, "/assets/srv1", key, "")
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed asset, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}
}

func TestRemoteSessionRoutesEnforceAllowedAssets(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:read", "terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, id := range []string{"srv1", "srv2"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id,
			Name:    strings.ToUpper(id),
			Source:  "agent",
			Type:    "host",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed asset %s: %v", id, err)
		}
	}

	terminalRec := invokeLegacyRoute(t, handlers["/terminal/sessions"], http.MethodPost, "/terminal/sessions", key, `{"target":"srv2","mode":"interactive"}`)
	if terminalRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed terminal target, got %d: %s", terminalRec.Code, terminalRec.Body.String())
	}

	desktopRec := invokeLegacyRoute(t, handlers["/desktop/sessions"], http.MethodPost, "/desktop/sessions", key, `{"target":"srv2"}`)
	if desktopRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed desktop target, got %d: %s", desktopRec.Code, desktopRec.Body.String())
	}
}

func TestRemoteSessionRoutesRequireCredentialUseForStoredProfiles(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:read", "terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1",
		Name:    "SRV1",
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.credentialStore.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "srv1",
		Host:                "192.0.2.10",
		Port:                22,
		CredentialProfileID: "cred-private",
	}); err != nil {
		t.Fatalf("seed terminal credential binding: %v", err)
	}

	terminalRec := invokeLegacyRoute(t, handlers["/terminal/sessions"], http.MethodPost, "/terminal/sessions", key, `{"target":"srv1","mode":"interactive"}`)
	if terminalRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without credentials:use for terminal, got %d: %s", terminalRec.Code, terminalRec.Body.String())
	}

	desktopRec := invokeLegacyRoute(t, handlers["/desktop/sessions"], http.MethodPost, "/desktop/sessions", key, `{"target":"srv1"}`)
	if desktopRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without credentials:use for desktop, got %d: %s", desktopRec.Code, desktopRec.Body.String())
	}
}

func TestTerminalConfigPatchCannotRetainCredentialWithoutUseScope(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"terminal:write"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1",
		Name:    "SRV1",
		Source:  "agent",
		Type:    "host",
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.credentialStore.SaveAssetTerminalConfig(credentials.AssetTerminalConfig{
		AssetID:             "srv1",
		Host:                "192.0.2.10",
		Port:                22,
		CredentialProfileID: "cred-private",
	}); err != nil {
		t.Fatalf("seed terminal credential binding: %v", err)
	}

	rec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPatch, "/assets/srv1/terminal/config", key, `{"host":"192.0.2.11"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when retained binding lacks credentials:use, got %d: %s", rec.Code, rec.Body.String())
	}
	cfg, ok, err := sut.credentialStore.GetAssetTerminalConfig("srv1")
	if err != nil || !ok {
		t.Fatalf("reload terminal config: ok=%v err=%v", ok, err)
	}
	if cfg.Host != "192.0.2.10" {
		t.Fatalf("forbidden patch mutated stored config host to %q", cfg.Host)
	}
}

func TestDesktopCredentialRetrieveRequiresUseScope(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "srv1", Name: "SRV1", Source: "agent", Type: "host", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	writeKey := createLegacyRouteAPIKey(t, sut, []string{"credentials:write"}, []string{"srv1"})
	rec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodPost, "/assets/srv1/desktop/credentials/retrieve", writeKey, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when credentials:write lacks credentials:use, got %d: %s", rec.Code, rec.Body.String())
	}

	useKey := createLegacyRouteAPIKey(t, sut, []string{"credentials:use"}, []string{"srv1"})
	rec = invokeLegacyRoute(t, handlers["/assets/"], http.MethodPost, "/assets/srv1/desktop/credentials/retrieve", useKey, "")
	if rec.Code == http.StatusForbidden {
		t.Fatalf("credentials:use should pass the scope boundary, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogsQueryFiltersAPIKeyAssetAllowlist(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"logs:read"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	now := time.Now().UTC()
	for _, event := range []logs.Event{
		{ID: "allowed", AssetID: "srv1", Source: "agent", Message: "allowed", Timestamp: now},
		{ID: "denied", AssetID: "srv2", Source: "agent", Message: "denied", Timestamp: now},
	} {
		if err := sut.logStore.AppendEvent(event); err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	rec := invokeLegacyRoute(t, handlers["/logs/query"], http.MethodGet, "/logs/query?window=1h&limit=20", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Events []logs.Event `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode logs response: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0].AssetID != "srv1" {
		t.Fatalf("expected only authorized asset logs, got %#v", payload.Events)
	}
}

func TestPrometheusMetricsRequiresMetricsRead(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusScrapeEnabled: "true",
	}); err != nil {
		t.Fatalf("enable prometheus scrape: %v", err)
	}
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	unauthReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	unauthReq.TLS = &tls.ConnectionState{}
	unauthRec := httptest.NewRecorder()
	handlers["/metrics"](unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without credentials, got %d: %s", unauthRec.Code, unauthRec.Body.String())
	}

	wrongKey := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, nil)
	wrongRec := invokeLegacyRoute(t, handlers["/metrics"], http.MethodGet, "/metrics", wrongKey, "")
	if wrongRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without metrics:read, got %d: %s", wrongRec.Code, wrongRec.Body.String())
	}

	metricsKey := createLegacyRouteAPIKey(t, sut, []string{"metrics:read"}, nil)
	allowedRec := invokeLegacyRoute(t, handlers["/metrics"], http.MethodGet, "/metrics", metricsKey, "")
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with metrics:read, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}

	restrictedKey := createLegacyRouteAPIKey(t, sut, []string{"metrics:read"}, []string{"srv1"})
	restrictedRec := invokeLegacyRoute(t, handlers["/metrics"], http.MethodGet, "/metrics", restrictedKey, "")
	if restrictedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an asset-restricted metrics key, got %d: %s", restrictedRec.Code, restrictedRec.Body.String())
	}
}

func TestAssetRestrictedKeysCannotAccessGlobalRoutes(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	tests := []struct {
		name       string
		handlerKey string
		path       string
		scopes     []string
	}{
		{name: "tailscale serve", handlerKey: "/settings/tailscale/serve", path: "/settings/tailscale/serve", scopes: []string{"hub:read", "hub:admin"}},
		{name: "worker stats", handlerKey: "/worker/stats", path: "/worker/stats", scopes: []string{"hub:read"}},
		{name: "topology", handlerKey: "/api/v2/topology", path: "/api/v2/topology", scopes: []string{"topology:read"}},
		{name: "topology zones", handlerKey: "/api/v2/topology/zones", path: "/api/v2/topology/zones", scopes: []string{"topology:read"}},
		{name: "topology zone actions", handlerKey: "/api/v2/topology/zones/", path: "/api/v2/topology/zones/zone-1", scopes: []string{"topology:write"}},
		{name: "topology connections", handlerKey: "/api/v2/topology/connections", path: "/api/v2/topology/connections", scopes: []string{"topology:read"}},
		{name: "topology connection actions", handlerKey: "/api/v2/topology/connections/", path: "/api/v2/topology/connections/connection-1", scopes: []string{"topology:write"}},
		{name: "topology viewport", handlerKey: "/api/v2/topology/viewport", path: "/api/v2/topology/viewport", scopes: []string{"topology:read"}},
		{name: "topology unsorted", handlerKey: "/api/v2/topology/unsorted", path: "/api/v2/topology/unsorted", scopes: []string{"topology:read"}},
		{name: "topology auto place", handlerKey: "/api/v2/topology/auto-place", path: "/api/v2/topology/auto-place", scopes: []string{"topology:write"}},
		{name: "topology reset", handlerKey: "/api/v2/topology/reset", path: "/api/v2/topology/reset", scopes: []string{"topology:write"}},
		{name: "topology dismiss", handlerKey: "/api/v2/topology/dismiss", path: "/api/v2/topology/dismiss", scopes: []string{"topology:write"}},
		{name: "topology undismiss", handlerKey: "/api/v2/topology/dismiss/", path: "/api/v2/topology/dismiss/srv1", scopes: []string{"topology:write"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := createLegacyRouteAPIKey(t, sut, tc.scopes, []string{"srv1"})
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], http.MethodGet, tc.path, key, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for asset-restricted key, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAssetRestrictedOperatorKeyCanReachMCPScopedToolSurface(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	key := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, []string{"srv1"})
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"scope-test","version":"1"}}}`

	rec := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset-restricted operator key could not initialize MCP: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"serverInfo"`) {
		t.Fatalf("MCP initialize response omitted server information: %s", rec.Body.String())
	}
}

func TestAssetRestrictedViewerKeyCanReadMCPButCannotMutateOrReadGlobalData(t *testing.T) {
	sut := newTestAPIServer(t)
	for _, heartbeat := range []assets.HeartbeatRequest{
		{AssetID: "srv1", Name: "allowed-mcp-asset", Status: "online", Platform: "linux"},
		{AssetID: "hidden", Name: "hidden-mcp-asset-sentinel", Status: "online", Platform: "linux"},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(heartbeat); err != nil {
			t.Fatal(err)
		}
	}
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleViewer, []string{"*"}, []string{"srv1"})

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"viewer-test","version":"1"}}}`
	initialized := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, initialize)
	if initialized.Code != http.StatusOK || !strings.Contains(initialized.Body.String(), `"serverInfo"`) {
		t.Fatalf("viewer could not initialize MCP: status=%d body=%s", initialized.Code, initialized.Body.String())
	}
	sessionID := initialized.Header().Get(mcptransport.HeaderKeySessionID)
	if sessionID == "" {
		t.Fatal("MCP initialize response omitted the session ID")
	}
	mcpHeaders := http.Header{mcptransport.HeaderKeySessionID: []string{sessionID}}

	initializedNotification := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	initializedResult := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, initializedNotification, mcpHeaders)
	if initializedResult.Code != http.StatusAccepted {
		t.Fatalf("viewer could not complete MCP initialization: status=%d body=%s", initializedResult.Code, initializedResult.Body.String())
	}

	assetsCall := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"assets_list","arguments":{}}}`
	assetsResult := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, assetsCall, mcpHeaders)
	if assetsResult.Code != http.StatusOK {
		t.Fatalf("viewer assets_list status=%d body=%s", assetsResult.Code, assetsResult.Body.String())
	}
	if !strings.Contains(assetsResult.Body.String(), "allowed-mcp-asset") || strings.Contains(assetsResult.Body.String(), "hidden-mcp-asset-sentinel") {
		t.Fatalf("viewer assets_list leaked or omitted an asset: %s", assetsResult.Body.String())
	}

	resourceCall := `{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"labtether://assets"}}`
	resourceResult := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, resourceCall, mcpHeaders)
	if resourceResult.Code != http.StatusOK || !strings.Contains(resourceResult.Body.String(), "allowed-mcp-asset") || strings.Contains(resourceResult.Body.String(), "hidden-mcp-asset-sentinel") {
		t.Fatalf("viewer assets resource leaked, omitted, or failed: status=%d body=%s", resourceResult.Code, resourceResult.Body.String())
	}

	globalCall := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"webhooks_list","arguments":{}}}`
	globalResult := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, globalCall, mcpHeaders)
	if globalResult.Code != http.StatusOK || !strings.Contains(globalResult.Body.String(), "asset-restricted") {
		t.Fatalf("restricted viewer reached global MCP data: status=%d body=%s", globalResult.Code, globalResult.Body.String())
	}

	wakeCall := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"asset_wake","arguments":{"asset_id":"srv1"}}}`
	wakeResult := invokeLegacyRoute(t, handlers["/mcp"], http.MethodPost, "/mcp", key, wakeCall, mcpHeaders)
	if wakeResult.Code != http.StatusOK || !strings.Contains(wakeResult.Body.String(), "operator role") || !strings.Contains(wakeResult.Body.String(), `"isError":true`) {
		t.Fatalf("viewer MCP mutation was not denied by role: status=%d body=%s", wakeResult.Code, wakeResult.Body.String())
	}
	events, err := sut.auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "asset.wake.requested" {
			t.Fatalf("viewer mutation reached Wake-on-LAN dispatch: %#v", event)
		}
	}
}

func TestTelemetryIngestRequiresLogsWrite(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "frontend", path: "/telemetry/frontend/perf", body: `{"route":"dashboard","metric":"render","duration_ms":1}`},
		{name: "mobile", path: "/telemetry/mobile/client", body: `{"route":"api.logs","metric":"request.duration","duration_ms":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrongKey := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, nil)
			denied := invokeLegacyRoute(t, handlers[tc.path], http.MethodPost, tc.path, wrongKey, tc.body)
			if denied.Code != http.StatusForbidden {
				t.Fatalf("expected 403 without logs:write, got %d: %s", denied.Code, denied.Body.String())
			}

			writeKey := createLegacyRouteAPIKey(t, sut, []string{"logs:write"}, nil)
			allowed := invokeLegacyRoute(t, handlers[tc.path], http.MethodPost, tc.path, writeKey, tc.body)
			if allowed.Code != http.StatusAccepted {
				t.Fatalf("expected 202 with logs:write, got %d: %s", allowed.Code, allowed.Body.String())
			}
		})
	}
}

func TestAssetRestrictedAPIKeyCannotSubscribeToGlobalEvents(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"events:subscribe"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	rec := invokeLegacyRoute(t, handlers["/ws/events/ticket"], http.MethodPost, "/ws/events/ticket", key, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for asset-restricted event subscription, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sut.streamTicketStore.Tickets) != 0 {
		t.Fatalf("restricted API key received a stream ticket: %#v", sut.streamTicketStore.Tickets)
	}
}

func TestViewerCanMintReadOnlyEventSubscriptionTicket(t *testing.T) {
	sut := newTestAPIServer(t)
	viewer, err := sut.authStore.CreateUserWithRole("viewer-events", "unused", auth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	rawToken := "viewer-events-session"
	if _, err := sut.authStore.CreateAuthSession(viewer.ID, auth.HashToken(rawToken), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create viewer session: %v", err)
	}
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/ws/events/ticket", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: rawToken})
	rec := httptest.NewRecorder()
	handlers["/ws/events/ticket"](rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer event ticket status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(sut.streamTicketStore.Tickets) != 1 {
		t.Fatalf("viewer ticket count = %d, want 1", len(sut.streamTicketStore.Tickets))
	}
}

func TestViewerAPIKeyCanMintScopedReadOnlyEventSubscriptionTicket(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleViewer, []string{"events:subscribe"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	rec := invokeLegacyRoute(t, handlers["/ws/events/ticket"], http.MethodPost, "/ws/events/ticket", key, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer API key event ticket status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(sut.streamTicketStore.Tickets) != 1 {
		t.Fatalf("viewer API key ticket count = %d, want 1", len(sut.streamTicketStore.Tickets))
	}
}

func TestConsumeEventTicketRejectsRestrictedLegacyTicket(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.streamTicketStore.Tickets = map[string]streamTicket{
		"restricted": {
			SessionID:     "__browser_events__",
			APIKeyID:      "key-restricted",
			AllowedAssets: []string{"srv1"},
			ExpiresAt:     time.Now().UTC().Add(time.Minute),
		},
	}
	if sut.consumeEventTicket("restricted") {
		t.Fatal("restricted legacy ticket authenticated to the global event stream")
	}
	if _, ok := sut.streamTicketStore.Tickets["restricted"]; ok {
		t.Fatal("rejected restricted ticket was not consumed")
	}
}

func TestAdminRoutesEnforceAPIKeyScopes(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"assets:read"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct {
		name       string
		handlerKey string
		path       string
	}{
		{
			name:       "api key management",
			handlerKey: "/api/v2/keys",
			path:       "/api/v2/keys",
		},
		{
			name:       "runtime settings",
			handlerKey: "/settings/runtime",
			path:       "/settings/runtime",
		},
		{
			name:       "auth users",
			handlerKey: "/auth/users",
			path:       "/auth/users",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], http.MethodGet, tc.path, key, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestManagedDatabaseRevealRequiresCredentialUseScope(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)
	readOnlyKey := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"settings:read"}, nil)
	rec := invokeLegacyRoute(t, handlers["/settings/managed-database/reveal"], http.MethodPost, "/settings/managed-database/reveal", readOnlyKey, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for settings:read without credentials:use, got %d: %s", rec.Code, rec.Body.String())
	}
}

func createLegacyRouteAPIKey(t *testing.T, sut *apiServer, scopes []string, allowedAssets []string) string {
	t.Helper()
	return createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleOperator, scopes, allowedAssets)
}

func createLegacyRouteAPIKeyWithRole(t *testing.T, sut *apiServer, role string, scopes []string, allowedAssets []string) string {
	t.Helper()
	generated, err := apikeys.GenerateKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	key := apikeys.APIKey{
		ID:            "key_" + generated.Prefix,
		Name:          "legacy route test",
		Prefix:        generated.Prefix,
		SecretHash:    generated.Hash,
		Role:          role,
		Scopes:        scopes,
		AllowedAssets: allowedAssets,
		CreatedBy:     "owner",
		CreatedAt:     time.Now().UTC(),
	}
	if err := sut.apiKeyStore.CreateAPIKey(context.Background(), key); err != nil {
		t.Fatalf("store api key: %v", err)
	}
	return generated.Raw
}

func invokeLegacyRoute(
	t *testing.T,
	handler http.HandlerFunc,
	method string,
	path string,
	key string,
	body string,
	requestHeaders ...http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	if handler == nil {
		t.Fatalf("missing handler for %s", path)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+key)
	for _, headers := range requestHeaders {
		for name, values := range headers {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}
