package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// Stream helpers whose tests moved to internal/hubapi/terminal, kept here
// verbatim for the cmd tests that still use them
// (session_stream_delete_test.go, desktop_stream_handlers_test.go and the
// terminal_persistent_* tests). The moved copies are in
// internal/hubapi/terminal/agent_stream_*_test.go and
// terminal_stream_lifecycle_test.go.

// The old request already captured its agent before Hijack. Pausing here makes
// the retirement/publication race deterministic without a production test hook.
type pausedStreamUpgrade struct {
	http.ResponseWriter
	entered chan<- struct{}
	resume  <-chan struct{}
}

func (w pausedStreamUpgrade) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	close(w.entered)
	<-w.resume
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func streamGenerationID(t *testing.T, ids <-chan string) string {
	t.Helper()
	select {
	case id := <-ids:
		return id
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for actual agent frame")
		return ""
	}
}

func streamGenerationReply(s *apiServer, conn *agentmgr.AgentConn, kind, event, wireID, value string) {
	data := map[string]any{"session_id": wireID, "reason": value, "tmux_attached": true}
	if event == "data" {
		data["data"] = base64.StdEncoding.EncodeToString([]byte(value))
		data["sdp"] = value
		data["type"] = "answer"
	}
	raw, _ := json.Marshal(data)
	msg := agentmgr.Message{Data: raw}
	switch kind + ":" + event {
	case "terminal:started":
		s.processAgentTerminalStarted(conn, msg)
	case "terminal:closed":
		s.processAgentTerminalClosed(conn, msg)
	case "terminal:data":
		s.processAgentTerminalData(conn, msg)
	case "desktop:started":
		s.ensureDesktopDeps().ProcessAgentDesktopStarted(conn, msg)
	case "desktop:closed":
		s.ensureDesktopDeps().ProcessAgentDesktopClosed(conn, msg)
	case "desktop:data":
		s.ensureDesktopDeps().ProcessAgentDesktopData(conn, msg)
	case "webrtc:started":
		s.ensureDesktopDeps().ProcessAgentWebRTCStarted(conn, msg)
	case "webrtc:closed":
		s.ensureDesktopDeps().ProcessAgentWebRTCStopped(conn, msg)
	case "webrtc:data":
		s.ensureDesktopDeps().ProcessAgentWebRTCAnswer(conn, msg)
	}
}

func streamGenerationBrowserPayload(t *testing.T, conn *websocket.Conn, kind, publicID string) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		typ, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if kind != "webrtc" && typ == websocket.BinaryMessage {
			return string(payload)
		}
		var signal struct {
			Type string `json:"type"`
			Data struct {
				SessionID string `json:"session_id"`
				SDP       string `json:"sdp"`
			} `json:"data"`
		}
		if json.Unmarshal(payload, &signal) == nil && signal.Type == "answer" {
			if signal.Data.SessionID != publicID {
				t.Fatalf("browser saw wire ID %q", signal.Data.SessionID)
			}
			return signal.Data.SDP
		}
	}
}

func newTerminalBridgeWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket failed: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		server.Close()
		t.Fatal("timed out waiting for server websocket connection")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}

	return serverConn, clientConn, cleanup
}

func waitForAgentTerminalMessage(t *testing.T, conn *websocket.Conn, wantType string) agentmgr.Message {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read agent websocket message: %v", err)
	}
	if msg.Type != wantType {
		t.Fatalf("agent message type=%q, want %q", msg.Type, wantType)
	}
	return msg
}

// newMemoryTerminalBookmarkStore: the store moved to testutil.
func newMemoryTerminalBookmarkStore() *testutil.MemoryTerminalBookmarkStore {
	return testutil.NewMemoryTerminalBookmarkStore()
}
