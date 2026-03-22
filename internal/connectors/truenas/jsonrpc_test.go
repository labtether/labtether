package truenas

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func TestJSONRPCCallRejectsDisallowedOutboundEndpoint(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	dialCalled := false
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		dialCalled = true
		return nil, errors.New("dial should not be called for disallowed endpoints")
	})

	client := &Client{
		BaseURL: "wss://blocked.example.net",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
	}

	var result map[string]any
	err := client.Call(context.Background(), "system.info", nil, &result)
	if err == nil {
		t.Fatal("expected outbound policy validation error")
	}
	if !strings.Contains(err.Error(), "endpoint validation") || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialCalled {
		t.Fatal("expected dialWS not to be called on endpoint validation failure")
	}
}

func TestJSONRPCSubscribeRejectsDisallowedOutboundEndpoint(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	dialCalled := false
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		dialCalled = true
		return nil, errors.New("dial should not be called for disallowed endpoints")
	})

	client := &Client{
		BaseURL: "wss://blocked.example.net",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
	}

	err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
	if err == nil {
		t.Fatal("expected outbound policy validation error")
	}
	if !strings.Contains(err.Error(), "endpoint validation") || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialCalled {
		t.Fatal("expected dialWS not to be called on endpoint validation failure")
	}
}

// TestJSONRPCCall_Success verifies that a successful auth + method call
// correctly unmarshals the result into dest.
func TestJSONRPCCall_Success(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	type systemInfo struct {
		Hostname string `json:"hostname"`
		Version  string `json:"version"`
	}

	expectedInfo := systemInfo{Hostname: "truenas-scale", Version: "25.04.0"}

	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		if call.Method != "system.info" {
			t.Errorf("expected method system.info, got %s", call.Method)
			if writeErr := writeRPCError(conn, call.ID, -32601, "Method not found"); writeErr != nil {
				t.Logf("write error response: %v", writeErr)
			}
			return
		}
		if err := writeRPCResult(conn, call.ID, expectedInfo); err != nil {
			t.Errorf("write result: %v", err)
		}
	})
	defer srv.Close()

	client := newTestClient(srv.URL)

	var info systemInfo
	err := client.Call(context.Background(), "system.info", nil, &info)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}

	if info.Hostname != expectedInfo.Hostname {
		t.Errorf("hostname: got %q, want %q", info.Hostname, expectedInfo.Hostname)
	}
	if info.Version != expectedInfo.Version {
		t.Errorf("version: got %q, want %q", info.Version, expectedInfo.Version)
	}
}

// TestJSONRPCCall_AuthFailure verifies that an auth rejection is surfaced as
// an RPCError.
func TestJSONRPCCall_AuthFailure(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServerAuthFail(t)
	defer srv.Close()

	client := newTestClient(srv.URL)

	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatal("expected error from auth failure, got nil")
	}

	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != -32000 {
		t.Errorf("expected error code -32000, got %d", rpcErr.Code)
	}
	if !strings.Contains(rpcErr.Message, "Invalid API key") {
		t.Errorf("expected message to contain 'Invalid API key', got %q", rpcErr.Message)
	}
}

// TestJSONRPCCall_MethodNotFound verifies that a -32601 response is returned
// as an RPCError and that IsMethodNotFound correctly identifies it.
func TestJSONRPCCall_MethodNotFound(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		// vm.query is SCALE-only; simulate CORE returning method-not-found.
		if err := writeRPCError(conn, call.ID, -32601, "Method not found"); err != nil {
			t.Errorf("write method-not-found error: %v", err)
		}
	})
	defer srv.Close()

	client := newTestClient(srv.URL)

	var dest []any
	err := client.Call(context.Background(), "vm.query", nil, &dest)
	if err == nil {
		t.Fatal("expected method-not-found error, got nil")
	}

	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", rpcErr.Code)
	}

	if !IsMethodNotFound(err) {
		t.Errorf("IsMethodNotFound returned false for code -32601 error")
	}

	// Ensure IsMethodNotFound is false for other errors.
	otherErr := &RPCError{Code: -32000, Message: "other error"}
	if IsMethodNotFound(otherErr) {
		t.Errorf("IsMethodNotFound returned true for non-32601 error")
	}
	if IsMethodNotFound(nil) {
		t.Errorf("IsMethodNotFound returned true for nil error")
	}
	if IsMethodCallError(err) {
		t.Errorf("IsMethodCallError returned true for -32601 error")
	}
}

