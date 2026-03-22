package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/assets"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

type failingTrueNASSessionAssetStore struct {
	persistence.AssetStore
	err error
}

func (s *failingTrueNASSessionAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	if s.err != nil {
		return assets.Asset{}, false, s.err
	}
	return s.AssetStore.GetAsset(id)
}

func TestTrueNASShellEndpointVariants(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "https", base: "https://truenas.local", want: "wss://truenas.local/websocket/shell"},
		{name: "http with path/query", base: "http://truenas.local:8080/path?q=1#frag", want: "ws://truenas.local:8080/websocket/shell"},
		{name: "ws passthrough", base: "ws://truenas.local/custom", want: "ws://truenas.local/websocket/shell"},
		{name: "wss passthrough", base: "wss://truenas.local/custom", want: "wss://truenas.local/websocket/shell"},
		{name: "unknown scheme", base: "ftp://truenas.local/root", want: "wss://truenas.local/websocket/shell"},
		{name: "parse fallback", base: "http://%", want: "wss://http://%/websocket/shell"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truenasShellEndpoint(tc.base); got != tc.want {
				t.Fatalf("truenasShellEndpoint(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

func TestTryTrueNASTerminalStreamAuthFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/current" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		var authReq map[string]any
		if err := conn.ReadJSON(&authReq); err != nil {
			t.Fatalf("read auth request: %v", err)
		}
		if method := strings.TrimSpace(collectorAnyString(authReq["method"])); method != "auth.login_with_api_key" {
			t.Fatalf("expected auth.login_with_api_key, got %q", method)
		}
		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      authReq["id"],
			"result":  true,
		}); err != nil {
			t.Fatalf("write auth response: %v", err)
		}

		var tokenReq map[string]any
		if err := conn.ReadJSON(&tokenReq); err != nil {
			t.Fatalf("read token request: %v", err)
		}
		if err := conn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      tokenReq["id"],
			"error": map[string]any{
				"code":    -32000,
				"message": "token denied",
			},
		}); err != nil {
			t.Fatalf("write token failure: %v", err)
		}
	}))
	defer nas.Close()

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/terminal/stream", nil)
	rec := httptest.NewRecorder()
	err := sut.tryTrueNASTerminalStream(rec, req, terminal.Session{ID: "sess-1", Target: "truenas-host-1"}, truenasShellTarget{
		BaseURL:    nas.URL,
		APIKey:     "api-key",
		SkipVerify: true,
		Timeout:    time.Second,
	})
	if err == nil {
		t.Fatalf("expected auth token failure")
	}
	if !strings.Contains(err.Error(), "auth.generate_token failed") {
		t.Fatalf("unexpected auth failure: %v", err)
	}
}

func TestTryTrueNASTerminalStreamProxySuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/current":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("api/current upgrade failed: %v", err)
			}
			defer conn.Close()

			var authReq map[string]any
			if err := conn.ReadJSON(&authReq); err != nil {
				t.Fatalf("read auth request: %v", err)
			}
			if err := conn.WriteJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      authReq["id"],
				"result":  true,
			}); err != nil {
				t.Fatalf("write auth response: %v", err)
			}

			var tokenReq map[string]any
			if err := conn.ReadJSON(&tokenReq); err != nil {
				t.Fatalf("read token request: %v", err)
			}
			if strings.TrimSpace(collectorAnyString(tokenReq["method"])) != "auth.generate_token" {
				t.Fatalf("expected auth.generate_token, got %#v", tokenReq["method"])
			}
			if err := conn.WriteJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      tokenReq["id"],
				"result":  "token-123",
			}); err != nil {
				t.Fatalf("write token response: %v", err)
			}
		case "/websocket/shell":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("shell upgrade failed: %v", err)
			}
			defer conn.Close()

			var authMsg map[string]any
			if err := conn.ReadJSON(&authMsg); err != nil {
				t.Fatalf("read shell auth message: %v", err)
			}
			if got := strings.TrimSpace(collectorAnyString(authMsg["token"])); got != "token-123" {
				t.Fatalf("shell token = %q, want token-123", got)
			}
			if err := conn.WriteJSON(map[string]any{"msg": "connected"}); err != nil {
				t.Fatalf("write shell connected: %v", err)
			}

			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read proxied browser payload: %v", err)
			}
			if msgType != websocket.TextMessage {
				t.Fatalf("expected text message, got %d", msgType)
			}
			if string(payload) != "pwd\n" {
				t.Fatalf("unexpected browser payload %q", string(payload))
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte("echo:pwd\n")); err != nil {
				t.Fatalf("write proxied shell payload: %v", err)
			}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), time.Now().Add(500*time.Millisecond))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer nas.Close()

	sut := newTestAPIServer(t)
	session := terminal.Session{ID: "sess-truenas-1", Target: "truenas-host-1"}
	target := truenasShellTarget{
		BaseURL:    nas.URL,
		APIKey:     "api-key",
		SkipVerify: true,
		Timeout:    2 * time.Second,
		Options:    map[string]any{"vm_id": "101"},
	}

	errCh := make(chan error, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errCh <- sut.tryTrueNASTerminalStream(w, r, session, target)
	}))
	defer proxy.Close()

	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
	browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("browser websocket dial failed: %v", err)
	}
	defer browserWS.Close()

	if err := browserWS.WriteMessage(websocket.TextMessage, []byte("pwd\n")); err != nil {
		t.Fatalf("browser write failed: %v", err)
	}

	_, payload, err := browserWS.ReadMessage()
	if err != nil {
		t.Fatalf("browser read failed: %v", err)
	}
	if string(payload) != "echo:pwd\n" {
		t.Fatalf("unexpected proxied payload %q", string(payload))
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("tryTrueNASTerminalStream returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for stream handler completion")
	}
}

