package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/terminal"
)

func TestTrueNASPendingShellAuthStopsOnSessionCancel(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	authReady, upstreamClosed := make(chan struct{}), make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if r.URL.Path == "/api/current" {
			for _, result := range []any{true, "synthetic-shell-token"} {
				var request map[string]any
				if conn.ReadJSON(&request) != nil {
					return
				}
				if conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result}) != nil {
					return
				}
			}
			return
		}
		if r.URL.Path != "/websocket/shell" {
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		close(authReady)
		// Never send the shell-auth reply.
		_, _, _ = conn.ReadMessage()
		close(upstreamClosed)
	}))
	defer nas.Close()
	sut := newTestAPIServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- sut.tryTrueNASTerminalStream(w, r.WithContext(ctx), terminal.Session{ID: "cancel-nas"}, truenasShellTarget{
			BaseURL: nas.URL, APIKey: "synthetic-api-key", Timeout: time.Second,
		})
	}))
	defer proxy.Close()
	browser, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	awaitConnectorSignal(t, authReady, "TrueNAS auth request")
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancelled auth failure")
		}
	case <-time.After(time.Second):
		t.Fatal("TrueNAS shell auth ignored session cancellation")
	}
	awaitConnectorSignal(t, upstreamClosed, "TrueNAS upstream closure")
	_ = browser.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := browser.ReadMessage(); err == nil {
		t.Fatal("browser remained open")
	}
}

func TestProxmoxPendingTerminalAuthStopsOnSessionCancel(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	authReady, upstreamClosed := make(chan struct{}), make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/termproxy") {
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"synthetic-ticket","user":"test@pam"}}`))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		close(authReady)
		_, _, _ = conn.ReadMessage()
		close(upstreamClosed)
	}))
	defer upstream.Close()
	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, upstream.URL, "collector-cancel", "credential-cancel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/test/stream", nil).WithContext(ctx)
	done := make(chan error, 1)
	go func() {
		done <- sut.tryProxmoxTerminalStream(httptest.NewRecorder(), req, terminal.Session{ID: "cancel-pve"}, proxmoxSessionTarget{
			Kind: "qemu", Node: "test-node", VMID: "101", CollectorID: "collector-cancel",
		})
	}()
	awaitConnectorSignal(t, authReady, "Proxmox auth request")
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancelled auth failure")
		}
	case <-time.After(time.Second):
		t.Fatal("Proxmox terminal auth ignored session cancellation")
	}
	awaitConnectorSignal(t, upstreamClosed, "Proxmox upstream closure")
}

func awaitConnectorSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