func TestIsMethodCallError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	if !IsMethodCallError(&RPCError{Code: -32001, Message: "Method call error"}) {
		t.Fatalf("expected IsMethodCallError to detect -32001")
	}
	if IsMethodCallError(&RPCError{Code: -32601, Message: "Method not found"}) {
		t.Fatalf("expected IsMethodCallError to ignore -32601")
	}
	if IsMethodCallError(nil) {
		t.Fatalf("expected IsMethodCallError to return false for nil")
	}
}

func TestRPCErrorIncludesReason(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	err := (&RPCError{Code: -32001, Message: "Method call error", Reason: "Missing positional argument: options"}).Error()
	if !strings.Contains(err, "Missing positional argument") {
		t.Fatalf("expected RPC error text to include reason, got %q", err)
	}
}

func TestRPCErrorFormattingFallbacks(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	unknown := (&RPCError{Code: -32000}).Error()
	if !strings.Contains(unknown, "unknown error") {
		t.Fatalf("expected unknown error fallback, got %q", unknown)
	}

	reasonOnly := (&RPCError{Code: -32000, Reason: "token expired"}).Error()
	if !strings.Contains(reasonOnly, "token expired") {
		t.Fatalf("expected reason-only error to include reason, got %q", reasonOnly)
	}

	alreadyIncluded := (&RPCError{
		Code:    -32001,
		Message: "Method call error: missing positional argument",
		Reason:  "missing positional argument",
	}).Error()
	if strings.Count(strings.ToLower(alreadyIncluded), "missing positional argument") != 1 {
		t.Fatalf("expected reason not to be duplicated, got %q", alreadyIncluded)
	}
}

func TestClientWSEndpointVariants(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "empty default", base: "", want: "wss://localhost/api/current"},
		{name: "https", base: "https://truenas.local", want: "wss://truenas.local/api/current"},
		{name: "http with path query fragment", base: "http://truenas.local:8080/old/path?x=1#frag", want: "ws://truenas.local:8080/api/current"},
		{name: "ws passthrough", base: "ws://truenas.local/custom", want: "ws://truenas.local/api/current"},
		{name: "wss passthrough", base: "wss://truenas.local/custom", want: "wss://truenas.local/api/current"},
		{name: "unknown scheme coerced", base: "ftp://truenas.local/root", want: "wss://truenas.local/api/current"},
		{name: "parse fallback", base: "http://%", want: "wss://%/api/current"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := (&Client{BaseURL: tc.base}).wsEndpoint()
			if got != tc.want {
				t.Fatalf("wsEndpoint(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

func TestJSONRPCCall_TimeoutDefaultsWhenUnset(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		if call.Method != "system.info" {
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
			return
		}
		_ = writeRPCResult(conn, call.ID, map[string]any{"hostname": "truenas"})
	})
	defer srv.Close()

	client := &Client{
		BaseURL: serverURLToWS(srv.URL),
		APIKey:  "test-api-key",
		Timeout: 0, // Force default timeout path.
	}

	var info map[string]any
	if err := client.Call(context.Background(), "system.info", nil, &info); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
}

func TestJSONRPCCall_DialError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	client := &Client{
		BaseURL: "wss://127.0.0.1:1",
		APIKey:  "test-api-key",
		Timeout: 200 * time.Millisecond,
	}
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected dial error")
	}
	if !strings.Contains(err.Error(), "truenas ws dial") {
		t.Fatalf("expected dial error message, got %v", err)
	}
}

func TestJSONRPCCall_AuthReadError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()
		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		// Close without sending an auth response.
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected auth read error")
	}
	if !strings.Contains(err.Error(), "truenas ws auth read") {
		t.Fatalf("expected auth read error, got %v", err)
	}
}

func TestJSONRPCCall_AuthResponseIDMismatch(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID+1, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected auth id mismatch error")
	}
	if !strings.Contains(err.Error(), "auth: response id mismatch") {
		t.Fatalf("expected auth id mismatch message, got %v", err)
	}
}

func TestJSONRPCCall_AuthUnexpectedResultFormat(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, map[string]any{"ok": true}); err != nil {
			t.Fatalf("write auth response: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected auth format error")
	}
	if !strings.Contains(err.Error(), "unexpected result format") {
		t.Fatalf("expected auth format error message, got %v", err)
	}
}

func TestJSONRPCCall_AuthRejectedFalse(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, false); err != nil {
			t.Fatalf("write auth response: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected auth rejection error")
	}
	if !strings.Contains(err.Error(), "server rejected api key") {
		t.Fatalf("expected auth rejection error message, got %v", err)
	}
}

