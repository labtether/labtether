package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/terminal"
)

// Exercise the actual DELETE and stream routes, not just map cleanup: active
// browsers, pending starts, and upgrades paused before Hijack must all retire.
func TestSessionDeleteEndsBrowserAndAgentLifecycle(t *testing.T) {
	for _, kind := range []string{"terminal", "persistent", "saved-delete", "docker", "desktop", "webrtc"} {
		for _, phase := range []string{"active", "pending", "delayed"} {
			t.Run(kind+"/"+phase, func(t *testing.T) { runSessionDeleteLifecycle(t, kind, phase, false) })
		}
	}
}

func TestSessionDeleteStorageFailurePreservesLiveStream(t *testing.T) {
	for _, kind := range []string{"terminal", "desktop"} {
		t.Run(kind, func(t *testing.T) { runSessionDeleteLifecycle(t, kind, "active", true) })
	}
}

func runSessionDeleteLifecycle(t *testing.T, kind, phase string, failDelete bool) {
	t.Helper()
	s := newTestAPIServer(t)
	s.agentMgr = agentmgr.NewManager()
	agentWS, peer, cleanup := newWebSocketPair(t)
	defer cleanup()
	agent := agentmgr.NewAgentConn(agentWS, "node", "linux")
	agent.SetMeta("terminal.tmux.has", "true")
	agent.SetMeta("webrtc_available", "true")
	agent.SetMeta("desktop_webrtc_real_desktop_supported", "true")
	s.agentMgr.Register(agent)
	defer s.agentMgr.Unregister(agent.AssetID)
	request := terminal.CreateSessionRequest{ActorID: "owner", Target: agent.AssetID, Mode: "interactive"}
	if kind == "persistent" || kind == "saved-delete" {
		persistent, err := s.terminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: request.Target, Title: "Keep this saved shell"})
		if err != nil {
			t.Fatal(err)
		}
		request.PersistentSessionID, request.TmuxSessionName = persistent.ID, persistent.TmuxSessionName
	}
	if kind == "desktop" || kind == "webrtc" {
		request.Mode = "desktop"
	}
	session, err := s.terminalStore.CreateSession(request)
	if err != nil {
		t.Fatal(err)
	}
	if failDelete {
		originalStore := s.terminalStore
		s.terminalStore = failingTerminalStore{TerminalStore: originalStore, deleteErr: errors.New("storage unavailable")}
		defer func() { s.terminalStore = originalStore }()
	}
	if request.Mode == "desktop" {
		protocol := "vnc"
		if kind == "webrtc" {
			protocol = "webrtc"
		}
		s.setDesktopSessionOptions(session.ID, desktopSessionOptions{Protocol: protocol})
	}
	if kind == "docker" {
		s.ensureTerminalDeps().ResolveDockerExecSessionTarget = func(string) (string, string, bool) { return agent.AssetID, "container", true }
	}
	starts, stops, tmuxKills := make(chan string, 2), make(chan string, 4), make(chan struct{}, 1)
	audioStarts, audioStops := make(chan string, 1), make(chan string, 1)
	go func() {
		for {
			var msg agentmgr.Message
			if peer.ReadJSON(&msg) != nil {
				return
			}
			var data struct {
				SessionID string `json:"session_id"`
			}
			_ = json.Unmarshal(msg.Data, &data)
			switch msg.Type {
			case agentmgr.MsgTerminalStart, agentmgr.MsgDockerExecStart, agentmgr.MsgDesktopStart, agentmgr.MsgWebRTCStart:
				starts <- data.SessionID
				if phase == "active" {
					sessionDeleteReply(s, agent, kind, "started", data.SessionID, "")
				}
			case agentmgr.MsgTerminalClose, agentmgr.MsgDockerExecClose, agentmgr.MsgDesktopClose, agentmgr.MsgWebRTCStop:
				stops <- data.SessionID
			case agentmgr.MsgTerminalTmuxKill:
				tmuxKills <- struct{}{}
				var kill agentmgr.TerminalTmuxKillData
				_ = json.Unmarshal(msg.Data, &kill)
				raw, _ := json.Marshal(agentmgr.CommandResultData{JobID: kill.JobID, SessionID: kill.SessionID, CommandID: kill.CommandID, Status: "succeeded"})
				s.processAgentCommandResult(agent, agentmgr.Message{Data: raw})
			case agentmgr.MsgDesktopAudioStart:
				audioStarts <- data.SessionID
			case agentmgr.MsgDesktopAudioStop:
				audioStops <- data.SessionID
			}
		}
	}()
	entered, resume, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	audioFinished := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(resume) }) }
	prefix := "/terminal/sessions/"
	if request.Mode == "desktop" {
		prefix = "/desktop/sessions/"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(contextWithPrincipal(r.Context(), "owner", auth.RoleOwner))
		if strings.HasSuffix(r.URL.Path, "/audio") {
			defer close(audioFinished)
		}
		if strings.HasSuffix(r.URL.Path, "/stream") && r.URL.Query().Get("test") == "first" {
			defer close(finished)
			if phase == "delayed" {
				w = pausedStreamUpgrade{ResponseWriter: w, entered: entered, resume: resume}
			}
		}
		if strings.HasPrefix(r.URL.Path, "/terminal/persistent-sessions/") {
			s.handlePersistentSessionActions(w, r)
		} else if request.Mode == "desktop" {
			s.handleDesktopSessionActions(w, r)
		} else {
			s.handleSessionActions(w, r)
		}
	}))
	defer func() { release(); server.Close() }()
	url := server.URL + prefix + session.ID
	type dialResult struct {
		conn *websocket.Conn
		err  error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http")+"/stream?test=first", nil)
		dialed <- dialResult{conn, err}
	}()
	var browser *websocket.Conn
	var wire string
	if phase == "delayed" {
		select {
		case <-entered:
		case result := <-dialed:
			if result.conn != nil {
				_ = result.conn.Close()
			}
			t.Fatalf("stream did not reach delayed upgrade: %v", result.err)
		case <-time.After(3 * time.Second):
			t.Fatal("stream did not reach delayed upgrade")
		}
	} else {
		select {
		case result := <-dialed:
			if result.err != nil {
				t.Fatal(result.err)
			}
			browser = result.conn
			defer browser.Close()
		case <-time.After(3 * time.Second):
			t.Fatal("browser upgrade timed out")
		}
		wire = streamGenerationID(t, starts)
	}
	var audio *websocket.Conn
	var audioWire string
	if kind == "desktop" && phase == "active" {
		audio, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http")+"/audio", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer audio.Close()
		audioWire = streamGenerationID(t, audioStarts)
	}
	deleteURL := url
	if kind == "saved-delete" {
		deleteURL = server.URL + "/terminal/persistent-sessions/" + request.PersistentSessionID
	}
	deleteRequest, _ := http.NewRequest(http.MethodDelete, deleteURL, nil)
	response, err := server.Client().Do(deleteRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if failDelete {
		if response.StatusCode != http.StatusInternalServerError {
			t.Fatalf("failed store delete returned %d", response.StatusCode)
		}
		sessionDeleteReply(s, agent, kind, "data", wire, "still active after failed delete")
		if got := streamGenerationBrowserPayload(t, browser, kind, session.ID); got != "still active after failed delete" {
			t.Fatalf("stream changed after rejected delete: %q", got)
		}
		if audio != nil {
			raw, _ := json.Marshal(agentmgr.DesktopAudioDataPayload{SessionID: audioWire, Data: base64.StdEncoding.EncodeToString([]byte("audio survived"))})
			s.processAgentDesktopAudioData(agent, agentmgr.Message{Data: raw})
			if got := streamGenerationBrowserPayload(t, audio, "audio", session.ID); got != "audio survived" {
				t.Fatalf("audio changed after rejected delete: %q", got)
			}
		}
		return
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete returned %d", response.StatusCode)
	}
	release()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("DELETE left active or pending browser handler alive")
	}
	if phase == "delayed" {
		select {
		case result := <-dialed:
			if result.conn != nil {
				_ = result.conn.Close()
				t.Fatal("deleted pending request upgraded successfully")
			}
		case <-time.After(time.Second):
			t.Fatal("deleted pending browser dial did not finish")
		}
		select {
		case id := <-starts:
			t.Fatalf("deleted pending request started agent session %q", id)
		default:
		}
	} else if stopped := streamGenerationID(t, stops); stopped != wire {
		t.Fatalf("DELETE closed %q rather than its wire owner %q", stopped, wire)
	}
	if audio != nil {
		if stopped := streamGenerationID(t, audioStops); stopped != audioWire {
			t.Fatalf("DELETE closed wrong audio wire owner %q", stopped)
		}
		select {
		case <-audioFinished:
		case <-time.After(time.Second):
			t.Fatal("DELETE left the audio child browser alive")
		}
	}
	if _, exists, err := s.terminalStore.GetSession(session.ID); err != nil || exists {
		t.Fatalf("deleted logical record still exists=%v err=%v", exists, err)
	}
	if kind == "persistent" {
		if _, exists, err := s.terminalPersistentStore.GetPersistentSession(request.PersistentSessionID); err != nil || !exists {
			t.Fatalf("logical DELETE removed saved shell exists=%v err=%v", exists, err)
		}
		select {
		case <-tmuxKills:
			t.Fatal("logical DELETE killed saved tmux instead of detaching")
		default:
		}
	}
	if kind == "saved-delete" {
		if _, exists, err := s.terminalPersistentStore.GetPersistentSession(request.PersistentSessionID); err != nil || exists {
			t.Fatalf("explicit saved DELETE left record exists=%v err=%v", exists, err)
		}
		if phase != "delayed" {
			select {
			case <-tmuxKills:
			default:
				t.Fatal("explicit saved DELETE did not kill attached tmux")
			}
		}
	}
	fresh, err := server.Client().Get(url + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	_ = fresh.Body.Close()
	if fresh.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted session reopened with status %d", fresh.StatusCode)
	}
}

func sessionDeleteReply(s *apiServer, conn *agentmgr.AgentConn, kind, event, wire, data string) {
	if kind == "persistent" || kind == "saved-delete" {
		kind = "terminal"
	}
	if kind != "docker" {
		streamGenerationReply(s, conn, kind, event, wire, data)
		return
	}
	raw, _ := json.Marshal(map[string]any{"session_id": wire, "data": base64.StdEncoding.EncodeToString([]byte(data))})
	if event == "started" {
		s.processAgentDockerExecStarted(conn, agentmgr.Message{Data: raw})
	} else {
		s.processAgentDockerExecData(conn, agentmgr.Message{Data: raw})
	}
}
