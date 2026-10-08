package main

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