func TestJSONRPCCall_MethodReadError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var method rpcCall
		if err := conn.ReadJSON(&method); err != nil {
			t.Fatalf("read method call: %v", err)
		}
		// Close without returning the method result.
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected method read error")
	}
	if !strings.Contains(err.Error(), "truenas ws call read (system.info)") {
		t.Fatalf("expected method read error message, got %v", err)
	}
}

func TestJSONRPCCall_MethodResponseIDMismatch(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var method rpcCall
		if err := conn.ReadJSON(&method); err != nil {
			t.Fatalf("read method call: %v", err)
		}
		if err := writeRPCResult(conn, method.ID+10, map[string]any{"ok": true}); err != nil {
			t.Fatalf("write method response: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected method id mismatch")
	}
	if !strings.Contains(err.Error(), "call (system.info): response id mismatch") {
		t.Fatalf("expected method id mismatch message, got %v", err)
	}
}

func TestJSONRPCCall_MethodDecodeError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var method rpcCall
		if err := conn.ReadJSON(&method); err != nil {
			t.Fatalf("read method call: %v", err)
		}
		if err := writeRPCResult(conn, method.ID, map[string]any{"hostname": "truenas"}); err != nil {
			t.Fatalf("write method response: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	var dest []string
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if !strings.Contains(err.Error(), "truenas ws decode result (system.info)") {
		t.Fatalf("expected decode error message, got %v", err)
	}
}

func TestJSONRPCCall_SetWriteDeadlineError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			setWriteDeadlineFn: func(_ time.Time) error { return errors.New("set write failed") },
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected set write deadline error")
	}
	if !strings.Contains(err.Error(), "truenas ws set write deadline") {
		t.Fatalf("expected set write deadline error message, got %v", err)
	}
}

func TestJSONRPCCall_SetReadDeadlineError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			setReadDeadlineFn: func(_ time.Time) error { return errors.New("set read failed") },
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected set read deadline error")
	}
	if !strings.Contains(err.Error(), "truenas ws set read deadline") {
		t.Fatalf("expected set read deadline error message, got %v", err)
	}
}

func TestJSONRPCCall_AuthSendError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			writeJSONFn: func(any) error { return errors.New("write failed") },
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected auth send error")
	}
	if !strings.Contains(err.Error(), "truenas ws auth send") {
		t.Fatalf("expected auth send error message, got %v", err)
	}
}

func TestJSONRPCCall_MethodSendError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var authID uint64
	writeCount := 0
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			writeJSONFn: func(v any) error {
				req, ok := v.(rpcRequest)
				if !ok {
					t.Fatalf("expected rpcRequest, got %T", v)
				}
				writeCount++
				if writeCount == 1 {
					authID = req.ID
					return nil
				}
				return errors.New("method send failed")
			},
			readJSONFn: func(v any) error {
				resp, ok := v.(*rpcResponse)
				if !ok {
					t.Fatalf("expected *rpcResponse destination, got %T", v)
				}
				*resp = rpcResponse{
					JSONRPC: "2.0",
					ID:      authID,
					Result:  json.RawMessage("true"),
				}
				return nil
			},
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	var dest map[string]any
	err := client.Call(context.Background(), "system.info", nil, &dest)
	if err == nil {
		t.Fatalf("expected method send error")
	}
	if !strings.Contains(err.Error(), "truenas ws call send (system.info)") {
		t.Fatalf("expected method send error message, got %v", err)
	}
}

