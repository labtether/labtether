package truenas

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
