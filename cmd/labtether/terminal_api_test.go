package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	ctx = contextWithScopes(ctx, []string{"terminal:read", "credentials:use"})
	ctx = contextWithAllowedAssets(ctx, []string{"srv1"})
	ctx = contextWithAPIKeyID(ctx, "key-stream-01")
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
	if got := scopesFromContext(authReq.Context()); len(got) != 2 || got[0] != "terminal:read" || got[1] != "credentials:use" {
		t.Fatalf("expected ticket scopes to be restored, got %#v", got)
	}
	if got := allowedAssetsFromContext(authReq.Context()); len(got) != 1 || got[0] != "srv1" {
		t.Fatalf("expected ticket asset allowlist to be restored, got %#v", got)
	}
	if got := apiKeyIDFromContext(authReq.Context()); got != "key-stream-01" {
		t.Fatalf("expected ticket API key id to be restored, got %q", got)
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

func TestTerminalCommandAuditDoesNotPersistRawCommand(t *testing.T) {
	sut := newTestAPIServer(t)
	sessionID := mustCreateSession(t, sut)
	secretCommand := "uptime"
	payload, err := json.Marshal(map[string]any{"command": secretCommand})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/terminal/sessions/"+sessionID+"/commands", bytes.NewReader(payload))
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", auth.RoleOwner))
	rec := httptest.NewRecorder()

	sut.handleSessionActions(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want queue-unavailable 503", rec.Code)
	}

	events, err := sut.auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"command":"`+secretCommand+`"`) {
		t.Fatalf("audit events persisted raw terminal command: %s", encoded)
	}
	foundMetadata := false
	for _, event := range events {
		if event.Type == "terminal.command.policy_checked" {
			if got := event.Details["command_bytes"]; got == len([]byte(secretCommand)) {
				foundMetadata = true
			}
		}
	}
	if !foundMetadata {
		t.Fatal("expected non-sensitive command length metadata in policy audit")
	}
}

func TestQuickSessionCredentialsRemainEphemeralAndResolveAfterPersistentReload(t *testing.T) {
	sut := newTestAPIServer(t)
	deps := sut.ensureTerminalDeps()
	secret := "LTQA_QUICK_CONNECT_SECRET_62bc"
	body, err := json.Marshal(map[string]any{
		"host": "192.0.2.10", "port": 22, "username": "qa-user",
		"auth_method": "password", "password": secret, "strict_host_key": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/terminal/quick-session", bytes.NewReader(body))
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", auth.RoleOwner))
	rec := httptest.NewRecorder()
	deps.HandleQuickSession(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("quick-session response exposed the credential")
	}
	var response struct {
		Session terminal.Session `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	stored, ok, err := sut.terminalStore.GetSession(response.Session.ID)
	if err != nil || !ok {
		t.Fatalf("reload persistent session: ok=%t err=%v", ok, err)
	}
	if stored.InlineSSHConfig != nil {
		t.Fatal("persistent terminal store retained inline SSH credentials")
	}
	resolved, err := deps.ResolveSessionSSHConfig(stored)
	if err != nil {
		t.Fatalf("resolve ephemeral quick-session config: %v", err)
	}
	if resolved.Password != secret || resolved.Host != "192.0.2.10" || resolved.User != "qa-user" {
		t.Fatal("resolved quick-session config did not match the process-local credential")
	}

	events, err := sut.auditStore.List(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	encodedEvents, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedEvents), secret) {
		t.Fatal("quick-session audit persisted the credential")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/terminal/sessions/"+stored.ID, nil)
	deleteReq = deleteReq.WithContext(contextWithPrincipal(deleteReq.Context(), "owner", auth.RoleOwner))
	deleteRec := httptest.NewRecorder()
	deps.HandleSessionActions(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d, want 200; body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, ok := deps.EphemeralSSHConfigs.Get(stored.ID); ok {
		t.Fatal("session deletion did not remove ephemeral SSH credentials")
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