func TestResolveTrueNASSessionTargetAdditionalBranches(t *testing.T) {
	t.Run("asset store unavailable", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.assetStore = nil
		target, ok, err := sut.resolveTrueNASSessionTarget("any")
		if err != nil || ok || target.BaseURL != "" || target.APIKey != "" || target.Options != nil {
			t.Fatalf("expected unresolved target when asset store unavailable, got target=%+v ok=%v err=%v", target, ok, err)
		}
	})

	t.Run("app asset options", func(t *testing.T) {
		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-app", "api-key-app", "https://truenas-app.local")
		configureTrueNASCollectors(t, sut, hubcollector.Collector{
			ID:            "collector-truenas-app",
			AssetID:       "truenas-cluster-app",
			CollectorType: hubcollector.CollectorTypeTrueNAS,
			Enabled:       true,
			Config: map[string]any{
				"base_url":      "https://truenas-app.local",
				"credential_id": "cred-truenas-app",
				"skip_verify":   true,
			},
		})

		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "truenas-app-portainer",
			Type:    "app",
			Name:    "portainer",
			Source:  "truenas",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-truenas-app",
			},
		})
		if err != nil {
			t.Fatalf("failed to seed app asset: %v", err)
		}

		target, ok, err := sut.resolveTrueNASSessionTarget("truenas-app-portainer")
		if err != nil {
			t.Fatalf("resolveTrueNASSessionTarget() error = %v", err)
		}
		if !ok {
			t.Fatalf("expected truenas app target resolution")
		}
		optionsJSON, _ := json.Marshal(target.Options)
		if !strings.Contains(string(optionsJSON), `"app_name":"portainer"`) {
			t.Fatalf("expected app_name option, got %s", string(optionsJSON))
		}
	})

	t.Run("asset lookup error bubbles up", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.assetStore = &failingTrueNASSessionAssetStore{
			AssetStore: sut.assetStore,
			err:        errors.New("asset store unavailable"),
		}
		target, ok, err := sut.resolveTrueNASSessionTarget("truenas-host-1")
		if err == nil || !strings.Contains(err.Error(), "asset store unavailable") {
			t.Fatalf("expected asset store error, got target=%+v ok=%v err=%v", target, ok, err)
		}
	})

	t.Run("non truenas asset returns false", func(t *testing.T) {
		sut := newTestAPIServer(t)
		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "docker-host-1",
			Type:    "container-host",
			Name:    "docker-host-1",
			Source:  "docker",
			Status:  "online",
		})
		if err != nil {
			t.Fatalf("failed to seed non-truenas asset: %v", err)
		}
		target, ok, err := sut.resolveTrueNASSessionTarget("docker-host-1")
		if err != nil || ok || target.BaseURL != "" {
			t.Fatalf("expected non-truenas target to be ignored, got target=%+v ok=%v err=%v", target, ok, err)
		}
	})

	t.Run("stale collector metadata fallback still fails when no collector exists", func(t *testing.T) {
		sut := newTestAPIServer(t)
		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "truenas-host-stale",
			Type:    "nas",
			Name:    "omeganas",
			Source:  "truenas",
			Status:  "online",
			Metadata: map[string]string{
				"collector_id": "collector-missing",
			},
		})
		if err != nil {
			t.Fatalf("failed to seed truenas asset: %v", err)
		}

		_, _, err = sut.resolveTrueNASSessionTarget("truenas-host-stale")
		if err == nil || !strings.Contains(err.Error(), "hub collector store unavailable") {
			t.Fatalf("expected runtime fallback failure, got %v", err)
		}
	})
}

