package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

type trackingPersistentSessionStore struct {
	*persistence.MemoryTerminalStore
	listAllCalls   int
	listActorCalls int
}

func (s *trackingPersistentSessionStore) ListPersistentSessions() ([]terminal.PersistentSession, error) {
	s.listAllCalls++
	return s.MemoryTerminalStore.ListPersistentSessions()
}

func (s *trackingPersistentSessionStore) ListPersistentSessionsByActor(actorID string) ([]terminal.PersistentSession, error) {
	s.listActorCalls++
	return s.MemoryTerminalStore.ListPersistentSessionsByActor(actorID)
}

type memoryTerminalBookmarkStore struct {
	mu        sync.RWMutex
	bookmarks map[string]terminal.Bookmark
	nextID    int
}

func newMemoryTerminalBookmarkStore() *memoryTerminalBookmarkStore {
	return &memoryTerminalBookmarkStore{bookmarks: make(map[string]terminal.Bookmark)}
}

func (s *memoryTerminalBookmarkStore) CreateBookmark(req terminal.CreateBookmarkRequest) (terminal.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	now := time.Now().UTC()
	bookmark := terminal.Bookmark{
		ID:                  "bkm-test-" + strconv.Itoa(s.nextID),
		ActorID:             strings.TrimSpace(req.ActorID),
		Title:               strings.TrimSpace(req.Title),
		AssetID:             strings.TrimSpace(req.AssetID),
		Host:                strings.TrimSpace(req.Host),
		Port:                req.Port,
		Username:            strings.TrimSpace(req.Username),
		CredentialProfileID: strings.TrimSpace(req.CredentialProfileID),
		JumpChainGroupID:    strings.TrimSpace(req.JumpChainGroupID),
		Tags:                append([]string(nil), req.Tags...),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	s.bookmarks[bookmark.ID] = bookmark
	return bookmark, nil
}

func (s *memoryTerminalBookmarkStore) GetBookmark(id string) (terminal.Bookmark, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	return bookmark, ok, nil
}

func (s *memoryTerminalBookmarkStore) ListBookmarks(actorID string) ([]terminal.Bookmark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]terminal.Bookmark, 0, len(s.bookmarks))
	for _, bookmark := range s.bookmarks {
		if bookmark.ActorID == strings.TrimSpace(actorID) {
			out = append(out, bookmark)
		}
	}
	return out, nil
}

func (s *memoryTerminalBookmarkStore) UpdateBookmark(id string, req terminal.UpdateBookmarkRequest) (terminal.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	if !ok {
		return terminal.Bookmark{}, persistence.ErrNotFound
	}
	if req.Title != nil {
		bookmark.Title = strings.TrimSpace(*req.Title)
	}
	if req.AssetID != nil {
		bookmark.AssetID = strings.TrimSpace(*req.AssetID)
	}
	if req.Host != nil {
		bookmark.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		bookmark.Port = req.Port
	}
	if req.Username != nil {
		bookmark.Username = strings.TrimSpace(*req.Username)
	}
	if req.CredentialProfileID != nil {
		bookmark.CredentialProfileID = strings.TrimSpace(*req.CredentialProfileID)
	}
	if req.JumpChainGroupID != nil {
		bookmark.JumpChainGroupID = strings.TrimSpace(*req.JumpChainGroupID)
	}
	if req.Tags != nil {
		bookmark.Tags = append([]string(nil), req.Tags...)
	}
	bookmark.UpdatedAt = time.Now().UTC()
	s.bookmarks[bookmark.ID] = bookmark
	return bookmark, nil
}

func (s *memoryTerminalBookmarkStore) DeleteBookmark(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.bookmarks[strings.TrimSpace(id)]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.bookmarks, strings.TrimSpace(id))
	return nil
}

func (s *memoryTerminalBookmarkStore) TouchBookmarkLastUsed(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	if !ok {
		return persistence.ErrNotFound
	}
	atCopy := at.UTC()
	bookmark.LastUsedAt = &atCopy
	bookmark.UpdatedAt = atCopy
	s.bookmarks[bookmark.ID] = bookmark
	return nil
}

