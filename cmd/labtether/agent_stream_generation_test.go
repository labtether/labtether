package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/terminal"
)

// A second browser must survive the first browser's finalizer and late agent
// replies, even when both use the same public session and agent asset.
func TestAgentBrowserStreamReplacementUsesDistinctWireSession(t *testing.T) {
	for _, mode := range []string{"terminal", "persistent", "desktop", "webrtc"} {
		t.Run(mode, func(t *testing.T) {
			kind := mode
			if kind == "persistent" {
				kind = "terminal"
			}
			s := newTestAPIServer(t)
			s.agentMgr = agentmgr.NewManager()
			serverWS, peerWS, cleanup := newWebSocketPair(t)
			defer cleanup()
			agent := agentmgr.NewAgentConn(serverWS, "node", "linux")
			agent.SetMeta("terminal.tmux.has", "false")
			s.agentMgr.Register(agent)
			defer s.agentMgr.Unregister("node")
			public := terminal.Session{ID: "sess-reused", Target: "node"}
			if mode == "persistent" {
				persistent, err := s.terminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: public.Target, Title: "Saved shell"})
				if err != nil {
					t.Fatal(err)
				}
				public.PersistentSessionID = persistent.ID
				public.TmuxSessionName = persistent.TmuxSessionName
				agent.SetMeta("terminal.tmux.has", "true")
			}
			starts, closes := make(chan string, 4), make(chan string, 4)
			audioStarts, audioCloses := make(chan string, 2), make(chan string, 2)
			finished := make(chan struct{}, 4)
			audioFinished := make(chan struct{}, 2)
			go func() {
				for {
					var msg agentmgr.Message
					if peerWS.ReadJSON(&msg) != nil {
						return
					}
					var data struct {
						SessionID string `json:"session_id"`
					}
					if json.Unmarshal(msg.Data, &data) != nil {
						continue
					}
					switch msg.Type {
					case agentmgr.MsgTerminalStart, agentmgr.MsgDesktopStart, agentmgr.MsgWebRTCStart:
						streamGenerationReply(s, agent, kind, "started", data.SessionID, "")
						starts <- data.SessionID
					case agentmgr.MsgTerminalClose, agentmgr.MsgDesktopClose, agentmgr.MsgWebRTCStop:
						closes <- data.SessionID
					case agentmgr.MsgDesktopAudioStart:
						audioStarts <- data.SessionID
					case agentmgr.MsgDesktopAudioStop:
						audioCloses <- data.SessionID
					}
				}
			}()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/audio" {
					defer func() { audioFinished <- struct{}{} }()
					s.handleDesktopAudioStream(w, r, public)
					return
				}
				defer func() { finished <- struct{}{} }()
				switch kind {
				case "terminal":
					s.handleAgentTerminalStream(w, r, public)
				case "desktop":
					s.handleAgentDesktopStream(w, r, public)
				case "webrtc":
					s.handleWebRTCStream(w, r, public)
				}
			}))
			defer server.Close()
			dial := func() *websocket.Conn {
				t.Helper()
				ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
				if err != nil {
					t.Fatal(err)
				}
				return ws
			}
			oldBrowser := dial()
			defer oldBrowser.Close()
			oldWire := streamGenerationID(t, starts)
			newBrowser := dial()
			defer newBrowser.Close()
			newWire := streamGenerationID(t, starts)
			if oldWire == "" || newWire == "" || oldWire == newWire || oldWire == public.ID || newWire == public.ID {
				t.Fatalf("wire owners were reused: old=%q new=%q public=%q", oldWire, newWire, public.ID)
			}
			if stopped := streamGenerationID(t, closes); stopped != oldWire {
				t.Fatalf("old browser stopped %q instead of its wire owner %q", stopped, oldWire)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("old browser finalizer did not complete")
			}
			if mode == "persistent" {
				stored, ok, err := s.terminalPersistentStore.GetPersistentSession(public.PersistentSessionID)
				if err != nil || !ok || stored.Status != "attached" {
					t.Fatalf("old browser detached replacement: %+v err=%v", stored, err)
				}
			}
			streamGenerationReply(s, agent, kind, "closed", oldWire, "late close")
			wrongSocket := &agentmgr.AgentConn{AssetID: agent.AssetID}
			streamGenerationReply(s, wrongSocket, kind, "data", newWire, "wrong socket")
			streamGenerationReply(s, agent, kind, "data", public.ID, "public ID is not a wire owner")
			streamGenerationReply(s, agent, kind, "data", newWire, "current stream")
			if got := streamGenerationBrowserPayload(t, newBrowser, kind, public.ID); got != "current stream" {
				t.Fatalf("replacement payload=%q", got)
			}
			var secondAudioWire string
			if kind == "desktop" {
				audio, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/audio", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer audio.Close()
				audioWire := streamGenerationID(t, audioStarts)
				if audioWire == "" || audioWire == newWire || audioWire == public.ID {
					t.Fatalf("audio reused desktop owner %q", audioWire)
				}
				raw, _ := json.Marshal(agentmgr.DesktopAudioStateData{SessionID: audioWire})
				s.processAgentDesktopAudioState(wrongSocket, agentmgr.Message{Data: raw})
				s.processAgentDesktopAudioState(agent, agentmgr.Message{Data: raw})
				_ = audio.SetReadDeadline(time.Now().Add(time.Second))
				var state agentmgr.DesktopAudioStateData
				if err := audio.ReadJSON(&state); err != nil {
					t.Fatal(err)
				}
				if state.SessionID != public.ID {
					t.Fatalf("audio browser saw wire ID %q", state.SessionID)
				}
				_ = audio.Close()
				if wire := streamGenerationID(t, audioCloses); wire != audioWire {
					t.Fatalf("audio stop used %q", wire)
				}
				select {
				case <-audioFinished:
				case <-time.After(time.Second):
					t.Fatal("first audio attachment did not release ownership")
				}
				secondAudio, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/audio", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer secondAudio.Close()
				secondAudioWire = streamGenerationID(t, audioStarts)
				if secondAudioWire == audioWire || secondAudioWire == newWire || secondAudioWire == public.ID {
					t.Fatalf("second audio attachment reused owner %q", secondAudioWire)
				}
				late, _ := json.Marshal(agentmgr.DesktopAudioStateData{SessionID: audioWire, State: "stopped"})
				s.processAgentDesktopAudioState(agent, agentmgr.Message{Data: late})
				current, _ := json.Marshal(agentmgr.DesktopAudioDataPayload{SessionID: secondAudioWire, Data: base64.StdEncoding.EncodeToString([]byte("current audio"))})
				wrong, _ := json.Marshal(agentmgr.DesktopAudioDataPayload{SessionID: secondAudioWire, Data: base64.StdEncoding.EncodeToString([]byte("wrong socket"))})
				s.processAgentDesktopAudioData(wrongSocket, agentmgr.Message{Data: wrong})
				s.processAgentDesktopAudioData(agent, agentmgr.Message{Data: current})
				_ = secondAudio.SetReadDeadline(time.Now().Add(time.Second))
				messageType, data, err := secondAudio.ReadMessage()
				if err != nil || messageType != websocket.BinaryMessage || string(data) != "current audio" {
					t.Fatalf("replacement audio received type=%d data=%q err=%v", messageType, data, err)
				}
			}
			_ = newBrowser.Close()
			if stopped := streamGenerationID(t, closes); stopped != newWire {
				t.Fatalf("new browser stopped %q instead of its wire owner %q", stopped, newWire)
			}
			if secondAudioWire != "" {
				if wire := streamGenerationID(t, audioCloses); wire != secondAudioWire {
					t.Fatalf("desktop cleanup stopped wrong audio owner %q", wire)
				}
				select {
				case <-audioFinished:
				case <-time.After(time.Second):
					t.Fatal("desktop cleanup left its audio browser alive")
				}
			}
		})
	}
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
		s.processAgentDesktopStarted(conn, msg)
	case "desktop:closed":
		s.processAgentDesktopClosed(conn, msg)
	case "desktop:data":
		s.processAgentDesktopData(conn, msg)
	case "webrtc:started":
		s.processAgentWebRTCStarted(conn, msg)
	case "webrtc:closed":
		s.processAgentWebRTCStopped(conn, msg)
	case "webrtc:data":
		s.processAgentWebRTCAnswer(conn, msg)
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

