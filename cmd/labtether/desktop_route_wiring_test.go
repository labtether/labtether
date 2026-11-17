package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
)

// TestDesktopRoutesReachHandlersThroughRouteTable sends one signed-in admin
// request to each desktop route through the real route table and auth, and
// checks the answer only that route's handler gives. The device's agent is a
// test WebSocket that answers diagnose and clipboard requests through cmd's
// own reply handlers.
//
// The desktop handler tests live in internal/hubapi/desktop and build their
// Deps with testutil/hubdeps. This test catches a fault in cmd's own wiring
// that those tests cannot see: a route bound to the wrong handler, or
// buildDesktopDeps leaving out the agent manager, the reply waiters the
// agent router delivers to, the session or asset store, or the stream
// tickets.
func TestDesktopRoutesReachHandlersThroughRouteTable(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1", Type: "host", Name: "node-1", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	serverConn, agentConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")
	go func() {
		from := &agentmgr.AgentConn{AssetID: "node-1"}
		for {
			var msg agentmgr.Message
			if agentConn.ReadJSON(&msg) != nil {
				return
			}
			switch msg.Type {
			case agentmgr.MsgDesktopDiagnose:
				raw, _ := json.Marshal(agentmgr.DesktopDiagnosticData{RequestID: msg.ID, XvfbRunning: true})
				sut.ensureDesktopDeps().ProcessAgentDesktopDiagnosed(from, agentmgr.Message{Type: agentmgr.MsgDesktopDiagnosed, Data: raw})
			case agentmgr.MsgClipboardGet:
				raw, _ := json.Marshal(agentmgr.ClipboardDataPayload{RequestID: msg.ID, Format: "text", Text: "from agent"})
				sut.ensureDesktopDeps().ProcessAgentClipboardData(from, agentmgr.Message{Type: agentmgr.MsgClipboardData, Data: raw})
			}
		}
	}()
	admin := createRoleSessionToken(t, sut, "admin-desktop-wiring", auth.RoleAdmin)
	call := routeWiringCaller(t, sut.buildHTTPHandlers(nil, nil, nil), admin)

	session := call("/desktop/sessions", http.MethodPost, "/desktop/sessions", `{"target":"node-1"}`, http.StatusCreated, `"mode":"desktop"`)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(session), &created); err != nil || created.ID == "" {
		t.Fatalf("no desktop session id in %s (err %v)", session, err)
	}
	call("/desktop/sessions/", http.MethodPost, "/desktop/sessions/"+created.ID+"/stream-ticket", "", http.StatusCreated, `"stream_path"`)
	call("/desktop/diagnose/", http.MethodPost, "/desktop/diagnose/node-1", "", http.StatusOK, `"xvfb_running":true`)
	call("/recordings", http.MethodPost, "/recordings", `{"session_id":"`+created.ID+`"}`, http.StatusNotFound, "desktop session bridge not found")
	call("/recordings/", http.MethodPost, "/recordings/"+created.ID, "", http.StatusNotFound, "no active recording for session")
	call("/api/v1/nodes/", http.MethodPost, "/api/v1/nodes/node-1/clipboard/get", `{"format":"text"}`, http.StatusOK, `"text":"from agent"`)
}

// routeWiringCaller returns a function that sends one request with the
// session cookie to route's handler in handlers and checks the status and a
// piece of the body. It returns the body.
func routeWiringCaller(t *testing.T, handlers map[string]http.HandlerFunc, sessionToken string) func(route, method, path, body string, wantCode int, wantBody string) string {
	return func(route, method, path, body string, wantCode int, wantBody string) string {
		t.Helper()
		handler := handlers[route]
		if handler == nil {
			t.Fatalf("route %s is not registered", route)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionToken})
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != wantCode || !strings.Contains(rec.Body.String(), wantBody) {
			t.Fatalf("%s %s (route %s): status=%d body=%s, want %d with %s", method, path, route, rec.Code, rec.Body.String(), wantCode, wantBody)
		}
		return rec.Body.String()
	}
}
