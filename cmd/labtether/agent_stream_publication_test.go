package main

import (
	"bufio"
	"encoding/json"
	"net"
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

func TestAgentBrowserPublicationRejectsRetiredConnection(t *testing.T) {
	for _, mode := range []string{"terminal", "persistent", "desktop", "webrtc"} {
		t.Run(mode, func(t *testing.T) {
			kind := mode
			if kind == "persistent" {
				kind = "terminal"
			}
			s := newTestAPIServer(t)
			s.agentMgr = agentmgr.NewManager()
			oldWS, _, oldCleanup := newWebSocketPair(t)
			defer oldCleanup()
			oldAgent := agentmgr.NewAgentConn(oldWS, "node", "linux")
			oldAgent.SetMeta("terminal.tmux.has", "true")
			s.agentMgr.Register(oldAgent)
			defer s.agentMgr.Unregister("node")
			public := terminal.Session{ID: "sess-delayed-upgrade", Target: "node"}
			if mode == "persistent" {
				saved, err := s.terminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: public.Target, Title: "Saved shell"})
				if err != nil {
					t.Fatal(err)
				}
				public.PersistentSessionID, public.TmuxSessionName = saved.ID, saved.TmuxSessionName
			}
			entered, resume, oldFinished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(resume) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("paused") == "1" {
					defer close(oldFinished)
					w = pausedStreamUpgrade{ResponseWriter: w, entered: entered, resume: resume}
				}
				switch kind {
				case "terminal":
					s.handleAgentTerminalStream(w, r, public)
				case "desktop":
					s.handleAgentDesktopStream(w, r, public)
				case "webrtc":
					s.handleWebRTCStream(w, r, public)
				}
			}))
			defer func() { release(); server.Close() }()
			url := "ws" + strings.TrimPrefix(server.URL, "http")
			oldDial := make(chan error, 1)
			go func() {
				browser, _, err := websocket.DefaultDialer.Dial(url+"?paused=1", nil)
				if browser != nil {
					defer browser.Close()
					_ = browser.SetReadDeadline(time.Now().Add(3 * time.Second))
					for err == nil {
						_, _, err = browser.ReadMessage()
					}
				}
				oldDial <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("old browser did not pause after capturing its agent")
			}

			newWS, newPeer, newCleanup := newWebSocketPair(t)
			defer newCleanup()
			currentAgent := agentmgr.NewAgentConn(newWS, "node", "linux")
			currentAgent.SetMeta("terminal.tmux.has", "true")
			s.agentMgr.Register(currentAgent)
			starts := make(chan string, 1)
			go func() {
				for {
					var msg agentmgr.Message
					if newPeer.ReadJSON(&msg) != nil {
						return
					}
					if msg.Type != agentmgr.MsgTerminalStart && msg.Type != agentmgr.MsgDesktopStart && msg.Type != agentmgr.MsgWebRTCStart {
						continue
					}
					var payload struct {
						SessionID string `json:"session_id"`
					}
					if json.Unmarshal(msg.Data, &payload) == nil {
						streamGenerationReply(s, currentAgent, kind, "started", payload.SessionID, "")
						starts <- payload.SessionID
					}
				}
			}()
			browser, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			wire := streamGenerationID(t, starts)
			release()
			select {
			case <-oldFinished:
			case <-time.After(3 * time.Second):
				t.Fatal("retired browser handler did not finish")
			}
			select {
			case <-oldDial:
			case <-time.After(3 * time.Second):
				t.Fatal("retired browser socket remained open")
			}
			streamGenerationReply(s, currentAgent, kind, "data", wire, "fresh browser still owns stream")
			if got := streamGenerationBrowserPayload(t, browser, kind, public.ID); got != "fresh browser still owns stream" {
				t.Fatalf("fresh browser output=%q", got)
			}
			if mode == "persistent" {
				stored, exists, err := s.terminalPersistentStore.GetPersistentSession(public.PersistentSessionID)
				if err != nil || !exists || stored.Status != "attached" {
					t.Fatalf("retired request detached fresh persistent owner: %+v err=%v", stored, err)
				}
			}
		})
	}
}
