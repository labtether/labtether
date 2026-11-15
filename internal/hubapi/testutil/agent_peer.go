package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
)

// AgentPeer is the agent end of a real WebSocket for handler tests. Conn is
// the Hub side, registered in Manager under the asset ID; Next returns each
// message the Hub sends to the agent, in order.
type AgentPeer struct {
	Manager *agentmgr.AgentManager
	Conn    *agentmgr.AgentConn
	sent    chan agentmgr.Message
}

// NewAgentPeer connects an agent for assetID. Everything is closed when the
// test ends.
func NewAgentPeer(t testing.TB, assetID string) *AgentPeer {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ws, err := upgrader.Upgrade(w, r, nil); err == nil {
			accepted <- ws
		}
	}))
	t.Cleanup(server.Close)
	agentSide, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial agent socket: %v", err)
	}
	t.Cleanup(func() { _ = agentSide.Close() })
	var hubSide *websocket.Conn
	select {
	case hubSide = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("agent socket was not accepted")
	}
	peer := &AgentPeer{
		Manager: agentmgr.NewManager(),
		Conn:    agentmgr.NewAgentConn(hubSide, assetID, "linux"),
		sent:    make(chan agentmgr.Message, 16),
	}
	peer.Manager.Register(peer.Conn)
	t.Cleanup(peer.Conn.Close)
	go func() {
		defer close(peer.sent)
		for {
			var msg agentmgr.Message
			if agentSide.ReadJSON(&msg) != nil {
				return
			}
			peer.sent <- msg
		}
	}()
	return peer
}

// Next waits up to two seconds for the next message the Hub sends.
func (p *AgentPeer) Next(t testing.TB) agentmgr.Message {
	t.Helper()
	select {
	case msg, ok := <-p.sent:
		if !ok {
			t.Fatal("agent socket closed before the Hub sent a message")
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("the Hub sent nothing to the agent")
	}
	return agentmgr.Message{}
}
