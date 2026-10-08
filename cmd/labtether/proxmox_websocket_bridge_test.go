package main

import (
	"errors"
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"io"
	"testing"
	"time"
)

func TestBridgeWebSocketPairAdditionalLifecycleBranches(t *testing.T) {
	t.Run("ignore non binary/text and handle upstream write failure", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

		if err := browserClientConn.WriteMessage(websocket.PingMessage, []byte("ignored")); err != nil {
			t.Fatalf("failed to write browser ping message: %v", err)
		}

		_ = upstreamClientConn.Close()
		if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("forward-me")); err != nil {
			t.Fatalf("failed to write browser payload after upstream close: %v", err)
		}
	})

	t.Run("upstream close triggers done branch", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)

		if err := browserClientConn.WriteMessage(websocket.TextMessage, []byte("first-frame")); err != nil {
			t.Fatalf("failed to write first browser frame: %v", err)
		}
		_ = upstreamClientConn.Close()
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("browser write failure while forwarding upstream payload", func(t *testing.T) {
		browserServerConn, browserClientConn, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, upstreamClientConn, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		go proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
		_ = browserClientConn.Close()

		if err := upstreamClientConn.WriteMessage(websocket.BinaryMessage, []byte("upstream-data")); err != nil {
			t.Fatalf("failed to write upstream data: %v", err)
		}
	})
}

func TestBridgeWebSocketPairDeterministicRemainingBranches(t *testing.T) {
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

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})

	t.Run("done channel exits main loop", func(t *testing.T) {
		browserServerConn, _, browserCleanup := newWebSocketPair(t)
		defer browserCleanup()
		upstreamServerConn, _, upstreamCleanup := newWebSocketPair(t)
		defer upstreamCleanup()

		releaseUpstream := make(chan struct{})
		browserReads := 0
		originalWrite := proxmoxpkg.ProxmoxWSWriteMessage
		originalRead := proxmoxpkg.ProxmoxWSReadMessage
		withProxmoxStreamHooks(
			t,
			func(conn *websocket.Conn, messageType int, payload []byte) error {
				if conn == upstreamServerConn && messageType == websocket.TextMessage && string(payload) == "first-frame" {
					close(releaseUpstream)
					time.Sleep(20 * time.Millisecond)
					return nil
				}
				return originalWrite(conn, messageType, payload)
			},
			func(conn *websocket.Conn) (int, []byte, error) {
				if conn == upstreamServerConn {
					<-releaseUpstream
					return 0, nil, io.EOF
				}
				if conn == browserServerConn {
					browserReads++
					if browserReads == 1 {
						return websocket.TextMessage, []byte("first-frame"), nil
					}
					t.Fatalf("browser ReadMessage should not execute after done channel closes")
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
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

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})

	t.Run("upstream write failure", func(t *testing.T) {
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
				if conn == upstreamServerConn {
					return errors.New("forced upstream write failure")
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
						return websocket.TextMessage, []byte("forward-me"), nil
					}
					return 0, nil, io.EOF
				}
				return originalRead(conn)
			},
			nil,
			nil,
		)

		proxmoxpkg.BridgeWebSocketPair(browserServerConn, upstreamServerConn)
	})
}
