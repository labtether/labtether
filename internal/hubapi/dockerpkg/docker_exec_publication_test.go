package dockerpkg

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// A browser that captured an old agent before its upgrade must not replace a
// fresh browser or claim its persistent attachment after the agent reconnects.
func TestDockerDelayedBrowserCannotReplaceNewConnectionStream(t *testing.T) {
	oldConn, _ := newDockerLifecycleSocket(t)
	freshConn, freshPeer := newDockerLifecycleSocket(t)
	manager := agentmgr.NewManager()
	manager.Register(oldConn)
	defer manager.Unregister(freshConn.AssetID)
	var claims atomic.Int32
	deps := &Deps{AgentMgr: manager, DockerExecBridges: &sync.Map{},
		TerminalWebSocketUpgrader:  websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		ParseTerminalSize:          func(string, string) (int, int) { return 80, 24 },
		IsControlMessage:           func([]byte) bool { return false },
		TouchBrowserWSReadDeadline: func(*websocket.Conn) error { return nil },
		StartBrowserWSKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
		ClaimPersistentTerminalStream: func(terminal.Session, *websocket.Conn) (func(), func()) {
			claims.Add(1)
			return func() {}, func() {}
		},
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ended := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		label := r.URL.Query().Get("browser")
		if label == "old" {
			w = &dockerDelayedHijacker{ResponseWriter: w, entered: entered, release: release}
		}
		deps.HandleDockerExecTerminalStream(w, r,
			terminal.Session{ID: "logical", Target: "container", PersistentSessionID: "persistent"},
			oldConn.AssetID, "container")
		ended <- label
	}))
	defer server.Close()
	defer unblock()
	type dialResult struct {
		conn *websocket.Conn
		err  error
	}
	oldDial := make(chan dialResult, 1)
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	go func() {
		ws, _, err := websocket.DefaultDialer.Dial(url+"?browser=old", nil)
		oldDial <- dialResult{ws, err}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("old browser did not reach upgrade")
	}
	manager.Register(freshConn)
	fresh, _, err := websocket.DefaultDialer.Dial(url+"?browser=new", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	start := readDockerLifecycleMessage(t, freshPeer, agentmgr.MsgDockerExecStart)
	var request agentmgr.DockerExecStartData
	if err := json.Unmarshal(start.Data, &request); err != nil {
		t.Fatal(err)
	}
	started, _ := json.Marshal(agentmgr.DockerExecStartedData{SessionID: request.SessionID})
	deps.ProcessAgentDockerExecStarted(freshConn, agentmgr.Message{Data: started})
	unblock()
	select {
	case result := <-oldDial:
		if result.err != nil {
			t.Fatal(result.err)
		}
		defer result.conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("old upgrade did not finish")
	}
	select {
	case label := <-ended:
		if label != "old" {
			t.Fatalf("fresh browser ended: %s", label)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retired browser was not rejected")
	}
	if claims.Load() != 1 {
		t.Fatalf("retired browser claimed persistent session: %d claims", claims.Load())
	}
	raw, ok := deps.dockerExecOwners.Load("logical")
	if !ok || raw.(*DockerExecBridge).AgentConn != freshConn {
		t.Fatal("retired browser replaced fresh bridge")
	}
	payload, _ := json.Marshal(agentmgr.DockerExecDataPayload{
		SessionID: request.SessionID, Data: base64.StdEncoding.EncodeToString([]byte("fresh survives")),
	})
	deps.ProcessAgentDockerExecData(freshConn, agentmgr.Message{Data: payload})
	_ = fresh.SetReadDeadline(time.Now().Add(2 * time.Second))
	kind, data, err := fresh.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage || string(data) != "fresh survives" {
		t.Fatalf("fresh output kind=%d data=%q err=%v", kind, data, err)
	}
}

type dockerDelayedHijacker struct {
	http.ResponseWriter
	entered chan<- struct{}
	release <-chan struct{}
}

func (w *dockerDelayedHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	close(w.entered)
	<-w.release
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func TestDockerSessionDeleteClosesPendingAndReadyStreams(t *testing.T) {
	for _, ready := range []bool{false, true} {
		name := "waiting_for_agent"
		if ready {
			name = "ready"
		}
		t.Run(name, func(t *testing.T) {
			agent, peer := newDockerLifecycleSocket(t)
			manager := agentmgr.NewManager()
			manager.Register(agent)
			defer manager.Unregister(agent.AssetID)
			deps := &Deps{AgentMgr: manager, DockerExecBridges: &sync.Map{},
				TerminalWebSocketUpgrader:  websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
				ParseTerminalSize:          func(string, string) (int, int) { return 80, 24 },
				IsControlMessage:           func([]byte) bool { return false },
				TouchBrowserWSReadDeadline: func(*websocket.Conn) error { return nil },
				StartBrowserWSKeepalive:    func(*websocket.Conn, *sync.Mutex, string) func() { return func() {} },
			}
			var registry shared.SessionStreamRegistry
			var exists atomic.Bool
			exists.Store(true)
			ended := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w, r, cleanup, err := registry.Open(w, r, "logical", func() (bool, error) { return exists.Load(), nil })
				if err != nil {
					http.Error(w, "session not found", http.StatusNotFound)
					return
				}
				defer cleanup()
				defer close(ended)
				deps.HandleDockerExecTerminalStream(w, r, terminal.Session{ID: "logical", Target: "container"}, agent.AssetID, "container")
			}))
			defer server.Close()
			url := "ws" + strings.TrimPrefix(server.URL, "http")
			browser, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()
			start := readDockerLifecycleMessage(t, peer, agentmgr.MsgDockerExecStart)
			var request agentmgr.DockerExecStartData
			if err := json.Unmarshal(start.Data, &request); err != nil {
				t.Fatal(err)
			}
			if ready {
				started, _ := json.Marshal(agentmgr.DockerExecStartedData{SessionID: request.SessionID})
				deps.ProcessAgentDockerExecStarted(agent, agentmgr.Message{Data: started})
			}
			if err := registry.Delete("logical", func() error { exists.Store(false); return nil }); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ended:
			case <-time.After(time.Second):
				t.Fatal("deleted Docker stream did not finish promptly")
			}
			stop := readDockerLifecycleMessage(t, peer, agentmgr.MsgDockerExecClose)
			var closed agentmgr.DockerExecCloseData
			if err := json.Unmarshal(stop.Data, &closed); err != nil || closed.SessionID != request.SessionID {
				t.Fatalf("wrong close owner: %+v, %v", closed, err)
			}
			if _, ok := deps.dockerExecOwners.Load("logical"); ok {
				t.Fatal("deleted logical bridge remains")
			}
			if _, ok := deps.DockerExecBridges.Load(request.SessionID); ok {
				t.Fatal("deleted wire bridge remains")
			}
			late, response, err := websocket.DefaultDialer.Dial(url, nil)
			if late != nil {
				_ = late.Close()
			}
			if err == nil || response == nil || response.StatusCode != http.StatusNotFound {
				t.Fatalf("deleted session reopened: response=%v err=%v", response, err)
			}
			_ = response.Body.Close()
		})
	}
}