func TestTryTrueNASTerminalStreamAdditionalFailureBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	newAuthServer := func(t *testing.T, shellHandler func(conn *websocket.Conn)) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/current":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("api/current upgrade failed: %v", err)
				}
				defer conn.Close()

				var authReq map[string]any
				if err := conn.ReadJSON(&authReq); err != nil {
					t.Fatalf("read auth request: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": authReq["id"], "result": true}); err != nil {
					t.Fatalf("write auth response: %v", err)
				}
				var tokenReq map[string]any
				if err := conn.ReadJSON(&tokenReq); err != nil {
					t.Fatalf("read token request: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": tokenReq["id"], "result": "token-xyz"}); err != nil {
					t.Fatalf("write token response: %v", err)
				}
			case "/websocket/shell":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("shell upgrade failed: %v", err)
				}
				defer conn.Close()
				shellHandler(conn)
			default:
				http.NotFound(w, r)
			}
		}))
	}

	t.Run("browser upgrade failure after token retrieval", func(t *testing.T) {
		nas := newAuthServer(t, func(conn *websocket.Conn) {})
		defer nas.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/terminal/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryTrueNASTerminalStream(rec, req, terminal.Session{ID: "sess-upgrade-fail"}, truenasShellTarget{
			BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
		})
		if err == nil || !strings.Contains(err.Error(), "browser ws upgrade") {
			t.Fatalf("expected browser upgrade error, got %v", err)
		}
	})

	t.Run("shell dial failure writes message to browser", func(t *testing.T) {
		nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/current" {
				http.NotFound(w, r)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("api/current upgrade failed: %v", err)
			}
			defer conn.Close()

			var authReq map[string]any
			if err := conn.ReadJSON(&authReq); err != nil {
				t.Fatalf("read auth request: %v", err)
			}
			if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": authReq["id"], "result": true}); err != nil {
				t.Fatalf("write auth response: %v", err)
			}
			var tokenReq map[string]any
			if err := conn.ReadJSON(&tokenReq); err != nil {
				t.Fatalf("read token request: %v", err)
			}
			if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": tokenReq["id"], "result": "token-shell-fail"}); err != nil {
				t.Fatalf("write token response: %v", err)
			}
		}))
		defer nas.Close()

		sut := newTestAPIServer(t)
		errCh := make(chan error, 1)
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-shell-dial-fail"}, truenasShellTarget{
				BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
			})
		}))
		defer proxy.Close()

		wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
		browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("browser dial failed: %v", err)
		}
		defer browserWS.Close()

		_, payload, err := browserWS.ReadMessage()
		if err != nil {
			t.Fatalf("expected error text from proxy, read failed: %v", err)
		}
		if !strings.Contains(string(payload), "Failed to connect to TrueNAS shell") {
			t.Fatalf("unexpected shell dial failure payload: %q", string(payload))
		}

		select {
		case err := <-errCh:
			if err == nil || !strings.Contains(err.Error(), "truenas shell dial") {
				t.Fatalf("expected shell dial error, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for proxy error")
		}
	})

	t.Run("shell auth send failure", func(t *testing.T) {
		nas := newAuthServer(t, func(conn *websocket.Conn) {
			time.Sleep(50 * time.Millisecond)
		})
		defer nas.Close()

		sut := newTestAPIServer(t)
		errCh := make(chan error, 1)
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-auth-send-fail"}, truenasShellTarget{
				BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
				Options: map[string]any{"invalid": make(chan int)},
			})
		}))
		defer proxy.Close()

		wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
		browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("browser dial failed: %v", err)
		}
		defer browserWS.Close()

		select {
		case err := <-errCh:
			if err == nil || !strings.Contains(err.Error(), "truenas shell auth send") {
				t.Fatalf("expected shell auth send error, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for shell auth send error")
		}
	})

	t.Run("proxy goroutine write-failure branches", func(t *testing.T) {
		t.Run("browser to nas write failure", func(t *testing.T) {
			origWriteToNAS := truenaspkg.TruenasTerminalWriteToNAS
			truenaspkg.TruenasTerminalWriteToNAS = func(conn *websocket.Conn, msgType int, data []byte) error {
				return errors.New("forced nas write failure")
			}
			defer func() { truenaspkg.TruenasTerminalWriteToNAS = origWriteToNAS }()

			nas := newAuthServer(t, func(conn *websocket.Conn) {
				var authMsg map[string]any
				if err := conn.ReadJSON(&authMsg); err != nil {
					t.Fatalf("read shell auth message: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{"msg": "connected"}); err != nil {
					t.Fatalf("write shell connected response: %v", err)
				}
				time.Sleep(80 * time.Millisecond)
			})
			defer nas.Close()

			sut := newTestAPIServer(t)
			errCh := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-browser-write-fail"}, truenasShellTarget{
					BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
				})
			}))
			defer proxy.Close()

			wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
			browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("browser dial failed: %v", err)
			}
			defer browserWS.Close()

			time.Sleep(50 * time.Millisecond)
			_ = browserWS.WriteMessage(websocket.TextMessage, []byte("whoami\n"))

			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("expected nil error when proxy loop exits on write failure, got %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for proxy completion")
			}
		})

		t.Run("nas to browser write failure", func(t *testing.T) {
			origWriteToBrowser := truenaspkg.TruenasTerminalWriteToBrowser
			truenaspkg.TruenasTerminalWriteToBrowser = func(conn *websocket.Conn, msgType int, data []byte) error {
				return errors.New("forced browser write failure")
			}
			defer func() { truenaspkg.TruenasTerminalWriteToBrowser = origWriteToBrowser }()

			nas := newAuthServer(t, func(conn *websocket.Conn) {
				var authMsg map[string]any
				if err := conn.ReadJSON(&authMsg); err != nil {
					t.Fatalf("read shell auth message: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{"msg": "connected"}); err != nil {
					t.Fatalf("write shell connected response: %v", err)
				}
				time.Sleep(50 * time.Millisecond)
				_ = conn.WriteMessage(websocket.TextMessage, []byte("hello\r\n"))
			})
			defer nas.Close()

			sut := newTestAPIServer(t)
			errCh := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-nas-write-fail"}, truenasShellTarget{
					BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
				})
			}))
			defer proxy.Close()

			wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
			browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("browser dial failed: %v", err)
			}
			defer browserWS.Close()
			go func() {
				time.Sleep(80 * time.Millisecond)
				_ = browserWS.Close()
			}()

			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("expected nil error when proxy loop exits on browser write failure, got %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for proxy completion")
			}
		})
	})

	t.Run("shell auth read/failed responses", func(t *testing.T) {
		t.Run("auth read error", func(t *testing.T) {
			nas := newAuthServer(t, func(conn *websocket.Conn) {
				var authMsg map[string]any
				if err := conn.ReadJSON(&authMsg); err != nil {
					t.Fatalf("read shell auth message: %v", err)
				}
				_ = conn.Close()
			})
			defer nas.Close()

			sut := newTestAPIServer(t)
			errCh := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-auth-read-fail"}, truenasShellTarget{
					BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
				})
			}))
			defer proxy.Close()

			wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
			browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("browser dial failed: %v", err)
			}
			defer browserWS.Close()

			select {
			case err := <-errCh:
				if err == nil || !strings.Contains(err.Error(), "truenas shell auth read") {
					t.Fatalf("expected shell auth read error, got %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for shell auth read error")
			}
		})

		t.Run("auth failed response", func(t *testing.T) {
			nas := newAuthServer(t, func(conn *websocket.Conn) {
				var authMsg map[string]any
				if err := conn.ReadJSON(&authMsg); err != nil {
					t.Fatalf("read shell auth message: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{
					"msg":   "failed",
					"error": map[string]any{"reason": "denied"},
				}); err != nil {
					t.Fatalf("write shell auth failure: %v", err)
				}
			})
			defer nas.Close()

			sut := newTestAPIServer(t)
			errCh := make(chan error, 1)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errCh <- sut.tryTrueNASTerminalStream(w, r, terminal.Session{ID: "sess-auth-failed"}, truenasShellTarget{
					BaseURL: nas.URL, APIKey: "api-key", SkipVerify: true, Timeout: time.Second,
				})
			}))
			defer proxy.Close()

			wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
			browserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("browser dial failed: %v", err)
			}
			defer browserWS.Close()

			select {
			case err := <-errCh:
				if err == nil || !strings.Contains(err.Error(), "truenas shell auth failed") {
					t.Fatalf("expected shell auth failed error, got %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for shell auth failed error")
			}
		})
	})
}