func TestPersistentTmuxDisconnectReportsUnknownOutcomePromptly(t *testing.T) {
	s := newTestAPIServer(t)
	s.agentMgr = agentmgr.NewManager()
	serverWS, peerWS, cleanup := newWebSocketPair(t)
	defer cleanup()
	conn := agentmgr.NewAgentConn(serverWS, "node", "linux")
	s.agentMgr.Register(conn)
	result := make(chan error, 1)
	go func() { result <- s.ensureTerminalDeps().KillPersistentTerminalTmuxViaAgent("node", "saved") }()
	_ = peerWS.SetReadDeadline(time.Now().Add(3 * time.Second))
	var request agentmgr.Message
	if err := peerWS.ReadJSON(&request); err != nil {
		t.Fatal(err)
	}
	if request.Type != agentmgr.MsgTerminalTmuxKill {
		t.Fatalf("request type=%q", request.Type)
	}
	s.agentMgr.UnregisterIfMatch(conn.AssetID, conn)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "outcome unknown") {
			t.Fatalf("disconnect result=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("tmux cleanup waited for its command timeout after disconnect")
	}
	if _, pending := s.pendingAgentCmds.Load(request.ID); pending {
		t.Fatal("tmux request remained pending")
	}
}

// A full output queue must end the stream visibly, not silently lose terminal
// bytes or block the per-agent dispatch/retirement gate forever.
func TestAgentStreamOutputBackpressureClosesBridge(t *testing.T) {
	for _, kind := range []string{"terminal", "desktop", "audio"} {
		t.Run(kind, func(t *testing.T) {
			s := &apiServer{}
			conn := &agentmgr.AgentConn{AssetID: "node"}
			output := make(chan []byte, 1)
			output <- []byte("queued")
			closed := make(chan struct{})
			var reason func() string
			if kind == "terminal" {
				bridge := &terminalBridge{OutputCh: output, ClosedCh: closed, ExpectedAgentID: conn.AssetID, AgentConn: conn, WireSessionID: "wire"}
				s.terminalBridges.Store("wire", bridge)
				reason = func() string { return bridge.CloseReasonOr("") }
			} else if kind == "audio" {
				// A legacy in-process audio bridge has no wire ID of its own.
				bridge := &desktopBridge{ExpectedAgentID: conn.AssetID, AgentConn: conn, ClosedCh: closed, AudioCh: make(chan desktopAudioOutbound, 1)}
				bridge.AudioCh <- desktopAudioOutbound{Payload: []byte("queued")}
				s.desktopBridges.Store("wire", bridge)
				reason = bridge.CloseReason
			} else {
				bridge := &desktopBridge{OutputCh: output, ClosedCh: closed, ExpectedAgentID: conn.AssetID, AgentConn: conn, WireSessionID: "wire"}
				s.desktopBridges.Store("wire", bridge)
				reason = bridge.CloseReason
			}
			finished := make(chan struct{})
			go func() {
				if kind == "audio" {
					raw, _ := json.Marshal(agentmgr.DesktopAudioDataPayload{SessionID: "wire", Data: base64.StdEncoding.EncodeToString([]byte("overflow"))})
					s.processAgentDesktopAudioData(conn, agentmgr.Message{Data: raw})
				} else {
					streamGenerationReply(s, conn, kind, "data", "wire", "overflow")
				}
				close(finished)
			}()
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("full output queue held the agent dispatcher")
			}
			select {
			case <-closed:
			default:
				t.Fatal("overflow silently dropped output and left stream active")
			}
			if got := reason(); !strings.Contains(got, "backpressure") && !strings.Contains(got, "overflow") {
				t.Fatalf("missing close reason: %q", got)
			}
		})
	}
}
