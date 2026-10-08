package main

import (
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTryProxmoxTerminalStreamLoadRuntimeFailure(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()

	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind: "qemu",
		Node: "pve01",
		VMID: "101",
	})
	if err == nil || !strings.Contains(err.Error(), "load runtime") {
		t.Fatalf("expected load runtime error, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamAuthRejected(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			_, authPayload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read proxmox auth payload: %v", err)
			}
			if got := string(authPayload); got != "root@pam:PVEVNC:terminal-ticket\n" {
				t.Fatalf("unexpected proxmox auth payload: %q", got)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ERR")); err != nil {
				t.Fatalf("failed to write auth reject: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()

	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "auth rejected") {
		t.Fatalf("expected auth rejected error, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamTicketAndDialFailures(t *testing.T) {
	sut := newTestAPIServer(t)

	ticketFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/termproxy" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"termproxy":"failed"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ticketFailure.Close()
	configureProxmoxStreamCollector(t, sut, ticketFailure.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()
	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "open terminal ticket") {
		t.Fatalf("expected terminal ticket failure, got %v", err)
	}

	dialFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve01/qemu/101/termproxy" {
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
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

	req = httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec = httptest.NewRecorder()
	err = sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err == nil || !strings.Contains(err.Error(), "dial websocket") {
		t.Fatalf("expected websocket dial failure, got %v", err)
	}
}

func TestTryProxmoxTerminalStreamAuthTransportFailures(t *testing.T) {
	t.Run("auth read failure", func(t *testing.T) {
		upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read proxmox auth payload: %v", err)
				}
				_ = conn.Close()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "auth read") {
			t.Fatalf("expected auth-read failure, got %v", err)
		}
	})
}

func TestTryProxmoxTerminalStreamAuthWriteAndDeadlineFailures(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	t.Run("auth write failure", func(t *testing.T) {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				_, _, _ = conn.ReadMessage()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if messageType == websocket.TextMessage && strings.Contains(string(payload), "PVEVNC:terminal-ticket") {
					return errors.New("forced auth write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "send auth") {
			t.Fatalf("expected auth-write failure, got %v", err)
		}
	})

	t.Run("set read deadline failure", func(t *testing.T) {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/nodes/pve01/qemu/101/termproxy":
				_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
			case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("failed to upgrade proxmox websocket: %v", err)
				}
				defer conn.Close()
				_, _, _ = conn.ReadMessage()
			default:
				http.NotFound(w, r)
			}
		}))
		defer mock.Close()

		sut := newTestAPIServer(t)
		configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

		withProxmoxStreamHooks(t,
			nil,
			nil,
			func(*websocket.Conn, time.Time) error {
				return errors.New("forced deadline failure")
			},
			nil,
		)

		req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
		rec := httptest.NewRecorder()
		err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err == nil || !strings.Contains(err.Error(), "set read deadline") {
			t.Fatalf("expected read-deadline failure, got %v", err)
		}
	})
}

func TestTryProxmoxTerminalStreamWebSocketBridge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	proxmoxInput := make(chan string, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			if !strings.Contains(r.Header.Get("Authorization"), "PVEAPIToken=labtether@pve!stream=stream-secret") {
				t.Fatalf("missing token auth header: %q", r.Header.Get("Authorization"))
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			_, authPayload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read proxmox auth payload: %v", err)
			}
			if got := string(authPayload); got != "root@pam:PVEVNC:terminal-ticket\n" {
				t.Fatalf("unexpected proxmox auth payload: %q", got)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("OKready")); err != nil {
				t.Fatalf("failed to write auth OK payload: %v", err)
			}

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("failed to read bridged terminal payload: %v", err)
			}
			proxmoxInput <- string(payload)

			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("from-proxmox")); err != nil {
				t.Fatalf("failed to write proxmox output payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	handler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := sut.tryProxmoxTerminalStream(w, r, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
			Kind:        "qemu",
			Node:        "pve01",
			VMID:        "101",
			CollectorID: "collector-proxmox-stream",
		})
		if err != nil {
			t.Errorf("tryProxmoxTerminalStream returned error: %v", err)
		}
	}))
	defer handler.Close()

	wsURL := "ws" + strings.TrimPrefix(handler.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial browser websocket: %v", err)
	}
	defer browserConn.Close()

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read initial proxmox payload: %v", err)
	}
	if string(payload) != "ready" {
		t.Fatalf("unexpected initial proxmox payload: %q", string(payload))
	}

	if err := browserConn.WriteMessage(websocket.TextMessage, []byte("ls\n")); err != nil {
		t.Fatalf("failed to write browser terminal input: %v", err)
	}

	select {
	case got := <-proxmoxInput:
		if got != "0:3:ls\n" {
			t.Fatalf("unexpected proxmox terminal frame: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for proxmox terminal frame")
	}

	_ = browserConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = browserConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read bridged proxmox output: %v", err)
	}
	if string(payload) != "from-proxmox" {
		t.Fatalf("unexpected bridged proxmox output: %q", string(payload))
	}
}

func TestTryProxmoxTerminalStreamBrowserUpgradeFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/qemu/101/termproxy":
			_, _ = w.Write([]byte(`{"data":{"port":"5900","ticket":"PVEVNC:terminal-ticket","user":"root@pam"}}`))
		case "/api2/json/nodes/pve01/qemu/101/vncwebsocket":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("failed to upgrade proxmox websocket: %v", err)
			}
			defer conn.Close()

			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("failed to read auth payload: %v", err)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, []byte("OK")); err != nil {
				t.Fatalf("failed to write auth OK payload: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	configureProxmoxStreamCollector(t, sut, mock.URL, "collector-proxmox-stream", "cred-proxmox-stream")

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess/stream", nil)
	rec := httptest.NewRecorder()
	err := sut.tryProxmoxTerminalStream(rec, req, terminal.Session{ID: "sess"}, proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-proxmox-stream",
	})
	if err != nil {
		t.Fatalf("expected nil when browser websocket upgrade fails post-auth, got %v", err)
	}
}
