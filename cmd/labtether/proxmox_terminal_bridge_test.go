package main

import (
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"io"
	"testing"
	"time"
)

func TestBridgeWebSocketPairAndBridgeProxmoxTerminal(t *testing.T) {
	browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
	defer browserCleanup()
	upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
	defer upstreamCleanup()

	go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

	if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("hello-upstream")); err != nil {
		t.Fatalf("failed to write browser->upstream message: %v", err)
	}
	_ = upstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := upstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read upstream message: %v", err)
	}
	if string(payload) != "hello-upstream" {
		t.Fatalf("unexpected upstream payload: %q", string(payload))
	}

	if err := upstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("hello-browser")); err != nil {
		t.Fatalf("failed to write upstream->browser message: %v", err)
	}
	_ = browserClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = browserClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser message: %v", err)
	}
	if string(payload) != "hello-browser" {
		t.Fatalf("unexpected browser payload: %q", string(payload))
	}

	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	if err := bridgeUpstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("proxmox-output")); err != nil {
		t.Fatalf("failed to write proxmox output: %v", err)
	}
	_ = bridgeBrowserClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeBrowserClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read browser terminal output: %v", err)
	}
	if string(payload) != "proxmox-output" {
		t.Fatalf("unexpected proxmox output payload: %q", string(payload))
	}

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":100,"rows":40}`)); err != nil {
		t.Fatalf("failed to write resize control message: %v", err)
	}
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read translated resize message: %v", err)
	}
	if string(payload) != "1:100:40:" {
		t.Fatalf("unexpected translated resize payload: %q", string(payload))
	}

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"ls\n"}`)); err != nil {
		t.Fatalf("failed to write input control message: %v", err)
	}
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err = bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read translated input message: %v", err)
	}
	if string(payload) != "0:3:ls\n" {
		t.Fatalf("unexpected translated input payload: %q", string(payload))
	}
}

func TestBridgeProxmoxTerminalKeepalivePing(t *testing.T) {
	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	setProxmoxTerminalKeepaliveIntervalForTest(t, 20*time.Millisecond)

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, payload, err := bridgeUpstreamClientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read keepalive ping: %v", err)
	}
	if string(payload) != "2" {
		t.Fatalf("expected keepalive ping payload '2', got %q", string(payload))
	}

	_ = bridgeBrowserClientConn.Close()
}

func TestBridgeProxmoxTerminalSkipsNilTranslatedPayloads(t *testing.T) {
	bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
	defer bridgeBrowserCleanup()
	bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
	defer bridgeUpstreamCleanup()

	go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

	if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"unknown"}`)); err != nil {
		t.Fatalf("failed to write unknown control message: %v", err)
	}

	// Unknown control messages translate to nil and should not be forwarded upstream.
	_ = bridgeUpstreamClientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := bridgeUpstreamClientConn.ReadMessage(); err == nil {
		t.Fatalf("expected no upstream payload for unknown control message")
	}
}

func TestBridgeProxmoxTerminalAdditionalLifecycleBranches(t *testing.T) {
	t.Run("ignore non terminal message types and handle upstream write failure", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

		if err := bridgeBrowserClientConn.WriteMessage(websocket.PingMessage, []byte("keepalive")); err != nil {
			t.Fatalf("failed to write browser ping message: %v", err)
		}

		_ = bridgeUpstreamClientConn.Close()
		if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"ls\n"}`)); err != nil {
			t.Fatalf("failed to write browser input message: %v", err)
		}
	})

	t.Run("upstream close triggers done branch", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)

		if err := bridgeBrowserClientConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"input","data":"pwd\n"}`)); err != nil {
			t.Fatalf("failed to write browser input message: %v", err)
		}
		_ = bridgeUpstreamClientConn.Close()

		// Allow the bridge loop to observe `done` on its next iteration.
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("browser write failure while forwarding upstream output", func(t *testing.T) {
		bridgeBrowserServerConn, bridgeBrowserClientConn, bridgeBrowserCleanup := newWebSocketPair(t)
		defer bridgeBrowserCleanup()
		bridgeUpstreamServerConn, bridgeUpstreamClientConn, bridgeUpstreamCleanup := newWebSocketPair(t)
		defer bridgeUpstreamCleanup()

		go proxmoxpkg.BridgeProxmoxTerminal(bridgeBrowserServerConn, bridgeUpstreamServerConn)
		_ = bridgeBrowserClientConn.Close()

		if err := bridgeUpstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("upstream-payload")); err != nil {
			t.Fatalf("failed to write upstream payload: %v", err)
		}
	})
}

func TestBridgeProxmoxTerminalDeterministicRemainingBranches(t *testing.T) {
	t.Run("upstream to browser write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		upstreamReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == browserServerConn {
					return errors.New("forced browser write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					if upstreamReads == 0 {
						upstreamReads++
						return websocket.BinaryMessage, []byte("from-upstream"), nil
					}
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					time.Sleep(40 * time.Millisecond)
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("keepalive write failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		setProxmoxTerminalKeepaliveIntervalForTest(t, 10*time.Millisecond)

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.BinaryMessage && string(payload) == "2" {
					return errors.New("forced keepalive write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(60 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					time.Sleep(80 * time.Millisecond)
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("ignore non data frame", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			nil,
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.PingMessage, []byte("ignored"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})

	t.Run("browser input write to proxmox failure", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		browserReads := 0
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.BinaryMessage {
					return errors.New("forced proxmox write failure")
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					time.Sleep(100 * time.Millisecond)
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.TextMessage, []byte("ls\n"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeProxmoxTerminal(browserServerConn, upstreamServerConn)
	})
}
