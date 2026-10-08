package main

import (
	"github.com/gorilla/websocket"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPerformProxmoxVNCAuthNone(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		version := []byte("RFB 003.008\n")
		if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to send version: %v", err)
		}
		_, echoedVersion, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read echoed version: %v", err)
		}
		if string(echoedVersion) != string(version) {
			t.Fatalf("unexpected echoed version: %q", string(echoedVersion))
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
			t.Fatalf("failed to send security types: %v", err)
		}
		_, selection, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read selection: %v", err)
		}
		if len(selection) != 1 || selection[0] != 1 {
			t.Fatalf("unexpected security selection: %v", selection)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
			t.Fatalf("failed to send auth result: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	version, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, "")
	if err != nil {
		t.Fatalf("proxmoxpkg.PerformProxmoxVNCAuth failed: %v", err)
	}
	if version != "RFB 003.008" {
		t.Fatalf("unexpected RFB version: %s", version)
	}
}

func TestPerformProxmoxVNCAuthVNCAuthChallenge(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	password := "s3cret!"
	challenge := []byte("0123456789ABCDEF")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		version := []byte("RFB 003.008\n")
		if err := conn.WriteMessage(websocket.BinaryMessage, version); err != nil {
			t.Fatalf("failed to send version: %v", err)
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("failed to read echoed version: %v", err)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
			t.Fatalf("failed to send VNC auth security type: %v", err)
		}
		_, selection, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read security selection: %v", err)
		}
		if len(selection) != 1 || selection[0] != 2 {
			t.Fatalf("unexpected VNC auth selection: %v", selection)
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, challenge); err != nil {
			t.Fatalf("failed to send challenge: %v", err)
		}
		_, response, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read challenge response: %v", err)
		}
		expected := proxmoxpkg.VNCEncryptChallenge(challenge, password)
		if string(response) != string(expected) {
			t.Fatalf("unexpected challenge response")
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 0}); err != nil {
			t.Fatalf("failed to send auth result: %v", err)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	if _, err := proxmoxpkg.PerformProxmoxVNCAuth(conn, password); err != nil {
		t.Fatalf("proxmoxpkg.PerformProxmoxVNCAuth VNC auth branch failed: %v", err)
	}
}

func TestPerformProxmoxVNCAuthFailureBranches(t *testing.T) {
	type scenario struct {
		name        string
		handler     func(t *testing.T, conn *websocket.Conn)
		expectError string
	}

	scenarios := []scenario{
		{
			name: "empty-security-message",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{}); err != nil {
					t.Fatalf("failed to write empty security message: %v", err)
				}
			},
			expectError: "empty security message",
		},
		{
			name: "incomplete-security-types",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{2, 1}); err != nil {
					t.Fatalf("failed to write incomplete security types: %v", err)
				}
			},
			expectError: "incomplete security types",
		},
		{
			name: "zero-security-types",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
					t.Fatalf("failed to write zero security types: %v", err)
				}
			},
			expectError: "0 security types",
		},
		{
			name: "no-supported-security-type",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 3}); err != nil {
					t.Fatalf("failed to write unsupported security types: %v", err)
				}
			},
			expectError: "no supported security type",
		},
		{
			name: "none-auth-rejected",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
					t.Fatalf("failed to write None security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0, 1}); err != nil {
					t.Fatalf("failed to write rejected auth result: %v", err)
				}
			},
			expectError: "none auth unexpectedly rejected",
		},
		{
			name: "short-vnc-challenge",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC auth security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("short")); err != nil {
					t.Fatalf("failed to write short challenge: %v", err)
				}
			},
			expectError: "unexpected challenge length",
		},
		{
			name: "short-auth-result",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")); err != nil {
					t.Fatalf("failed to write RFB version: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read echoed version: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2}); err != nil {
					t.Fatalf("failed to write VNC auth security type: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read selected security type: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte("0123456789ABCDEF")); err != nil {
					t.Fatalf("failed to write challenge: %v", err)
				}
				if _, _, err := conn.ReadMessage(); err != nil {
					t.Fatalf("failed to read challenge response: %v", err)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 0, 0}); err != nil {
					t.Fatalf("failed to write short auth result: %v", err)
				}
			},
			expectError: "short auth result",
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