func TestJSONRPCSubscribe_StreamEvent(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if auth.Method != "auth.login_with_api_key" {
			t.Fatalf("expected auth method, got %s", auth.Method)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var subscribe rpcCall
		if err := conn.ReadJSON(&subscribe); err != nil {
			t.Fatalf("read subscribe call: %v", err)
		}
		if subscribe.Method != "core.subscribe" {
			t.Fatalf("expected core.subscribe, got %s", subscribe.Method)
		}
		if err := writeRPCResult(conn, subscribe.ID, "sub-alerts"); err != nil {
			t.Fatalf("write subscribe response: %v", err)
		}

		if err := conn.WriteJSON(map[string]any{
			"id":         "sub-alerts",
			"collection": "alert.list",
			"msg":        "changed",
			"fields": map[string]any{
				"uuid":      "alert-1",
				"formatted": "Pool degraded",
			},
		}); err != nil {
			t.Fatalf("write event: %v", err)
		}

		// Keep the socket open until the client closes it on context cancellation.
		_, _, _ = conn.ReadMessage()
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := 0
	err := client.Subscribe(ctx, "alert.list", func(event SubscriptionEvent) error {
		received++
		if event.Collection != "alert.list" {
			t.Fatalf("event collection = %q, want alert.list", event.Collection)
		}
		if event.MessageType != "changed" {
			t.Fatalf("event message type = %q, want changed", event.MessageType)
		}
		if got := anyToIdentifier(event.Fields["uuid"]); got != "alert-1" {
			t.Fatalf("event uuid = %q, want alert-1", got)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Subscribe() error = %v, want context.Canceled", err)
	}
	if received != 1 {
		t.Fatalf("expected 1 event, got %d", received)
	}
}

func TestJSONRPCSubscribe_JSONRPCNotificationShape(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var subscribe rpcCall
		if err := conn.ReadJSON(&subscribe); err != nil {
			t.Fatalf("read subscribe call: %v", err)
		}
		if err := writeRPCResult(conn, subscribe.ID, "sub-1"); err != nil {
			t.Fatalf("write subscribe response: %v", err)
		}

		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"method":  "collection_update",
			"params": []any{
				"sub-1",
				"added",
				"alert.list",
				map[string]any{
					"uuid":      "alert-2",
					"formatted": "Disk warning",
				},
			},
		}); err != nil {
			t.Fatalf("write notification event: %v", err)
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var gotEvent SubscriptionEvent
	err := client.Subscribe(ctx, "alert.list", func(event SubscriptionEvent) error {
		gotEvent = event
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Subscribe() error = %v, want context.Canceled", err)
	}
	if gotEvent.Collection != "alert.list" {
		t.Fatalf("event collection = %q, want alert.list", gotEvent.Collection)
	}
	if gotEvent.MessageType != "added" {
		t.Fatalf("event message type = %q, want added", gotEvent.MessageType)
	}
	if got := anyToIdentifier(gotEvent.Fields["uuid"]); got != "alert-2" {
		t.Fatalf("event uuid = %q, want alert-2", got)
	}
}

func TestJSONRPCSubscribe_SubscribeRPCError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer conn.Close()

		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Fatalf("read auth call: %v", err)
		}
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var subscribe rpcCall
		if err := conn.ReadJSON(&subscribe); err != nil {
			t.Fatalf("read subscribe call: %v", err)
		}
		if err := writeRPCError(conn, subscribe.ID, -32601, "Method not found"); err != nil {
			t.Fatalf("write subscribe error: %v", err)
		}
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected subscribe rpc error")
	}
	if !IsMethodNotFound(err) {
		t.Fatalf("expected method-not-found error, got %v", err)
	}
}

func TestParseSubscriptionEventIgnoresRPCResponses(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	_, ok := parseSubscriptionEvent(map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"result":  true,
	}, "alert.list", "sub-1")
	if ok {
		t.Fatalf("expected parseSubscriptionEvent to ignore rpc result envelope")
	}
}

func TestJSONRPCSubscribeEmptyCollection(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key"}
	if err := client.Subscribe(context.Background(), "  ", nil); err == nil {
		t.Fatalf("expected validation error for empty collection")
	}
}

func TestJSONRPCSubscribeSetReadDeadlineClearError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var authID uint64
	var subscribeID uint64
	readDeadlineCalls := 0

	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			setReadDeadlineFn: func(deadline time.Time) error {
				readDeadlineCalls++
				if readDeadlineCalls == 2 {
					return errors.New("clear read deadline failed")
				}
				if deadline.IsZero() {
					t.Fatalf("expected handshake read deadline to be non-zero")
				}
				return nil
			},
			writeJSONFn: func(v any) error {
				req, ok := v.(rpcRequest)
				if !ok {
					t.Fatalf("expected rpcRequest, got %T", v)
				}
				switch req.Method {
				case "auth.login_with_api_key":
					authID = req.ID
				case "core.subscribe":
					subscribeID = req.ID
				default:
					t.Fatalf("unexpected method %q", req.Method)
				}
				return nil
			},
			readJSONFn: func(v any) error {
				resp, ok := v.(*rpcResponse)
				if !ok {
					t.Fatalf("expected *rpcResponse destination, got %T", v)
				}
				if authID != 0 && resp.ID == 0 {
					*resp = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
					authID = 0
					return nil
				}
				*resp = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-alerts"`)}
				return nil
			},
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
	if err == nil {
		t.Fatalf("expected set read deadline clear error")
	}
	if !strings.Contains(err.Error(), "truenas ws set read deadline") {
		t.Fatalf("expected set read deadline error message, got %v", err)
	}
}

func TestJSONRPCSubscribeStreamReadBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("stream read error", func(t *testing.T) {
		var authID uint64
		var subscribeID uint64
		readCount := 0

		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					if req.Method == "auth.login_with_api_key" {
						authID = req.ID
					}
					if req.Method == "core.subscribe" {
						subscribeID = req.ID
					}
					return nil
				},
				readJSONFn: func(v any) error {
					readCount++
					switch dst := v.(type) {
					case *rpcResponse:
						if readCount == 1 {
							*dst = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						*dst = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-alerts"`)}
						return nil
					case *map[string]any:
						return io.EOF
					default:
						t.Fatalf("unexpected destination type %T", v)
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil {
			t.Fatalf("expected stream read error")
		}
		if !strings.Contains(err.Error(), "subscribe stream (alert.list)") {
			t.Fatalf("unexpected stream error: %v", err)
		}
	})

	t.Run("context canceled during stream read", func(t *testing.T) {
		var authID uint64
		var subscribeID uint64
		readCount := 0

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					if req.Method == "auth.login_with_api_key" {
						authID = req.ID
					}
					if req.Method == "core.subscribe" {
						subscribeID = req.ID
					}
					return nil
				},
				readJSONFn: func(v any) error {
					readCount++
					switch dst := v.(type) {
					case *rpcResponse:
						if readCount == 1 {
							*dst = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						*dst = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-alerts"`)}
						return nil
					case *map[string]any:
						cancel()
						return io.EOF
					default:
						t.Fatalf("unexpected destination type %T", v)
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(ctx, "alert.list", func(event SubscriptionEvent) error { return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestJSONRPCSubscribeHandlerBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("handler returns error", func(t *testing.T) {
		var authID uint64
		var subscribeID uint64
		readCount := 0
		want := errors.New("handler failed")

		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					if req.Method == "auth.login_with_api_key" {
						authID = req.ID
					}
					if req.Method == "core.subscribe" {
						subscribeID = req.ID
					}
					return nil
				},
				readJSONFn: func(v any) error {
					readCount++
					switch dst := v.(type) {
					case *rpcResponse:
						if readCount == 1 {
							*dst = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						*dst = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-alerts"`)}
						return nil
					case *map[string]any:
						*dst = map[string]any{
							"collection": "alert.list",
							"msg":        "changed",
							"fields": map[string]any{
								"uuid": "alert-3",
							},
						}
						return nil
					default:
						t.Fatalf("unexpected destination type %T", v)
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return want })
		if !errors.Is(err, want) {
			t.Fatalf("expected handler error, got %v", err)
		}
	})

	t.Run("nil handler ignores events", func(t *testing.T) {
		var authID uint64
		var subscribeID uint64
		readCount := 0
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					if req.Method == "auth.login_with_api_key" {
						authID = req.ID
					}
					if req.Method == "core.subscribe" {
						subscribeID = req.ID
					}
					return nil
				},
				readJSONFn: func(v any) error {
					readCount++
					switch dst := v.(type) {
					case *rpcResponse:
						if readCount == 1 {
							*dst = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						*dst = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-alerts"`)}
						return nil
					case *map[string]any:
						if readCount == 3 {
							*dst = map[string]any{
								"collection": "alert.list",
								"msg":        "changed",
								"fields": map[string]any{
									"uuid": "alert-4",
								},
							}
							return nil
						}
						cancel()
						return io.EOF
					default:
						t.Fatalf("unexpected destination type %T", v)
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(ctx, "alert.list", nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	})
}

func TestParseSubscriptionIDAndIdentifierHelpers(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	if got := parseSubscriptionID(nil); got != "" {
		t.Fatalf("parseSubscriptionID(nil) = %q, want empty", got)
	}
	if got := parseSubscriptionID(json.RawMessage("{")); got != "" {
		t.Fatalf("parseSubscriptionID(invalid) = %q, want empty", got)
	}
	if got := parseSubscriptionID(json.RawMessage(`123`)); got != "123" {
		t.Fatalf("parseSubscriptionID(number) = %q, want 123", got)
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: ""},
		{name: "string", value: " abc ", want: "abc"},
		{name: "float64", value: float64(5), want: "5"},
		{name: "float32", value: float32(7), want: "7"},
		{name: "int", value: 9, want: "9"},
		{name: "int64", value: int64(11), want: "11"},
		{name: "uint64", value: uint64(13), want: "13"},
		{name: "json number", value: json.Number("15"), want: "15"},
		{name: "stringer", value: testIdentifierStringer("value"), want: "value"},
		{name: "fallback fmt", value: true, want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := anyToIdentifier(tt.value); got != tt.want {
				t.Fatalf("anyToIdentifier(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseNotificationParamsShapes(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subID := parseNotificationParams(nil)
	if collection != "" || messageType != "" || fields != nil || subID != "" {
		t.Fatalf("unexpected decode for nil params")
	}

	collection, messageType, fields, subID = parseNotificationParams([]any{
		"sub-1",
		"added",
		"alert.list",
		map[string]any{"uuid": "alert-5"},
	})
	if collection != "alert.list" || messageType != "added" || subID != "sub-1" {
		t.Fatalf("unexpected 4-arg decode: collection=%q msg=%q sub=%q", collection, messageType, subID)
	}
	if got := anyToIdentifier(fields["uuid"]); got != "alert-5" {
		t.Fatalf("unexpected fields uuid %q", got)
	}

	collection, messageType, fields, subID = parseNotificationParams([]any{
		"alert.list",
		"changed",
		map[string]any{"uuid": "alert-6"},
	})
	if collection != "map[uuid:alert-6]" || messageType != "changed" || subID != "alert.list" {
		t.Fatalf("unexpected alternate shape decode: collection=%q msg=%q sub=%q", collection, messageType, subID)
	}
	if got := anyToIdentifier(fields["uuid"]); got != "alert-6" {
		t.Fatalf("unexpected alternate fields uuid %q", got)
	}
}

func TestParseSubscriptionEventAdditionalBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("ignore rpc error envelope", func(t *testing.T) {
		_, ok := parseSubscriptionEvent(map[string]any{
			"id":    1,
			"error": map[string]any{"code": -32000},
		}, "alert.list", "sub-1")
		if ok {
			t.Fatalf("expected rpc error envelope to be ignored")
		}
	})

	t.Run("fallback by subscription id when collection mismatched", func(t *testing.T) {
		event, ok := parseSubscriptionEvent(map[string]any{
			"id":         "sub-1",
			"collection": "pool.query",
			"msg":        "changed",
			"fields": map[string]any{
				"uuid": "pool-1",
			},
		}, "alert.list", "sub-1")
		if !ok {
			t.Fatalf("expected event to pass via subscription id fallback")
		}
		if event.Collection != "pool.query" {
			t.Fatalf("event collection = %q, want pool.query", event.Collection)
		}
	})

	t.Run("filter mismatched collection and subscription id", func(t *testing.T) {
		_, ok := parseSubscriptionEvent(map[string]any{
			"id":         "sub-other",
			"collection": "pool.query",
			"msg":        "changed",
			"fields":     map[string]any{"uuid": "pool-2"},
		}, "alert.list", "sub-1")
		if ok {
			t.Fatalf("expected mismatched event to be filtered")
		}
	})

	t.Run("notification params without fields are still surfaced with collection context", func(t *testing.T) {
		event, ok := parseSubscriptionEvent(map[string]any{
			"method": "collection_update",
			"params": []any{"sub-1", "changed", "alert.list"},
		}, "alert.list", "sub-1")
		if !ok {
			t.Fatalf("expected event to be surfaced")
		}
		if event.Collection != "alert.list" {
			t.Fatalf("event collection = %q, want alert.list", event.Collection)
		}
	})
}

func TestJSONRPCSubscribeHandshakeErrorBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("set write deadline error", func(t *testing.T) {
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				setWriteDeadlineFn: func(_ time.Time) error { return errors.New("set write failed") },
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "set write deadline") {
			t.Fatalf("expected set write deadline error, got %v", err)
		}
	})

	t.Run("set read deadline error", func(t *testing.T) {
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				setReadDeadlineFn: func(_ time.Time) error { return errors.New("set read failed") },
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "set read deadline") {
			t.Fatalf("expected set read deadline error, got %v", err)
		}
	})

	t.Run("auth send error", func(t *testing.T) {
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(any) error { return errors.New("auth send failed") },
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "auth send") {
			t.Fatalf("expected auth send error, got %v", err)
		}
	})

	t.Run("auth read error", func(t *testing.T) {
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(any) error { return nil },
				readJSONFn:  func(any) error { return io.EOF },
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "auth read") {
			t.Fatalf("expected auth read error, got %v", err)
		}
	})

	t.Run("auth id mismatch", func(t *testing.T) {
		var authID uint64
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					authID = req.ID
					return nil
				},
				readJSONFn: func(v any) error {
					resp := v.(*rpcResponse)
					*resp = rpcResponse{JSONRPC: "2.0", ID: authID + 1, Result: json.RawMessage("true")}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "auth: response id mismatch") {
			t.Fatalf("expected auth id mismatch error, got %v", err)
		}
	})

	t.Run("auth rpc error", func(t *testing.T) {
		var authID uint64
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					authID = req.ID
					return nil
				},
				readJSONFn: func(v any) error {
					resp := v.(*rpcResponse)
					*resp = rpcResponse{
						JSONRPC: "2.0",
						ID:      authID,
						Error:   &rpcErrorBody{Code: -32000, Message: "invalid api key"},
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil {
			t.Fatalf("expected auth rpc error")
		}
		if rpcErr, ok := err.(*RPCError); !ok || rpcErr.Code != -32000 {
			t.Fatalf("expected *RPCError code -32000, got %T %v", err, err)
		}
	})

	t.Run("auth unexpected result format", func(t *testing.T) {
		var authID uint64
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					authID = req.ID
					return nil
				},
				readJSONFn: func(v any) error {
					resp := v.(*rpcResponse)
					*resp = rpcResponse{
						JSONRPC: "2.0",
						ID:      authID,
						Result:  json.RawMessage(`{"ok":true}`),
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "unexpected result format") {
			t.Fatalf("expected auth format error, got %v", err)
		}
	})

	t.Run("auth rejected", func(t *testing.T) {
		var authID uint64
		stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
			return &fakeWSConn{
				writeJSONFn: func(v any) error {
					req := v.(rpcRequest)
					authID = req.ID
					return nil
				},
				readJSONFn: func(v any) error {
					resp := v.(*rpcResponse)
					*resp = rpcResponse{
						JSONRPC: "2.0",
						ID:      authID,
						Result:  json.RawMessage("false"),
					}
					return nil
				},
			}, nil
		})

		client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
		err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "server rejected api key") {
			t.Fatalf("expected auth rejection error, got %v", err)
		}
	})

	t.Run("subscribe send/read/id mismatch errors", func(t *testing.T) {
		t.Run("send error", func(t *testing.T) {
			var writeCount int
			var authID uint64
			stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
				return &fakeWSConn{
					writeJSONFn: func(v any) error {
						req := v.(rpcRequest)
						writeCount++
						if req.Method == "auth.login_with_api_key" {
							authID = req.ID
							return nil
						}
						return errors.New("subscribe send failed")
					},
					readJSONFn: func(v any) error {
						resp := v.(*rpcResponse)
						*resp = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
						return nil
					},
				}, nil
			})

			client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
			err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
			if err == nil || !strings.Contains(err.Error(), "subscribe send") {
				t.Fatalf("expected subscribe send error, got %v", err)
			}
		})

		t.Run("read error", func(t *testing.T) {
			var authID uint64
			readCount := 0
			stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
				return &fakeWSConn{
					writeJSONFn: func(v any) error {
						req := v.(rpcRequest)
						if req.Method == "auth.login_with_api_key" {
							authID = req.ID
						}
						return nil
					},
					readJSONFn: func(v any) error {
						readCount++
						resp := v.(*rpcResponse)
						if readCount == 1 {
							*resp = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						return io.EOF
					},
				}, nil
			})

			client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
			err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
			if err == nil || !strings.Contains(err.Error(), "subscribe read") {
				t.Fatalf("expected subscribe read error, got %v", err)
			}
		})

		t.Run("id mismatch", func(t *testing.T) {
			var authID uint64
			var subscribeID uint64
			readCount := 0
			stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
				return &fakeWSConn{
					writeJSONFn: func(v any) error {
						req := v.(rpcRequest)
						if req.Method == "auth.login_with_api_key" {
							authID = req.ID
						}
						if req.Method == "core.subscribe" {
							subscribeID = req.ID
						}
						return nil
					},
					readJSONFn: func(v any) error {
						readCount++
						resp := v.(*rpcResponse)
						if readCount == 1 {
							*resp = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
							return nil
						}
						*resp = rpcResponse{JSONRPC: "2.0", ID: subscribeID + 1, Result: json.RawMessage(`"sub-1"`)}
						return nil
					},
				}, nil
			})

			client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
			err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
			if err == nil || !strings.Contains(err.Error(), "subscribe (alert.list): response id mismatch") {
				t.Fatalf("expected subscribe id mismatch error, got %v", err)
			}
		})
	})
}

func TestParseNotificationParamsAdditionalBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subscriptionID := parseNotificationParams([]any{})
	if collection != "" || messageType != "" || fields != nil || subscriptionID != "" {
		t.Fatalf("expected empty decode for empty params slice")
	}

	collection, messageType, fields, subscriptionID = parseNotificationParams([]any{
		"sub-only",
		"changed",
		"alert.list",
		"not-a-map",
	})
	if collection != "alert.list" || messageType != "changed" || subscriptionID != "sub-only" || fields != nil {
		t.Fatalf("unexpected decode for non-map fields payload: collection=%q msg=%q sub=%q fields=%#v", collection, messageType, subscriptionID, fields)
	}
}

func TestParseSubscriptionEventFieldsOverrideAndDefaults(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	event, ok := parseSubscriptionEvent(map[string]any{
		"method": "collection_update",
		"params": []any{"sub-1", "", "alert.list", map[string]any{"uuid": "from-params"}},
		"fields": map[string]any{"uuid": "from-fields"},
	}, "alert.list", "sub-1")
	if !ok {
		t.Fatalf("expected event to be parsed")
	}
	if event.MessageType != "event" {
		t.Fatalf("expected default message type event, got %q", event.MessageType)
	}
	if got := anyToIdentifier(event.Fields["uuid"]); got != "from-fields" {
		t.Fatalf("expected explicit fields payload to override params payload, got %q", got)
	}
}

