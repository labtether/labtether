package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
)

type failingTerminalStore struct {
	persistence.TerminalStore
	createErr error
	listErr   error
	deleteErr error
}

func (s failingTerminalStore) CreateSession(req terminal.CreateSessionRequest) (terminal.Session, error) {
	if s.createErr != nil {
		return terminal.Session{}, s.createErr
	}
	return s.TerminalStore.CreateSession(req)
}

func (s failingTerminalStore) ListSessions() ([]terminal.Session, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.TerminalStore.ListSessions()
}

func (s failingTerminalStore) DeleteTerminalSession(id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.TerminalStore.DeleteTerminalSession(id)
}

func TestCreateSession(t *testing.T) {
	sut := newTestAPIServer(t)

	body := map[string]any{
		"actor_id": "owner",
		"target":   "lab-host-01",
		"mode":     "interactive",
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	sut.handleSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestBlockedCommandIsDenied(t *testing.T) {
	sut := newTestAPIServer(t)

	sessionID := mustCreateSession(t, sut)

	cmdPayload := map[string]any{
		"actor_id": "owner",
		"command":  "rm -rf /",
	}
	payload, _ := json.Marshal(cmdPayload)

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions/"+sessionID+"/commands", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	sut.handleSessionActions(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestStreamTicketLifecycle(t *testing.T) {
	sut := newTestAPIServer(t)
	sessionID := mustCreateSession(t, sut)

	ctx := contextWithPrincipal(context.Background(), "usr-stream-01", auth.RoleAdmin)
	ticket, _, err := sut.issueStreamTicket(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to issue stream ticket: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+sessionID+"/stream?ticket="+ticket, nil)
	authReq, ok := sut.consumeStreamTicketAuth(req)
	if !ok {
		t.Fatalf("expected stream ticket auth to pass")
	}
	if got := userIDFromContext(authReq.Context()); got != "usr-stream-01" {
		t.Fatalf("expected ticket actor to be restored, got %q", got)
	}
	if got := userRoleFromContext(authReq.Context()); got != auth.RoleAdmin {
		t.Fatalf("expected ticket role to be restored, got %q", got)
	}

	replayReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+sessionID+"/stream?ticket="+ticket, nil)
	if _, ok := sut.consumeStreamTicketAuth(replayReq); ok {
		t.Fatalf("expected one-time stream ticket to fail on replay")
	}
}

func TestSessionStreamTicketEndpoint(t *testing.T) {
	sut := newTestAPIServer(t)
	sessionID := mustCreateSession(t, sut)

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions/"+sessionID+"/stream-ticket", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", auth.RoleOwner))
	rec := httptest.NewRecorder()
	sut.handleSessionActions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var payload struct {
		SessionID  string `json:"session_id"`
		Ticket     string `json:"ticket"`
		StreamPath string `json:"stream_path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode stream ticket response: %v", err)
	}
	if payload.SessionID != sessionID {
		t.Fatalf("expected session id %s, got %s", sessionID, payload.SessionID)
	}
	if payload.Ticket == "" {
		t.Fatalf("expected non-empty ticket")
	}
	if !strings.Contains(payload.StreamPath, "/terminal/sessions/"+sessionID+"/stream") {
		t.Fatalf("unexpected stream path: %s", payload.StreamPath)
	}
}

func TestCreateSessionUsesAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)

	body := map[string]any{
		"actor_id": "spoofed-actor",
		"target":   "lab-host-01",
		"mode":     "interactive",
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader(payload))
	req = req.WithContext(contextWithUserID(req.Context(), "usr-session-01"))
	rec := httptest.NewRecorder()

	sut.handleSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var response struct {
		Session struct {
			ActorID string `json:"actor_id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Session.ActorID != "usr-session-01" {
		t.Fatalf("expected actor_id to be context user, got %q", response.Session.ActorID)
	}
}

