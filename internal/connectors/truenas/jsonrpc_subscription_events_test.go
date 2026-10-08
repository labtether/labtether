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
)

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