func TestParseSubscriptionEventCollectionFallbackAndEmptyPayloadRejection(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	event, ok := parseSubscriptionEvent(map[string]any{
		"msg": "changed",
	}, "alert.list", "sub-1")
	if !ok {
		t.Fatalf("expected event with expected collection fallback")
	}
	if event.Collection != "alert.list" {
		t.Fatalf("event collection = %q, want alert.list", event.Collection)
	}

	if _, ok := parseSubscriptionEvent(map[string]any{
		"msg": "changed",
	}, "", ""); ok {
		t.Fatalf("expected empty collection+fields event to be rejected")
	}
}

func TestParseNotificationParamsLenOneCollectionFallback(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subscriptionID := parseNotificationParams([]any{"alert.list"})
	if collection != "alert.list" {
		t.Fatalf("collection = %q, want alert.list", collection)
	}
	if messageType != "" || fields != nil {
		t.Fatalf("unexpected message/fields for len1 params: msg=%q fields=%#v", messageType, fields)
	}
	if subscriptionID != "alert.list" {
		t.Fatalf("subscriptionID = %q, want alert.list", subscriptionID)
	}
}

func TestJSONRPCSubscribeDefaultTimeoutAndDialError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	client := &Client{
		BaseURL: "wss://127.0.0.1:1",
		APIKey:  "test-api-key",
		Timeout: 0,
	}

	err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
	if err == nil {
		t.Fatalf("expected dial error with default timeout path")
	}
	if !strings.Contains(err.Error(), "truenas ws dial") {
		t.Fatalf("expected dial error message, got %v", err)
	}
}