func TestCreateCommandUsesAuthenticatedActor(t *testing.T) {
	sut := newTestAPIServer(t)
	createPayload := []byte(`{"actor_id":"ignored","target":"lab-host-01","mode":"interactive"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "usr-command-01"))
	createRec := httptest.NewRecorder()
	sut.handleSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating session, got %d", createRec.Code)
	}
	var created struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode create session response: %v", err)
	}
	if created.Session.ID == "" {
		t.Fatal("expected session ID")
	}
	sessionID := created.Session.ID

	cmdPayload := map[string]any{
		"actor_id": "spoofed-actor",
		"command":  "uptime",
	}
	payload, _ := json.Marshal(cmdPayload)

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions/"+sessionID+"/commands", bytes.NewReader(payload))
	req = req.WithContext(contextWithUserID(req.Context(), "usr-command-01"))
	rec := httptest.NewRecorder()

	sut.handleSessionActions(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when queue is unavailable, got %d", rec.Code)
	}

	commands, err := sut.terminalStore.ListCommands(sessionID)
	if err != nil {
		t.Fatalf("failed to list commands: %v", err)
	}
	if len(commands) == 0 {
		t.Fatal("expected at least one command")
	}
	if commands[0].ActorID != "usr-command-01" {
		t.Fatalf("expected command actor_id to be context user, got %q", commands[0].ActorID)
	}
}

func TestSessionActionsDenyNonOwnerCrossActorAccess(t *testing.T) {
	sut := newTestAPIServer(t)

	createPayload := []byte(`{"actor_id":"ignored","target":"lab-host-01","mode":"interactive"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader(createPayload))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handleSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating session, got %d", createRec.Code)
	}

	var created struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create session response: %v", err)
	}
	if created.Session.ID == "" {
		t.Fatal("expected session id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+created.Session.ID, nil)
	getReq = getReq.WithContext(contextWithUserID(getReq.Context(), "actor-b"))
	getRec := httptest.NewRecorder()
	sut.handleSessionActions(getRec, getReq)

	if getRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-actor session access, got %d", getRec.Code)
	}
}

func TestHandleSessionsFiltersListForNonOwnerActor(t *testing.T) {
	sut := newTestAPIServer(t)

	createAs := func(actor string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","mode":"interactive"}`)))
		req = req.WithContext(contextWithUserID(req.Context(), actor))
		rec := httptest.NewRecorder()
		sut.handleSessions(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 creating session for %s, got %d", actor, rec.Code)
		}
	}
	createAs("actor-a")
	createAs("actor-b")

	listReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil)
	listReq = listReq.WithContext(contextWithUserID(listReq.Context(), "actor-a"))
	listRec := httptest.NewRecorder()
	sut.handleSessions(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing sessions, got %d", listRec.Code)
	}

	var response struct {
		Sessions []struct {
			ActorID string `json:"actor_id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(response.Sessions) != 1 {
		t.Fatalf("expected one visible session for actor-a, got %d", len(response.Sessions))
	}
	if response.Sessions[0].ActorID != "actor-a" {
		t.Fatalf("expected actor-a session, got %q", response.Sessions[0].ActorID)
	}
}

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

func TestCreateSessionWithoutAuditStoreStillSucceeds(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.auditStore = nil

	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions", bytes.NewReader([]byte(`{"target":"lab-host-01","mode":"interactive"}`)))
	rec := httptest.NewRecorder()
	sut.handleSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating session without audit store, got %d", rec.Code)
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

func TestDeletePersistentSessionRequiresRemoteCleanupPath(t *testing.T) {
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

	if deleteRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when remote cleanup path is unavailable, got %d", deleteRec.Code)
	}

	_, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.PersistentSession.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if !ok {
		t.Fatal("expected persistent session to remain when delete cleanup cannot run")
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

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "lab-host-01", "linux"))
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

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound cleanup message: %v", err)
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

func TestSessionDeleteRemovesSession(t *testing.T) {
	sut := newTestAPIServer(t)
	sessionID := mustCreateSession(t, sut)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/sessions/"+sessionID, nil)
	deleteRec := httptest.NewRecorder()
	sut.handleSessionActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting session, got %d", deleteRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+sessionID, nil)
	getRec := httptest.NewRecorder()
	sut.handleSessionActions(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deleting session, got %d", getRec.Code)
	}
}
