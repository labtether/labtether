package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubcollector"
)

// TestTrueNASRoutesReachUpstreamThroughRouteTable sends one signed-in admin
// request to the /truenas/ route through the real route table and auth, and
// checks that it reaches the configured TrueNAS API, logged in with the
// stored API key.
//
// The TrueNAS handler tests live in internal/hubapi/truenas and build their
// Deps with testutil/hubdeps. This test catches a fault in cmd's own wiring
// that those tests cannot see: the route bound to the wrong handler, or
// buildTruenasDeps (or ensureTruenasDeps) leaving out the collector store,
// credential store, secrets manager or asset store. The pools read answers
// 200 with a warning when the upstream call fails, so the test checks the
// call itself.
func TestTrueNASRoutesReachUpstreamThroughRouteTable(t *testing.T) {
	const apiKey = "wiring-api-key"
	var mu sync.Mutex
	var methods []string
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var req struct {
				ID     uint64          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if conn.ReadJSON(&req) != nil {
				return
			}
			var result any = []any{}
			if req.Method == "auth.login_with_api_key" {
				var params []string
				_ = json.Unmarshal(req.Params, &params)
				result = len(params) == 1 && params[0] == apiKey
			} else {
				mu.Lock()
				methods = append(methods, req.Method)
				mu.Unlock()
			}
			if conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}) != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-wiring", apiKey, upstream.URL)
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID: "collector-wiring", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true,
		Config: map[string]any{"base_url": upstream.URL, "credential_id": "cred-wiring"},
	})
	seedTrueNASAsset(t, sut, "truenas-host-wiring", "collector-wiring")
	admin := createRoleSessionToken(t, sut, "admin-truenas-wiring", auth.RoleAdmin)
	handler := sut.buildHTTPHandlers(nil, nil, nil)["/truenas/assets/"]
	if handler == nil {
		t.Fatal("route /truenas/assets/ is not registered")
	}

	req := httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-wiring/pools", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: admin})
	rec := httptest.NewRecorder()
	handler(rec, req)
	mu.Lock()
	defer mu.Unlock()
	var body struct {
		Pools    []any    `json:"pools"`
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.Pools == nil || len(body.Warnings) != 0 {
		t.Fatalf("GET pools status=%d body=%s upstream=%v", rec.Code, rec.Body.String(), methods)
	}
	if len(methods) == 0 || methods[0] != "pool.query" {
		t.Fatalf("GET pools reached upstream methods %v, want pool.query", methods)
	}
}