func TestJSONRPCSubscribeIgnoresNonEventPayloads(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var authID uint64
	var subscribeID uint64
	readCount := 0
	handlerCalls := 0
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		return &fakeWSConn{
			writeJSONFn: func(v any) error {
				req := v.(rpcRequest)
				if req.Method == "auth.login_with_api_key" {
					authID = req.ID
				}
				if req.Method == "core.subscribe" {
					subscribeID = req.ID
				}
				return nil
			},
			readJSONFn: func(v any) error {
				readCount++
				switch dst := v.(type) {
				case *rpcResponse:
					if readCount == 1 {
						*dst = rpcResponse{JSONRPC: "2.0", ID: authID, Result: json.RawMessage("true")}
						return nil
					}
					*dst = rpcResponse{JSONRPC: "2.0", ID: subscribeID, Result: json.RawMessage(`"sub-1"`)}
					return nil
				case *map[string]any:
					if readCount == 3 {
						*dst = map[string]any{"jsonrpc": "2.0", "id": 123, "result": true}
						return nil
					}
					cancel()
					return io.EOF
				default:
					t.Fatalf("unexpected destination type %T", v)
				}
				return nil
			},
		}, nil
	})

	client := &Client{BaseURL: "https://truenas.local", APIKey: "test-api-key", Timeout: time.Second}
	err := client.Subscribe(ctx, "alert.list", func(event SubscriptionEvent) error {
		handlerCalls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if handlerCalls != 0 {
		t.Fatalf("expected non-event payload to be ignored before handler, calls=%d", handlerCalls)
	}
}
