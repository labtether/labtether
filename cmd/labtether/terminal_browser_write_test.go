package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// A failed browser output write must close the input reader and send the actual
// agent close, both when output arrives first and after the started marker.
func TestTerminalBrowserWriteFailureEndsAgentStream(t *testing.T) {
	for _, outputFirst := range []bool{true, false} {
		name := "started_first"
		if outputFirst {
			name = "output_first"
		}
		t.Run(name, func(t *testing.T) {
			s := newTestAPIServer(t)
			s.agentMgr = agentmgr.NewManager()
			agentWS, peer, cleanup := newWebSocketPair(t)
			defer cleanup()
			agent := agentmgr.NewAgentConn(agentWS, "node", "linux")
			agent.SetMeta("terminal.tmux.has", "false")
			s.agentMgr.Register(agent)
			defer s.agentMgr.Unregister(agent.AssetID)
			starts, stops := make(chan string, 1), make(chan string, 1)
			go func() {
				for {
					var msg agentmgr.Message
					if peer.ReadJSON(&msg) != nil {
						return
					}
					var payload struct {
						SessionID string `json:"session_id"`
					}
					if json.Unmarshal(msg.Data, &payload) != nil {
						continue
					}
					if msg.Type == agentmgr.MsgTerminalStart {
						starts <- payload.SessionID
						if !outputFirst {
							streamGenerationReply(s, agent, "terminal", "started", payload.SessionID, "")
						}
						streamGenerationReply(s, agent, "terminal", "data", payload.SessionID, "cannot reach browser")
					} else if msg.Type == agentmgr.MsgTerminalClose {
						stops <- payload.SessionID
					}
				}
			}()
			finished := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				s.handleAgentTerminalStream(binaryWriteFailureResponse{w}, r, terminal.Session{ID: "sess-write-failure", Target: agent.AssetID})
			}))
			defer server.Close()
			browser, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			wire := streamGenerationID(t, starts)
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("failed output writer left the browser input reader and agent stream active")
			}
			if stopped := streamGenerationID(t, stops); stopped != wire {
				t.Fatalf("close named %q, expected exact wire owner %q", stopped, wire)
			}
		})
	}
}

// Leave the real HTTP handshake, status frames and input socket working. Only
// browser binary frame writes fail, so closing input is the handler's duty.
type binaryWriteFailureResponse struct{ http.ResponseWriter }

func (w binaryWriteFailureResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.(http.Hijacker).Hijack()
	if err != nil {
		return conn, rw, err
	}
	return binaryWriteFailureConn{conn}, rw, nil
}

type binaryWriteFailureConn struct{ net.Conn }

func (c binaryWriteFailureConn) Write(data []byte) (int, error) {
	if len(data) > 0 && data[0]&0x80 != 0 && data[0]&0x0f == websocket.BinaryMessage {
		return 0, io.ErrClosedPipe
	}
	return c.Conn.Write(data)
}
