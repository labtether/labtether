package truenas

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func allowInsecureTransportForTrueNASTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

type fakeWSConn struct {
	setWriteDeadlineFn func(time.Time) error
	setReadDeadlineFn  func(time.Time) error
	writeJSONFn        func(any) error
	readJSONFn         func(any) error
	closeFn            func() error
}

type testIdentifierStringer string

func (s testIdentifierStringer) String() string { return string(s) }

func (f *fakeWSConn) SetWriteDeadline(t time.Time) error {
	if f.setWriteDeadlineFn != nil {
		return f.setWriteDeadlineFn(t)
	}
	return nil
}

func (f *fakeWSConn) SetReadDeadline(t time.Time) error {
	if f.setReadDeadlineFn != nil {
		return f.setReadDeadlineFn(t)
	}
	return nil
}

func (f *fakeWSConn) WriteJSON(v any) error {
	if f.writeJSONFn != nil {
		return f.writeJSONFn(v)
	}
	return nil
}

func (f *fakeWSConn) ReadJSON(v any) error {
	if f.readJSONFn != nil {
		return f.readJSONFn(v)
	}
	return io.EOF
}

func (f *fakeWSConn) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func stubDialWSForTest(t *testing.T, fn func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error)) {
	t.Helper()
	previous := dialWS
	dialWS = fn
	t.Cleanup(func() {
		dialWS = previous
	})
}

// wsUpgrader is shared across all test helpers.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// rpcCall is the decoded form of a request arriving at the test server.
type rpcCall struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// writeRPCResult writes a successful JSON-RPC 2.0 response with the given result.
func writeRPCResult(conn *websocket.Conn, id uint64, result any) error {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	return conn.WriteJSON(resp)
}

// writeRPCError writes a JSON-RPC 2.0 error response.
func writeRPCError(conn *websocket.Conn, id uint64, code int, message string) error {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	return conn.WriteJSON(resp)
}

// mockTrueNASServer creates an httptest.Server that upgrades connections to
// WebSocket and invokes handler for each pair of messages (auth + method call).
// handler receives the method call rpcCall after auth succeeds and must write
// the appropriate response.
func mockTrueNASServer(t *testing.T, handler func(conn *websocket.Conn, call rpcCall)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("ws upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Read auth request.
		var authCall rpcCall
		if err := conn.ReadJSON(&authCall); err != nil {
			t.Errorf("read auth call: %v", err)
			return
		}
		if authCall.Method != "auth.login_with_api_key" {
			t.Errorf("expected auth.login_with_api_key, got %s", authCall.Method)
			if writeErr := writeRPCError(conn, authCall.ID, -32600, "expected auth first"); writeErr != nil {
				t.Logf("write auth error response: %v", writeErr)
			}
			return
		}

		// Respond to auth with success.
		if err := writeRPCResult(conn, authCall.ID, true); err != nil {
			t.Errorf("write auth result: %v", err)
			return
		}

		// Read the actual method call and delegate to the handler.
		var methodCall rpcCall
		if err := conn.ReadJSON(&methodCall); err != nil {
			t.Errorf("read method call: %v", err)
			return
		}
		handler(conn, methodCall)
	}))
	return srv
}

// mockTrueNASServerAuthFail creates a test server that rejects authentication.
func mockTrueNASServerAuthFail(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("ws upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		var authCall rpcCall
		if err := conn.ReadJSON(&authCall); err != nil {
			t.Errorf("read auth call: %v", err)
			return
		}

		// Return an error instead of success.
		if err := writeRPCError(conn, authCall.ID, -32000, "Invalid API key"); err != nil {
			t.Logf("write auth error: %v", err)
		}
	}))
	return srv
}

// serverURLToWS converts an httptest server URL (http://...) to ws://...
func serverURLToWS(serverURL string) string {
	return strings.Replace(serverURL, "http://", "ws://", 1)
}

// newTestClient constructs a Client pointing at a test server URL.
func newTestClient(serverURL string) *Client {
	return &Client{
		BaseURL: serverURLToWS(serverURL),
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
	}
}