func mustCreateBookmark(t *testing.T, sut *apiServer, actorID string, body string) terminal.Bookmark {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/terminal/bookmarks", bytes.NewBufferString(body))
	req = req.WithContext(contextWithUserID(req.Context(), actorID))
	rec := httptest.NewRecorder()
	sut.handleBookmarks(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating bookmark, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Bookmark terminal.Bookmark `json:"bookmark"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode bookmark response: %v", err)
	}
	return response.Bookmark
}

func mustCreatePersistentSession(t *testing.T, sut *apiServer, actorID, target string) terminal.PersistentSession {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewBufferString(`{"target":"`+target+`","title":"Shell"}`))
	req = req.WithContext(contextWithUserID(req.Context(), actorID))
	rec := httptest.NewRecorder()
	sut.handlePersistentSessions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating persistent session, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode persistent session response: %v", err)
	}
	return response.PersistentSession
}

func TestAttachPersistentSessionDoesNotMarkAttachedWhenStreamTicketIssueFails(t *testing.T) {
	sut := newTestAPIServer(t)
	created := mustCreatePersistentSession(t, sut, "actor-a", "lab-host-01")

	sut.ensureTerminalDeps().IssueStreamTicket = func(_ context.Context, _ string) (string, time.Time, error) {
		return "", time.Time{}, errors.New("synthetic stream ticket failure")
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)

	if attachRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 attaching persistent session when ticket issuance fails, got %d", attachRec.Code)
	}

	persistent, ok, err := sut.terminalPersistentStore.GetPersistentSession(created.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if !ok {
		t.Fatal("expected persistent session to remain after stream ticket failure")
	}
	if persistent.Status != "detached" {
		t.Fatalf("expected persistent session to remain detached, got %q", persistent.Status)
	}

	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("failed to list terminal sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no terminal sessions to remain after ticket failure, got %d", len(sessions))
	}
}

func TestConnectBookmarkDoesNotMarkAttachedWhenStreamTicketIssueFails(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.terminalBookmarkStore = newMemoryTerminalBookmarkStore()
	sut.resetTerminalDepsForTest()
	bookmark := mustCreateBookmark(t, sut, "actor-a", `{"title":"Lab Host","host":"lab-host-01","port":22}`)

	sut.ensureTerminalDeps().IssueStreamTicket = func(_ context.Context, _ string) (string, time.Time, error) {
		return "", time.Time{}, errors.New("synthetic stream ticket failure")
	}

	connectReq := httptest.NewRequest(http.MethodPost, "/terminal/bookmarks/"+bookmark.ID+"/connect", bytes.NewBufferString(`{}`))
	connectReq = connectReq.WithContext(contextWithUserID(connectReq.Context(), "actor-a"))
	connectRec := httptest.NewRecorder()
	sut.handleBookmarkActions(connectRec, connectReq)

	if connectRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 connecting bookmark when ticket issuance fails, got %d", connectRec.Code)
	}

	persistentSessions, err := sut.terminalPersistentStore.ListPersistentSessions()
	if err != nil {
		t.Fatalf("failed to list persistent sessions: %v", err)
	}
	if len(persistentSessions) != 1 {
		t.Fatalf("expected one persistent session to remain, got %d", len(persistentSessions))
	}
	if persistentSessions[0].Status != "detached" {
		t.Fatalf("expected bookmark-created persistent session to remain detached, got %q", persistentSessions[0].Status)
	}

	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("failed to list terminal sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no terminal sessions to remain after ticket failure, got %d", len(sessions))
	}
}

func TestUpdateBookmarkRejectsEmptyResolvedTarget(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.terminalBookmarkStore = newMemoryTerminalBookmarkStore()
	sut.resetTerminalDepsForTest()
	bookmark := mustCreateBookmark(t, sut, "actor-a", `{"title":"Lab Host","host":"lab-host-01","port":22}`)

	updateReq := httptest.NewRequest(http.MethodPut, "/terminal/bookmarks/"+bookmark.ID, bytes.NewBufferString(`{"asset_id":"","host":""}`))
	updateReq = updateReq.WithContext(contextWithUserID(updateReq.Context(), "actor-a"))
	updateRec := httptest.NewRecorder()
	sut.handleBookmarkActions(updateRec, updateReq)

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 updating bookmark to an empty target, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	reloaded, ok, err := sut.terminalBookmarkStore.GetBookmark(bookmark.ID)
	if err != nil {
		t.Fatalf("failed to reload bookmark: %v", err)
	}
	if !ok {
		t.Fatal("expected bookmark to remain after rejected update")
	}
	if reloaded.Host != "lab-host-01" {
		t.Fatalf("expected bookmark host to remain unchanged, got %q", reloaded.Host)
	}
}

func TestPersistentSessionListUsesActorScopedStoreWhenAvailable(t *testing.T) {
	sut := newTestAPIServer(t)
	trackingStore := &trackingPersistentSessionStore{MemoryTerminalStore: persistence.NewMemoryTerminalStore()}
	sut.terminalPersistentStore = trackingStore
	sut.resetTerminalDepsForTest()

	mustCreatePersistentSession(t, sut, "actor-a", "lab-host-01")
	mustCreatePersistentSession(t, sut, "actor-b", "lab-host-02")

	listReq := httptest.NewRequest(http.MethodGet, "/terminal/persistent-sessions", nil)
	listReq = listReq.WithContext(contextWithUserID(listReq.Context(), "actor-a"))
	listRec := httptest.NewRecorder()
	sut.handlePersistentSessions(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing persistent sessions, got %d: %s", listRec.Code, listRec.Body.String())
	}

	if trackingStore.listActorCalls != 1 {
		t.Fatalf("expected actor-scoped list path to be used once, got %d", trackingStore.listActorCalls)
	}
	if trackingStore.listAllCalls != 0 {
		t.Fatalf("expected full-session list path to be skipped for non-owner requests, got %d", trackingStore.listAllCalls)
	}
}
