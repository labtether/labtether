package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/policy"
)

// The collectors handler tests live in internal/hubapi/collectors and build
// their Deps with testutil/hubdeps. These two tests catch a fault in cmd's
// own wiring that those tests cannot see: one of the 16 collectors routes
// bound to the wrong handler, or buildCollectorsDeps leaving out a store,
// the secrets manager, the connector registry, the agent manager, the web
// service coordinator, or cmd's Settings switch for integration actions
// (the package tests have a stand-in for it). Each request is signed in as an
// admin, goes through the real route table and auth, and is checked for an
// answer only its handler gives with that field wired.

// TestCollectorsConnectorRoutesReachHandlersThroughRouteTable covers the
// four collector and connector routes.
func TestCollectorsConnectorRoutesReachHandlersThroughRouteTable(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	const token = "wiring-ha-token"
	var mu sync.Mutex
	var sawAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		sawAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer upstream.Close()

	sut := newTestAPIServer(t)
	sut.hubCollectorStore = &stubHubCollectorStore{collectors: []hubcollector.Collector{{
		ID: "collector-wiring", CollectorType: hubcollector.CollectorTypeHomeAssistant, Enabled: true,
		Config: map[string]any{"base_url": upstream.URL, "credential_id": "cred-wiring"},
	}}}
	sut.connectorRegistry.Register(homeassistant.New())
	createHomeAssistantCredentialProfile(t, sut, "cred-wiring", token, upstream.URL)
	admin := createRoleSessionToken(t, sut, "admin-collectors-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/hub-collectors", http.MethodGet, "/hub-collectors", "", http.StatusOK, `"collectors":[{"id":"collector-wiring"`)
	call("/hub-collectors/", http.MethodGet, "/hub-collectors/collector-wiring", "", http.StatusOK, `"collector":{"id":"collector-wiring"`)
	call("/connectors", http.MethodGet, "/connectors", "", http.StatusOK, `"id":"home-assistant"`)
	// Test connection with a saved credential: the stored token, decrypted.
	call("/connectors/", http.MethodPost, "/connectors/home-assistant/test", `{"credential_id":"cred-wiring"}`,
		http.StatusOK, "home assistant API reachable")
	mu.Lock()
	got := sawAuth
	mu.Unlock()
	if got != "Bearer "+token {
		t.Fatalf("Home Assistant saw Authorization %q, want the stored token", got)
	}

	// Settings › Integration actions off: the connector route refuses a write.
	cfg := policy.DefaultEvaluatorConfig()
	cfg.ConnectorEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)
	call("/connectors/", http.MethodPost, "/connectors/home-assistant/discover", "", http.StatusForbidden,
		"Integration actions are turned off in Settings.")
}

// TestCollectorsWebServiceRoutesReachHandlersThroughRouteTable covers the 12
// web service routes. The test Hub has no database, so the four routes that
// use it get a request their handler refuses before it reads the database.
func TestCollectorsWebServiceRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	peer := testutil.NewAgentPeer(t, "node-wiring")
	sut.agentMgr = peer.Manager
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-wiring", Type: "host", Name: "node-wiring", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	report, err := json.Marshal(agentmgr.WebServiceReportData{HostAssetID: "node-wiring", Services: []agentmgr.DiscoveredWebService{{
		ID: "svc-wiring", Name: "Wiring Home Assistant", Category: "Wiring", URL: "http://node-wiring:8123",
		Status: "up", Source: "docker", HostAssetID: "node-wiring",
		Metadata: map[string]string{"compat_connector": "homeassistant", "compat_confidence": "0.95"},
	}}})
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	sut.webServiceCoordinator.HandleReport("node-wiring", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: report})
	icon := `[{"id":"icon-wiring","name":"Wiring icon","data_url":"data:image/png;base64,iVBORw0KGgo="}]`
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{"services.custom_icon_library": icon}); err != nil {
		t.Fatalf("save icon library: %v", err)
	}
	admin := createRoleSessionToken(t, sut, "admin-webservice-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	call("/api/v1/services/web", http.MethodGet, "/api/v1/services/web", "", http.StatusOK, `"id":"svc-wiring"`)
	call("/api/v1/services/web/compat", http.MethodGet, "/api/v1/services/web/compat", "", http.StatusOK, `"service_id":"svc-wiring"`)
	call("/api/v1/services/web/categories", http.MethodGet, "/api/v1/services/web/categories", "", http.StatusOK, `"categories":["Wiring"]`)
	call("/api/v1/services/web/sync", http.MethodPost, "/api/v1/services/web/sync?host=node-wiring", "", http.StatusAccepted,
		`"sent_to":["node-wiring"]`)
	if msg := peer.Next(t); msg.Type != agentmgr.MsgWebServiceSync {
		t.Fatalf("agent got %q, want %q", msg.Type, agentmgr.MsgWebServiceSync)
	}
	// A device the asset store does not have is refused; a known one is saved.
	call("/api/v1/services/web/manual", http.MethodPost, "/api/v1/services/web/manual",
		`{"host_asset_id":"node-missing","name":"Wiring app","url":"http://node-wiring:9000"}`, http.StatusBadRequest, "host asset does not exist")
	created := call("/api/v1/services/web/manual", http.MethodPost, "/api/v1/services/web/manual",
		`{"host_asset_id":"node-wiring","name":"Wiring app","url":"http://node-wiring:9000"}`, http.StatusCreated, `"name":"Wiring app"`)
	var manual struct{ Service struct{ ID string } }
	if err := json.Unmarshal([]byte(created), &manual); err != nil || manual.Service.ID == "" {
		t.Fatalf("no manual service ID in %s (err %v)", created, err)
	}
	call("/api/v1/services/web/manual/", http.MethodDelete, "/api/v1/services/web/manual/"+manual.Service.ID, "", http.StatusNoContent, "")
	call("/api/v1/services/web/overrides", http.MethodGet, "/api/v1/services/web/overrides", "", http.StatusOK, `"overrides":`)
	call("/api/v1/services/web/icon-library", http.MethodGet, "/api/v1/services/web/icon-library", "", http.StatusOK, `"id":"icon-wiring"`)
	call("/api/v1/services/web/alt-urls/", http.MethodGet, "/api/v1/services/web/alt-urls/", "", http.StatusBadRequest,
		"web_service_id query parameter is required")
	call("/api/v1/services/web/never-group-rules", http.MethodPost, "/api/v1/services/web/never-group-rules", `{}`, http.StatusBadRequest,
		"url_a and url_b are required")
	call("/api/v1/services/web/grouping-settings", http.MethodPatch, "/api/v1/services/web/grouping-settings", `{"values":{}}`,
		http.StatusBadRequest, "values map is required and must not be empty")
	call("/api/v1/services/web/grouping-suggestions/", http.MethodPost, "/api/v1/services/web/grouping-suggestions/s1/maybe", "",
		http.StatusBadRequest, "action must be 'accept' or 'deny'")
}
