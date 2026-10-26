package dockerpkg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
)

// Browser events must carry the logical session ID, only from the exact socket
// and only while this stream still owns the public session.
func TestDockerExecBroadcastUsesLogicalOwnerOnly(t *testing.T) {
	conn := &agentmgr.AgentConn{AssetID: "node"}
	bridge := &DockerExecBridge{AgentConn: conn, WireSessionID: "wire", SessionID: "logical", ExpectedAgentID: "node", OutputCh: make(chan []byte, 4), ClosedCh: make(chan struct{})}
	var broadcastID string
	deps := &Deps{DockerExecBridges: &sync.Map{}, Broadcast: func(_ string, data json.RawMessage) {
		var v struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(data, &v)
		broadcastID = v.SessionID
	}}
	deps.DockerExecBridges.Store("wire", bridge)
	deps.dockerExecOwners.Store("logical", bridge)
	data, _ := json.Marshal(agentmgr.DockerExecDataPayload{SessionID: "wire", Data: "eA=="})
	msg := agentmgr.Message{Type: agentmgr.MsgDockerExecData, Data: data}
	deps.ProcessAgentDockerExecDataMessage(&agentmgr.AgentConn{AssetID: "node"}, msg)
	if len(bridge.OutputCh) != 0 || broadcastID != "" {
		t.Fatal("same-asset old connection reached current stream")
	}
	deps.ProcessAgentDockerExecDataMessage(conn, msg)
	if broadcastID != "logical" {
		t.Fatalf("browser event used wire ID: %s", broadcastID)
	}
	broadcastID = ""
	deps.dockerExecOwners.Store("logical", &DockerExecBridge{SessionID: "logical"})
	deps.ProcessAgentDockerExecDataMessage(conn, msg)
	if broadcastID != "" {
		t.Fatalf("superseded stream published a public event: %s", broadcastID)
	}
}

func newDockerLifecycleSocket(t *testing.T) (*agentmgr.AgentConn, *websocket.Conn) {
	t.Helper()
	sockets := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		ws, err := up.Upgrade(w, r, nil)
		if err == nil {
			sockets <- ws
		}
	}))
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	conn := agentmgr.NewAgentConn(<-sockets, "docker-node", "linux")
	t.Cleanup(func() { conn.Close(); _ = peer.Close(); server.Close() })
	return conn, peer
}

func readDockerLifecycleMessage(t *testing.T, peer *websocket.Conn, want string) agentmgr.Message {
	t.Helper()
	_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 4; i++ {
		var msg agentmgr.Message
		if err := peer.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == want {
			return msg
		}
		if msg.Type != agentmgr.MsgDockerExecClose {
			t.Fatalf("unexpected agent message: %s", msg.Type)
		}
	}
	t.Fatalf("missing agent message: %s", want)
	return agentmgr.Message{}
}
