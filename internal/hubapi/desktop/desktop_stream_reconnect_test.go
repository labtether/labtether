package desktop

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

func desktopTestSocket(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readDesktopWire(t *testing.T, conn *websocket.Conn) agentmgr.Message {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var message agentmgr.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(message.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SessionID == "" || payload.SessionID != message.ID {
		t.Fatalf("wire identity mismatch: %+v", message)
	}
	return message
}

// Drive the real browser handlers so start, input, reply and finalizer IDs must
// all agree across an overlapping reconnect of one public session.
func TestDesktopReconnectRoutesOnlyItsOwnWireStream(t *testing.T) {
	for _, kind := range []string{"vnc", "webrtc"} {
		t.Run(kind, func(t *testing.T) {
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			accepted := make(chan *websocket.Conn, 1)
			agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				socket, err := upgrader.Upgrade(w, r, nil)
				if err == nil {
					accepted <- socket
				}
			}))
			defer agentServer.Close()
			agentPeer := desktopTestSocket(t, agentServer.URL)
			var agentSocket *websocket.Conn
			select {
			case agentSocket = <-accepted:
			case <-time.After(time.Second):
				t.Fatal("agent did not connect")
			}
			conn := agentmgr.NewAgentConn(agentSocket, "node", "linux")
			defer conn.Close()
			manager := agentmgr.NewManager()
			manager.Register(conn)
			var desktops, webrtcs sync.Map
			d := &Deps{
				DesktopBridges: &desktops, WebRTCBridges: &webrtcs, AgentMgr: manager,
				TerminalWebSocketUpgrader: &upgrader, MaxDesktopInputReadBytes: 1024,
				CheckSameOrigin:            upgrader.CheckOrigin,
				BrowserStreamTraceID:       func(*http.Request) string { return "trace" },
				StreamTraceLogValue:        func(v string) string { return v },
				SanitizeAgentStreamReason:  func(v string) string { return v },
				StartBrowserWSKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
				TouchBrowserWSReadDeadline: func(*websocket.Conn) error { return nil },
			}
			defer d.CloseDesktopStreamsForConnection(conn)
			session := terminal.Session{ID: "public", Target: "node"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "webrtc" {
					d.HandleWebRTCStream(w, r, session)
				} else {
					d.HandleAgentDesktopStream(w, r, session)
				}
			}))
			defer server.Close()
			first := desktopTestSocket(t, server.URL)
			firstStart := readDesktopWire(t, agentPeer)
			startType, stopType := agentmgr.MsgDesktopStart, agentmgr.MsgDesktopClose
			if kind == "webrtc" {
				startType, stopType = agentmgr.MsgWebRTCStart, agentmgr.MsgWebRTCStop
			}
			if firstStart.Type != startType || firstStart.ID == session.ID {
				t.Fatalf("bad first start: %+v", firstStart)
			}
			if kind == "vnc" {
				d.ProcessAgentDesktopStarted(conn, desktopWireMessage(t, agentmgr.DesktopStartedData{SessionID: firstStart.ID}))
			}
			second := desktopTestSocket(t, server.URL)
			var currentID string
			oldStopped := false
			for i := 0; i < 2; i++ {
				message := readDesktopWire(t, agentPeer)
				switch message.Type {
				case startType:
					currentID = message.ID
				case stopType:
					oldStopped = message.ID == firstStart.ID
				default:
					t.Fatalf("unexpected reconnect message: %+v", message)
				}
			}
			if !oldStopped || currentID == "" || currentID == firstStart.ID || currentID == session.ID {
				t.Fatal("reconnect reused identity or stopped wrong stream")
			}
			_ = first.Close()
			wrongConn := &agentmgr.AgentConn{AssetID: "node"}
			d.CloseDesktopStreamsForConnection(wrongConn)
			if kind == "vnc" {
				d.ProcessAgentDesktopClosed(conn, desktopWireMessage(t, agentmgr.DesktopCloseData{SessionID: firstStart.ID}))
				d.ProcessAgentDesktopData(conn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: firstStart.ID, Data: "b2xk"}))
				d.ProcessAgentDesktopClosed(wrongConn, desktopWireMessage(t, agentmgr.DesktopCloseData{SessionID: currentID}))
				d.ProcessAgentDesktopData(wrongConn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: currentID, Data: "YmFk"}))
				d.ProcessAgentDesktopStarted(conn, desktopWireMessage(t, agentmgr.DesktopStartedData{SessionID: currentID}))
				d.ProcessAgentDesktopData(conn, desktopWireMessage(t, agentmgr.DesktopDataPayload{SessionID: currentID, Data: "bmV3"}))
				_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
				messageType, data, err := second.ReadMessage()
				if err != nil || messageType != websocket.BinaryMessage || string(data) != "new" {
					t.Fatalf("replacement output: %q %v", data, err)
				}
				// RFB resize/control bytes share the same opaque binary input path.
				input := []byte{251, 0, 3, 32, 2, 88, 1, 0}
				if err := second.WriteMessage(websocket.BinaryMessage, input); err != nil {
					t.Fatal(err)
				}
				message := readDesktopWire(t, agentPeer)
				var payload agentmgr.DesktopDataPayload
				if err := json.Unmarshal(message.Data, &payload); err != nil {
					t.Fatal(err)
				}
				decoded, err := base64.StdEncoding.DecodeString(payload.Data)
				if err != nil || message.Type != agentmgr.MsgDesktopData || message.ID != currentID || string(decoded) != string(input) {
					t.Fatal("input did not stay on replacement wire")
				}
			} else {
				d.ProcessAgentWebRTCStopped(conn, desktopWireMessage(t, agentmgr.WebRTCStoppedData{SessionID: firstStart.ID}))
				d.ProcessAgentWebRTCStopped(wrongConn, desktopWireMessage(t, agentmgr.WebRTCStoppedData{SessionID: currentID}))
				d.ProcessAgentWebRTCStarted(conn, desktopWireMessage(t, agentmgr.WebRTCStartedData{SessionID: currentID}))
				_ = second.SetReadDeadline(time.Now().Add(2 * time.Second))
				var ready struct {
					Type string                     `json:"type"`
					Data agentmgr.WebRTCStartedData `json:"data"`
				}
				if err := second.ReadJSON(&ready); err != nil {
					t.Fatal(err)
				}
				if ready.Type != "ready" || ready.Data.SessionID != session.ID {
					t.Fatalf("browser lost public identity: %+v", ready)
				}
				if err := second.WriteJSON(map[string]any{"type": "input", "data": map[string]any{"type": "mousemove", "session_id": firstStart.ID, "x": 12}}); err != nil {
					t.Fatal(err)
				}
				message := readDesktopWire(t, agentPeer)
				if message.Type != agentmgr.MsgWebRTCInput || message.ID != currentID {
					t.Fatal("input did not stay on replacement wire")
				}
			}
			_ = second.Close()
			stop := readDesktopWire(t, agentPeer)
			if stop.Type != stopType || stop.ID != currentID {
				t.Fatalf("wrong final stop: %+v", stop)
			}
		})
	}
}
