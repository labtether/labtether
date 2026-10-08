package main

import (
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleProxmoxDesktopStreamGuardsAndRuntimeFailure(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "node",
		Node: "pve01",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported desktop kind, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "qemu",
		Node: "pve01",
		VMID: "101",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when runtime is unavailable, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamVNCAuthFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to send RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed RFB version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
				t.Fatalf("failed to send invalid security types payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when VNC auth fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamProxyAndDialFailures(t *testing.T) {
	sut := newTestAPIServer(t)

	proxyFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vncproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer proxyFailure.Close()
	configureProxmoxStreamCollector(t, sut, proxyFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when vncproxy call fails, got %d", rec.Code)
	}

	dialFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncproxy" {
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
			return
		}
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/vncwebsocket" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("ws forbidden"))
			return
		}
		http.NotFound(w, r)
	}))
	defer dialFailure.Close()
	configureProxmoxStreamCollector(t, sut, dialFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req = httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when websocket dial fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamLXCProxyBranch(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/lxc/200/vncproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"vncproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "lxc",
		Node:        "pve01",
		VMID:        "200",
		CollectorID: "collector-proxmox-stream",
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when LXC vncproxy call fails, got %d", rec.Code)
	}
}

func TestHandleProxmoxDesktopStreamBrowserHandshakeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read security selection: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write auth result: %v", err)
			}

			// Browser-side handshake is expected to fail before bridge starts.
			_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			_, _, _ = conn.ReadMessage()
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleProxmoxDesktopStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial desktop websocket: %v", err)
	}

	// Read the initial version then close without replying to force
	// proxmoxpkg.SendBrowserVNCNoAuth to fail in the handler.
	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := browserConn.ReadMessage(); err != nil {
		t.Fatalf("failed to read initial browser handshake payload: %v", err)
	}
	_ = browserConn.Close()
}

func TestHandleProxmoxDesktopStreamWebSocketBridge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	upstreamInput := make(chan string, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			if !strings.Contains(r.Header.Get("Authorization"), "PVEAPIToken=labtether@pve!stream=stream-secret") {
				t.Fatalf("missing token auth header: %q", r.Header.Get("Authorization"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			_, echoedVersion, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if string(echoedVersion) != "RFB 003.008\n" {
				t.Fatalf("unexpected echoed version: %q", string(echoedVersion))
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			_, selectedType, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read selected security type: %v", err)
			}
			if len(selectedType) != 1 || selectedType[0] != 1 {
				t.Fatalf("unexpected selected security type payload: %v", selectedType)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write security result: %v", err)
			}

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read bridged browser payload: %v", err)
			}
			upstreamInput <- string(payload)

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("from-upstream")); err != nil {
				t.Fatalf("failed to write upstream payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handleProxmoxDesktopStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial desktop websocket: %v", err)
	}
	defer browserConn.Close()

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, version, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser RFB version: %v", err)
	}
	if string(version) != "RFB 003.008\n" {
		t.Fatalf("unexpected browser RFB version: %q", string(version))
	}
	if err := browserConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to send browser RFB version: %v", err)
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, securityTypes, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser security types: %v", err)
	}
	if len(securityTypes) != 2 || securityTypes[0] != 1 || securityTypes[1] != 1 {
		t.Fatalf("unexpected browser security types: %v", securityTypes)
	}
	if err := browserConn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
		t.Fatalf("failed to send browser security selection: %v", err)
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, authResult, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser auth result: %v", err)
	}
	if len(authResult) != 4 || authResult[0] != 0 || authResult[1] != 0 || authResult[2] != 0 || authResult[3] != 0 {
		t.Fatalf("unexpected browser auth result: %v", authResult)
	}

	if err := browserConn.WriteMessage(websocket.BinaryMessage, []byte("browser-frame")); err != nil {
		t.Fatalf("failed to send browser frame: %v", err)
	}

	select {
	case got := <-upstreamInput:
		if got != "browser-frame" {
			t.Fatalf("unexpected upstream payload: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for bridged browser payload")
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, upstreamPayload, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bridged upstream payload: %v", err)
	}
	if string(upstreamPayload) != "from-upstream" {
		t.Fatalf("unexpected bridged upstream payload: %q", string(upstreamPayload))
	}
}

func TestHandleProxmoxDesktopStreamBrowserUpgradeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/vncproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5901","ticket":"PVEVNC:desktop-ticket","user":"root@pam","password":"desktop-secret"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
				t.Fatalf("failed to write RFB version: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read echoed version: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
				t.Fatalf("failed to write security types: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read security selection: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
				t.Fatalf("failed to write security result: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/desktop", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxDesktopStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})

	// Upgrade happens after successful upstream auth; a plain HTTP request should
	// fail websocket upgrade and write a 400 response.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when browser upgrade fails post-auth, got %d", rec.Code)
	}
}
