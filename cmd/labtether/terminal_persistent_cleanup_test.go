package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeleteNeverAttachedPersistentSessionDoesNotRequireRemoteCleanup(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"target":"lab-host-01","title":"Ops Shell"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent session, got %d", createRec.Code)
	}

	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting never-attached metadata, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	_, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if ok {
		t.Fatal("expected never-attached persistent metadata to be deleted")
	}
}

func TestDeleteLegacyPlainAgentPersistentSessionClosesActualShellAndRemovesState(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Legacy Shell"}`)))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent metadata, got %d", createRec.Code)
	}
	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	// Reproduce the r6 legacy state: attach was accepted before a connected
	// agent's explicit has_tmux=false result was enforced.
	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 reproducing legacy attached state, got %d", attachRec.Code)
	}
	var attached struct {
		Session terminal.Session `json:"session"`
	}
	if err := json.Unmarshal(attachRec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("decode attach response: %v", err)
	}

	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	agentConn := agentmgr.NewAgentConn(serverConn, "lab-host-01", "linux")
	agentConn.SetMeta("terminal.tmux.has", "false")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("lab-host-01")
	sut.resetTerminalDepsForTest()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		message := waitForAgentTerminalMessage(t, clientConn, agentmgr.MsgTerminalClose)
		var closeData agentmgr.TerminalCloseData
		if err := json.Unmarshal(message.Data, &closeData); err != nil {
			t.Errorf("decode terminal close payload: %v", err)
			return
		}
		if closeData.SessionID != attached.Session.ID {
			t.Errorf("closed session=%q, want %q", closeData.SessionID, attached.Session.ID)
		}
	}()

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)
	<-closed
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 cleaning legacy plain-shell state, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	if _, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID); err != nil || ok {
		t.Fatalf("persistent state remains after cleanup: ok=%t err=%v", ok, err)
	}
	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("list terminal sessions: %v", err)
	}
	for _, session := range sessions {
		if session.PersistentSessionID == created.PersistentSession.ID {
			t.Fatalf("attached session %s remains after cleanup", session.ID)
		}
	}
}

func TestDeletePersistentSessionEndsRemoteRuntimeBeforeRemovingMetadata(t *testing.T) {
	sut := newTestAPIServer(t)

	originalCleanup := terminalpkg.PersistentTmuxCleanupSSHFunc
	terminalpkg.PersistentTmuxCleanupSSHFunc = func(_ *terminalpkg.Deps, cfg *terminal.SSHConfig, tmuxSessionName string) error {
		if cfg == nil {
			t.Fatal("expected ssh config for cleanup")
		}
		if cfg.Host != "lab-host-01" {
			t.Fatalf("expected ssh cleanup host lab-host-01, got %q", cfg.Host)
		}
		if tmuxSessionName == "" {
			t.Fatal("expected tmux session name")
		}
		return nil
	}
	defer func() {
		terminalpkg.PersistentTmuxCleanupSSHFunc = originalCleanup
	}()

	t.Setenv("SSH_USERNAME", "ops")
	t.Setenv("SSH_PASSWORD", "secret")
	t.Setenv("SSH_STRICT_HOST_KEY", "false")

	createPayload := []byte(`{"target":"lab-host-01","title":"Ops Shell"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent session, got %d", createRec.Code)
	}

	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 attaching persistent session, got %d", attachRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting persistent session, got %d", deleteRec.Code)
	}

	_, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if ok {
		t.Fatal("expected persistent session to be deleted after cleanup succeeded")
	}
}

func TestDeletePersistentSessionFailsClosedWhenAttachedSessionCleanupFails(t *testing.T) {
	sut := newTestAPIServer(t)

	originalCleanup := terminalpkg.PersistentTmuxCleanupSSHFunc
	terminalpkg.PersistentTmuxCleanupSSHFunc = func(_ *terminalpkg.Deps, _ *terminal.SSHConfig, tmuxSessionName string) error {
		if tmuxSessionName == "" {
			t.Fatal("expected tmux session name during cleanup")
		}
		return nil
	}
	defer func() {
		terminalpkg.PersistentTmuxCleanupSSHFunc = originalCleanup
	}()

	t.Setenv("SSH_USERNAME", "ops")
	t.Setenv("SSH_PASSWORD", "secret")
	t.Setenv("SSH_STRICT_HOST_KEY", "false")

	createPayload := []byte(`{"target":"lab-host-01","title":"Ops Shell"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent session, got %d", createRec.Code)
	}

	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 attaching persistent session, got %d", attachRec.Code)
	}

	originalStore := sut.terminalStore
	sut.terminalStore = failingTerminalStore{
		TerminalStore: originalStore,
		deleteErr:     errors.New("synthetic delete failure"),
	}
	sut.resetTerminalDepsForTest() // reset cached deps to pick up new terminalStore
	defer func() {
		sut.terminalStore = originalStore
		sut.resetTerminalDepsForTest()
	}()

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 deleting persistent session when attached session cleanup fails, got %d", deleteRec.Code)
	}

	_, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if !ok {
		t.Fatal("expected persistent session to remain after attached session cleanup failure")
	}
}

func TestDeletePersistentSessionUsesTerminalScopedAgentCleanup(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	agentConn := agentmgr.NewAgentConn(serverConn, "lab-host-01", "linux")
	agentConn.SetMeta("terminal.tmux.has", "true")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("lab-host-01")

	createPayload := []byte(`{"target":"lab-host-01","title":"Ops Shell"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent session, got %d", createRec.Code)
	}

	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 attaching persistent session, got %d: %s", attachRec.Code, attachRec.Body.String())
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound terminal close message: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgTerminalClose {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgTerminalClose)
			return
		}

		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound tmux cleanup message: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgTerminalTmuxKill {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgTerminalTmuxKill)
			return
		}

		var req agentmgr.TerminalTmuxKillData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode terminal tmux kill payload: %v", err)
			return
		}
		if strings.TrimSpace(req.TmuxSession) == "" {
			t.Error("expected tmux session name in terminal tmux kill payload")
			return
		}

		raw, err := json.Marshal(agentmgr.CommandResultData{
			JobID:     req.JobID,
			SessionID: req.SessionID,
			CommandID: req.CommandID,
			Status:    "succeeded",
		})
		if err != nil {
			t.Errorf("marshal command result payload: %v", err)
			return
		}
		sut.processAgentCommandResult(&agentmgr.AgentConn{AssetID: "lab-host-01"}, agentmgr.Message{
			Type: agentmgr.MsgCommandResult,
			ID:   req.JobID,
			Data: raw,
		})
	}()

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)
	<-done

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting persistent session over terminal-scoped agent cleanup, got %d", deleteRec.Code)
	}
}
