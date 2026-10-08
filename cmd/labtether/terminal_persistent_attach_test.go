package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersistentSessionCreateAttachAndDeleteLifecycle(t *testing.T) {
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
		t.Fatalf("failed to decode persistent session create response: %v", err)
	}
	if created.PersistentSession.ID == "" {
		t.Fatal("expected persistent session id")
	}
	if created.PersistentSession.ActorID != "actor-a" {
		t.Fatalf("expected actor-a owner, got %q", created.PersistentSession.ActorID)
	}
	if created.PersistentSession.TmuxSessionName == "" {
		t.Fatal("expected stable tmux session name")
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)

	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 attaching persistent session, got %d", attachRec.Code)
	}

	var attached struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
		Session           terminal.Session           `json:"session"`
	}
	if err := json.Unmarshal(attachRec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("failed to decode attach response: %v", err)
	}
	if attached.Session.PersistentSessionID != created.PersistentSession.ID {
		t.Fatalf("expected session to reference persistent id %q, got %q", created.PersistentSession.ID, attached.Session.PersistentSessionID)
	}
	if attached.Session.TmuxSessionName != created.PersistentSession.TmuxSessionName {
		t.Fatalf("expected stable tmux session name %q, got %q", created.PersistentSession.TmuxSessionName, attached.Session.TmuxSessionName)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+created.PersistentSession.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithUserID(deleteReq.Context(), "actor-a"))
	deleteRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting persistent session, got %d", deleteRec.Code)
	}

	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("failed to list terminal sessions: %v", err)
	}
	for _, session := range sessions {
		if session.PersistentSessionID == created.PersistentSession.ID {
			t.Fatalf("expected attached terminal sessions for %s to be deleted", created.PersistentSession.ID)
		}
	}
}

func TestPersistentSessionCreateAppliesInteractivePolicy(t *testing.T) {
	sut := newTestAPIServer(t)
	cfg := policy.DefaultEvaluatorConfig()
	cfg.InteractiveEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Ops Shell"}`)))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)

	if createRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 creating persistent session when interactive mode is disabled, got %d", createRec.Code)
	}
}

func TestCreatePersistentSessionRejectsConnectedAgentWithoutTmux(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	agentConn := agentmgr.NewAgentConn(serverConn, "lab-host-01", "linux")
	agentConn.SetMeta("terminal.tmux.has", "false")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("lab-host-01")
	sut.resetTerminalDepsForTest()

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Ops Shell"}`)))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)

	if createRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 creating a persistent session without tmux, got %d: %s", createRec.Code, createRec.Body.String())
	}
	persistentSessions, err := sut.terminalPersistentStore.ListPersistentSessions()
	if err != nil {
		t.Fatalf("list persistent sessions: %v", err)
	}
	if len(persistentSessions) != 0 {
		t.Fatalf("expected no persistent metadata after capability rejection, got %d records", len(persistentSessions))
	}
	_ = waitForAgentTerminalMessage(t, clientConn, agentmgr.MsgTerminalProbe)
}

func TestAttachPersistentSessionAppliesInteractivePolicy(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Ops Shell"}`)))
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

	cfg := policy.DefaultEvaluatorConfig()
	cfg.InteractiveEnabled = false
	sut.policyState = newPolicyRuntimeState(cfg)
	sut.resetTerminalDepsForTest() // reset cached deps to pick up new policyState

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)

	if attachRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 attaching persistent session when interactive mode is disabled, got %d", attachRec.Code)
	}
}

func TestAttachPersistentSessionRejectsConnectedAgentWithoutTmuxAndStaysDetached(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Ops Shell"}`)))
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

	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	agentConn := agentmgr.NewAgentConn(serverConn, "lab-host-01", "linux")
	agentConn.SetMeta("terminal.tmux.has", "false")
	sut.agentMgr.Register(agentConn)
	defer sut.agentMgr.Unregister("lab-host-01")
	sut.resetTerminalDepsForTest()

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 attaching a persistent session without tmux, got %d: %s", attachRec.Code, attachRec.Body.String())
	}

	persistent, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil || !ok {
		t.Fatalf("reload persistent session: ok=%t err=%v", ok, err)
	}
	if persistent.Status != "detached" || persistent.LastAttachedAt != nil {
		t.Fatalf("capability rejection created false attached state: %+v", persistent)
	}
	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("list terminal sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no attached terminal session after capability rejection, got %d", len(sessions))
	}
	_ = waitForAgentTerminalMessage(t, clientConn, agentmgr.MsgTerminalProbe)
}

func TestAttachPersistentSessionReturnsUpdatedAttachedState(t *testing.T) {
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

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 attaching persistent session, got %d", attachRec.Code)
	}

	var attached struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
		Session           terminal.Session           `json:"session"`
	}
	if err := json.Unmarshal(attachRec.Body.Bytes(), &attached); err != nil {
		t.Fatalf("failed to decode attach response: %v", err)
	}
	if got := strings.TrimSpace(attached.PersistentSession.Status); got != "attached" {
		t.Fatalf("expected attached persistent status, got %q", got)
	}
	if attached.PersistentSession.LastAttachedAt == nil {
		t.Fatal("expected attached persistent session timestamp")
	}
	if attached.Session.PersistentSessionID != created.PersistentSession.ID {
		t.Fatalf("expected session to link to persistent session %q, got %q", created.PersistentSession.ID, attached.Session.PersistentSessionID)
	}
}

func TestAttachPersistentSessionDoesNotMarkAttachedWhenSessionCreateFails(t *testing.T) {
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

	sut.terminalStore = failingTerminalStore{
		TerminalStore: sut.terminalStore,
		createErr:     errors.New("synthetic create failure"),
	}
	sut.resetTerminalDepsForTest() // reset cached deps to pick up new terminalStore

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 attaching persistent session when create fails, got %d", attachRec.Code)
	}

	persistent, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if !ok {
		t.Fatal("expected persistent session to remain after attach failure")
	}
	if got := strings.TrimSpace(persistent.Status); got != "detached" {
		t.Fatalf("expected persistent session to remain detached, got %q", got)
	}
	if persistent.LastAttachedAt != nil {
		t.Fatal("expected no attached timestamp after failed attach")
	}
}

func TestPersistentSessionListFiltersByAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)

	createAs := func(actor, target string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"`+target+`","title":"Shell"}`)))
		req = req.WithContext(contextWithUserID(req.Context(), actor))
		rec := httptest.NewRecorder()
		sut.handlePersistentSessions(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 creating persistent session for %s, got %d", actor, rec.Code)
		}
	}
	createAs("actor-a", "lab-host-01")
	createAs("actor-b", "lab-host-02")

	listReq := httptest.NewRequest(http.MethodGet, "/terminal/persistent-sessions", nil)
	listReq = listReq.WithContext(contextWithUserID(listReq.Context(), "actor-a"))
	listRec := httptest.NewRecorder()
	sut.handlePersistentSessions(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing persistent sessions, got %d", listRec.Code)
	}

	var response struct {
		PersistentSessions []terminal.PersistentSession `json:"persistent_sessions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode persistent session list response: %v", err)
	}
	if len(response.PersistentSessions) != 1 {
		t.Fatalf("expected one visible persistent session for actor-a, got %d", len(response.PersistentSessions))
	}
	if response.PersistentSessions[0].ActorID != "actor-a" {
		t.Fatalf("expected actor-a session, got %q", response.PersistentSessions[0].ActorID)
	}
}
