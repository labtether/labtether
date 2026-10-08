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
