package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

func TestPersistentDeleteUsesLatestAttachedStateForTmuxCleanup(t *testing.T) {
	s := newTestAPIServer(t)
	s.agentMgr = agentmgr.NewManager()
	agentWS, peer, cleanup := newWebSocketPair(t)
	defer cleanup()
	agent := agentmgr.NewAgentConn(agentWS, "node", "linux")
	agent.SetMeta("terminal.tmux.has", "true")
	s.agentMgr.Register(agent)
	defer s.agentMgr.Unregister(agent.AssetID)
	base := s.terminalPersistentStore
	saved, err := base.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: agent.AssetID, Title: "saved"})
	if err != nil {
		t.Fatal(err)
	}
	store := &pausedPersistentSnapshotStore{TerminalPersistentSessionStore: base, entered: make(chan struct{}), resume: make(chan struct{})}
	s.terminalPersistentStore = store
	var once sync.Once
	release := func() { once.Do(func() { close(store.resume) }) }
	defer release()
	request := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+saved.ID, nil)
	request = request.WithContext(contextWithPrincipal(request.Context(), "owner", auth.RoleOwner))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.handlePersistentSessionActions(response, request); close(done) }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("DELETE did not load its old snapshot")
	}
	if _, err := base.MarkPersistentSessionAttached(saved.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	release()
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	var msg agentmgr.Message
	if err := peer.ReadJSON(&msg); err != nil {
		t.Fatalf("DELETE skipped tmux that attached after its initial read: %v", err)
	}
	if msg.Type != agentmgr.MsgTerminalTmuxKill {
		t.Fatalf("cleanup sent %q", msg.Type)
	}
	var kill agentmgr.TerminalTmuxKillData
	_ = json.Unmarshal(msg.Data, &kill)
	raw, _ := json.Marshal(agentmgr.CommandResultData{JobID: kill.JobID, SessionID: kill.SessionID, CommandID: kill.CommandID, Status: "succeeded"})
	s.processAgentCommandResult(agent, agentmgr.Message{Data: raw})
	select {
	case <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("saved DELETE returned %d: %s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("saved DELETE did not finish after real tmux reply")
	}
}

// The endpoint can have loaded a saved shell before DELETE removes it. The
// attach owner must reread under the lifecycle gate before creating metadata.
func TestPersistentDeleteRejectsAttachWithStaleSnapshot(t *testing.T) {
	s := newTestAPIServer(t)
	saved, err := s.terminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: "node", Title: "saved"})
	if err != nil {
		t.Fatal(err)
	}
	store := &pausedPersistentSnapshotStore{TerminalPersistentSessionStore: s.terminalPersistentStore, entered: make(chan struct{}), resume: make(chan struct{})}
	s.terminalPersistentStore = store
	var once sync.Once
	release := func() { once.Do(func() { close(store.resume) }) }
	defer release()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handlePersistentSessionActions(w, r.WithContext(contextWithPrincipal(r.Context(), "owner", auth.RoleOwner)))
	}))
	defer func() { release(); server.Close() }()
	url := server.URL + "/terminal/persistent-sessions/" + saved.ID
	attached := make(chan int, 1)
	go func() {
		response, err := server.Client().Post(url+"/attach", "application/json", nil)
		if err != nil {
			attached <- 0
			return
		}
		_ = response.Body.Close()
		attached <- response.StatusCode
	}()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("attach did not load its initial snapshot")
	}
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("saved delete returned %d", response.StatusCode)
	}
	release()
	select {
	case code := <-attached:
		if code != http.StatusNotFound {
			t.Fatalf("stale attach returned %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("stale attach did not finish")
	}
	sessions, err := s.terminalStore.ListSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("stale attach recreated sessions=%v err=%v", sessions, err)
	}
}

type pausedPersistentSnapshotStore struct {
	persistence.TerminalPersistentSessionStore
	paused  atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (s *pausedPersistentSnapshotStore) GetPersistentSession(id string) (terminal.PersistentSession, bool, error) {
	session, present, err := s.TerminalPersistentSessionStore.GetPersistentSession(id)
	if s.paused.CompareAndSwap(false, true) {
		close(s.entered)
		<-s.resume
	}
	return session, present, err
}

// Claim runs inside the agent connection gate. An old detach's DB write must
// not hold Claim there; activation still writes the new owner's state last.
func TestPersistentBrowserClaimDoesNotWaitForOldDetachStore(t *testing.T) {
	s := newTestAPIServer(t)
	saved, err := s.terminalPersistentStore.CreateOrUpdatePersistentSession(terminal.CreatePersistentSessionRequest{ActorID: "owner", Target: "node", Title: "saved"})
	if err != nil {
		t.Fatal(err)
	}
	store := &pausedPersistentDetachStore{TerminalPersistentSessionStore: s.terminalPersistentStore, entered: make(chan struct{}), resume: make(chan struct{})}
	s.terminalPersistentStore = store
	var once sync.Once
	release := func() { once.Do(func() { close(store.resume) }) }
	defer release()
	oldBrowser, _, oldCleanup := newWebSocketPair(t)
	defer oldCleanup()
	newBrowser, _, newCleanup := newWebSocketPair(t)
	defer newCleanup()
	d := s.ensureTerminalDeps()
	session := terminal.Session{PersistentSessionID: saved.ID}
	activateOld, detachOld := d.ClaimPersistentTerminalStream(session, oldBrowser)
	activateOld()
	detached := make(chan struct{})
	go func() { detachOld(); close(detached) }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("old detach did not reach its store write")
	}
	type claim struct{ activate, detach func() }
	claimed := make(chan claim, 1)
	go func() {
		activate, detach := d.ClaimPersistentTerminalStream(session, newBrowser)
		claimed <- claim{activate, detach}
	}()
	var next claim
	select {
	case next = <-claimed:
	case <-time.After(time.Second):
		t.Fatal("new claim blocked the agent gate on the old store write")
	}
	release()
	<-detached
	next.activate()
	defer next.detach()
	current, present, err := store.GetPersistentSession(saved.ID)
	if err != nil || !present || current.Status != "attached" {
		t.Fatalf("new owner's final saved state=%+v present=%v err=%v", current, present, err)
	}
	// The old iOS view sends this notification after its logical DELETE. It
	// must report the new browser's state rather than mark the saved shell idle.
	request := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+saved.ID+"/detach", nil)
	request = request.WithContext(contextWithPrincipal(request.Context(), "owner", auth.RoleOwner))
	response := httptest.NewRecorder()
	s.handlePersistentSessionActions(response, request)
	var envelope struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || envelope.PersistentSession.Status != "attached" {
		t.Fatalf("old detach overwrote current ownership: status=%d body=%s err=%v", response.Code, response.Body.String(), err)
	}
}

type pausedPersistentDetachStore struct {
	persistence.TerminalPersistentSessionStore
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (s *pausedPersistentDetachStore) MarkPersistentSessionDetached(id string, at time.Time) (terminal.PersistentSession, error) {
	s.once.Do(func() { close(s.entered); <-s.resume })
	return s.TerminalPersistentSessionStore.MarkPersistentSessionDetached(id, at)
}
