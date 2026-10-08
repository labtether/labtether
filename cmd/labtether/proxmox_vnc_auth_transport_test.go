package main

import (
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPerformProxmoxVNCAuthTransportAndWriteBranches(t *testing.T) {
	type scenario struct {
		name        string
		handler     func(t *testing.T, conn *websocket.Conn)
		expectError string
	}

	scenarios := []scenario{
		{
			name: "read-rfb-version-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				_ = conn.Close()
			},
			expectError: "read RFB version",
		},
		{
			name: "read-security-types-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read security types",
		},
		{
			name: "read-none-auth-result-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
					t.Fatalf("failed to write security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read None auth selection: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read None auth result",
		},
		{
			name: "read-vnc-challenge-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read VNC challenge",
		},
		{
			name: "read-vnc-auth-result-failure",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth response: %v", err)
				}
				_ = conn.Close()
			},
			expectError: "read VNC auth result",
		},
		{
			name: "vnc-auth-result-rejected",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed RFB version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC security types: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth selection: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read VNC auth response: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 1}); err != nil {
					t.Fatalf("failed to write rejected VNC auth result: %v", err)
				}
			},
			expectError: "VNC authentication failed",
		},
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("upgrade failed: %v", err)
				}
				defer conn.Close()
				tc.handler(t, conn)
			}))
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("failed to dial websocket: %v", err)
			}
			defer conn.Close()

			if _, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, "secret"); err == nil || !strings.Contains(err.Error(), tc.expectError) {
				t.Fatalf("expected %q error, got %v", tc.expectError, err)
			}
		})
	}
}

func TestPerformProxmoxVNCAuthExplicitWriteFailures(t *testing.T) {
	t.Run("send RFB version failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && strings.HasPrefix(string(payload), "RFB ") {
					return errors.New("forced version write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send RFB version") {
			t.Fatalf("expected send RFB version failure, got %v", err)
		}
	})

	t.Run("send None selection failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 1 && payload[0] == 1 {
					return errors.New("forced none selection write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 1})
			_, _, _ = clientConn.ReadMessage()
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send None selection") {
			t.Fatalf("expected send None selection failure, got %v", err)
		}
	})

	t.Run("send VNC Auth selection failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 1 && payload[0] == 2 {
					return errors.New("forced vnc selection write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2})
			_, _, _ = clientConn.ReadMessage()
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send VNC Auth selection") {
			t.Fatalf("expected send VNC Auth selection failure, got %v", err)
		}
	})

	t.Run("send VNC auth response failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 16 {
					return errors.New("forced vnc response write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		go func() {
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n"))
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte{1, 2})
			_, _, _ = clientConn.ReadMessage()
			_ = clientConn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF"))
		}()

		if _, err := proxmoxpkg.PerformProxmoxVNCAuth(serverConn, "secret"); err == nil || !strings.Contains(err.Error(), "send VNC auth response") {
			t.Fatalf("expected send VNC auth response failure, got %v", err)
		}
	})
}
