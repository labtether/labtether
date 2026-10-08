package truenas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
