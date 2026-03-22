package truenas

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
)

func TestConnectorCallQueryRetriesMethodCallError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var attempts int

	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		attempts++
		if call.Method != "pool.query" {
			if err := writeRPCError(conn, call.ID, -32601, "Method not found"); err != nil {
				t.Fatalf("write error response: %v", err)
			}
			return
		}

		var params []any
		if err := json.Unmarshal(call.Params, &params); err != nil {
			t.Fatalf("decode params: %v", err)
		}

		switch attempts {
		case 1:
			if len(params) != 0 {
				t.Fatalf("attempt 1 params = %#v, want empty params", params)
			}
			if err := writeRPCError(conn, call.ID, -32001, "Method call error"); err != nil {
				t.Fatalf("write method call error: %v", err)
			}
		case 2:
			if len(params) != 2 {
				t.Fatalf("attempt 2 params len = %d, want 2", len(params))
			}
			if err := writeRPCResult(conn, call.ID, []map[string]any{{"id": 1, "name": "mainpool"}}); err != nil {
				t.Fatalf("write result: %v", err)
			}
		default:
			t.Fatalf("unexpected extra attempt: %d", attempts)
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	var pools []map[string]any
	if err := connector.callQuery(context.Background(), "pool.query", &pools); err != nil {
		t.Fatalf("callQuery() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(pools) != 1 || anyToString(pools[0]["name"]) != "mainpool" {
		t.Fatalf("unexpected pools result: %#v", pools)
	}
}

func TestConnectorCallQueryNoRetryForNonMethodCallError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var attempts int

	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		attempts++
		if err := writeRPCError(conn, call.ID, -32000, "permission denied"); err != nil {
			t.Fatalf("write error response: %v", err)
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	var pools []map[string]any
	err := connector.callQuery(context.Background(), "pool.query", &pools)
	if err == nil {
		t.Fatalf("expected callQuery error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestConnectorCallQueryReturnsFinalMethodCallErrorAfterAllRetries(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var attempts int

	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		attempts++
		if err := writeRPCError(conn, call.ID, -32001, "Method call error"); err != nil {
			t.Fatalf("write error response: %v", err)
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	var pools []map[string]any
	err := connector.callQuery(context.Background(), "pool.query", &pools)
	if err == nil {
		t.Fatalf("expected callQuery error after exhausting retries")
	}
	if !IsMethodCallError(err) {
		t.Fatalf("expected final error to be method call error, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestConnectorCallQueryStopsRetryOnNonMethodCallRetryError(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	var attempts int

	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		attempts++
		switch attempts {
		case 1:
			if err := writeRPCError(conn, call.ID, -32001, "Method call error"); err != nil {
				t.Fatalf("write method call error: %v", err)
			}
		case 2:
			if err := writeRPCError(conn, call.ID, -32000, "permission denied"); err != nil {
				t.Fatalf("write non-method error: %v", err)
			}
		default:
			t.Fatalf("unexpected extra attempt: %d", attempts)
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	var pools []map[string]any
	err := connector.callQuery(context.Background(), "pool.query", &pools)
	if err == nil {
		t.Fatalf("expected callQuery error")
	}
	if IsMethodCallError(err) {
		t.Fatalf("expected non-method-call error after retry stop, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
