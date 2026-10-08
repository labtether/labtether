package main

import (
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendBrowserVNCNoAuth(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		if err := proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008"); err != nil {
			t.Fatalf("proxmoxpkg.SendBrowserVNCNoAuth failed: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	_, version, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}
	if string(version) != "RFB 003.008\n" {
		t.Fatalf("unexpected version payload: %q", string(version))
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to write echoed version: %v", err)
	}

	_, securityTypes, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read security types: %v", err)
	}
	if len(securityTypes) != 2 || securityTypes[0] != 1 || securityTypes[1] != 1 {
		t.Fatalf("unexpected security types payload: %v", securityTypes)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
		t.Fatalf("failed to write security selection: %v", err)
	}

	_, authResult, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read auth result: %v", err)
	}
	if len(authResult) != 4 || authResult[0] != 0 || authResult[1] != 0 || authResult[2] != 0 || authResult[3] != 0 {
		t.Fatalf("unexpected auth result payload: %v", authResult)
	}
}

func TestSendBrowserVNCNoAuthFailureBranches(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	errCh := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()
		errCh <- proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008")
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}

	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("failed to read initial RFB version: %v", err)
	}
	_ = conn.Close()

	select {
	case sendErr := <-errCh:
		if sendErr == nil || !strings.Contains(sendErr.Error(), "read browser RFB version") {
			t.Fatalf("expected browser version read failure, got %v", sendErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for proxmoxpkg.SendBrowserVNCNoAuth error")
	}

	errCh = make(chan error, 1)
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()
		errCh <- proxmoxpkg.SendBrowserVNCNoAuth(conn, "RFB 003.008")
	}))
	defer secondServer.Close()

	wsURL = "ws" + strings.TrimPrefix(secondServer.URL, "http")
	conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial second websocket: %v", err)
	}
	_, version, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read second initial RFB version: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
		t.Fatalf("failed to echo browser version: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("failed to read security types: %v", err)
	}
	_ = conn.Close()

	select {
	case sendErr := <-errCh:
		if sendErr == nil || !strings.Contains(sendErr.Error(), "read browser security selection") {
			t.Fatalf("expected security selection read failure, got %v", sendErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for security selection failure")
	}
}

func TestSendBrowserVNCNoAuthWriteFailureBranches(t *testing.T) {
	t.Run("write RFB version failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()
		_ = clientConn.Close()
		_ = serverConn.Close()

		if err := proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008"); err == nil || !strings.Contains(err.Error(), "send RFB version") {
			t.Fatalf("expected send RFB version write error, got %v", err)
		}
	})

	t.Run("write security types failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo version: %v", err)
		}
		_ = clientConn.Close()

		select {
		case sendErr := <-errCh:
			if sendErr == nil {
				t.Fatalf("expected handshake failure after browser close")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send security types failure")
		}
	})
}

func TestSendBrowserVNCNoAuthDeterministicWriteFailures(t *testing.T) {
	t.Run("send security types failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 2 && payload[0] == 1 && payload[1] == 1 {
					return errors.New("forced security types write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read browser version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo browser version: %v", err)
		}

		select {
		case sendErr := <-errCh:
			if sendErr == nil || !strings.Contains(sendErr.Error(), "send security types") {
				t.Fatalf("expected send security types failure, got %v", sendErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send security types failure")
		}
	})

	t.Run("send auth result failure", func(t *testing.T) {
		serverConn, clientConn, cleanup := newWebSocketPair(t)
		defer cleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		withProxmoxStreamHooks(t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == serverConn && messageType == websocket.BinaryMessage && len(payload) == 4 &&
					payload[0] == 0 && payload[1] == 0 && payload[2] == 0 && payload[3] == 0 {
					return errors.New("forced auth-result write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			nil,
			nil,
			nil,
		)

		errCh := make(chan error, 1)
		go func() {
			errCh <- proxmoxpkg.SendBrowserVNCNoAuth(serverConn, "RFB 003.008")
		}()

		_, version, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read browser version: %v", err)
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to echo browser version: %v", err)
		}

		_, securityTypes, err := clientConn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read security types: %v", err)
		}
		if len(securityTypes) != 2 {
			t.Fatalf("expected security types payload")
		}
		if err := clientConn.WriteMessage(websocket.BinaryMessage, []byte{1}); err != nil {
			t.Fatalf("failed to write security selection: %v", err)
		}

		select {
		case sendErr := <-errCh:
			if sendErr == nil || !strings.Contains(sendErr.Error(), "send auth result") {
				t.Fatalf("expected send auth result failure, got %v", sendErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for send auth result failure")
		}
	})
}
