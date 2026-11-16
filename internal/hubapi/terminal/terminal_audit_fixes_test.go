package terminal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
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

func mustCreateBookmark(t *testing.T, sut *hubdeps.Hub, actorID string, body string) terminal.Bookmark {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/terminal/bookmarks", bytes.NewBufferString(body))
	req = req.WithContext(apiv2.ContextWithUserID(req.Context(), actorID))
	rec := httptest.NewRecorder()
	sut.TerminalDeps().HandleBookmarks(rec, req)

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

func mustCreatePersistentSession(t *testing.T, sut *hubdeps.Hub, actorID, target string) terminal.PersistentSession {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewBufferString(`{"target":"`+target+`","title":"Shell"}`))
	req = req.WithContext(apiv2.ContextWithUserID(req.Context(), actorID))
	rec := httptest.NewRecorder()
	sut.TerminalDeps().HandlePersistentSessions(rec, req)

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
	sut := hubdeps.New(t)
	created := mustCreatePersistentSession(t, sut, "actor-a", "lab-host-01")

	sut.TerminalDeps().IssueStreamTicket = func(_ context.Context, _ string) (string, time.Time, error) {
		return "", time.Time{}, errors.New("synthetic stream ticket failure")
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.ID+"/attach", nil)
	attachReq = attachReq.WithContext(apiv2.ContextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.TerminalDeps().HandlePersistentSessionActions(attachRec, attachReq)

	if attachRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 attaching persistent session when ticket issuance fails, got %d", attachRec.Code)
	}

	persistent, ok, err := sut.TerminalPersistentStore.GetPersistentSession(created.ID)
	if err != nil {
		t.Fatalf("failed to reload persistent session: %v", err)
	}
	if !ok {
		t.Fatal("expected persistent session to remain after stream ticket failure")
	}
	if persistent.Status != "detached" {
		t.Fatalf("expected persistent session to remain detached, got %q", persistent.Status)
	}

	sessions, err := sut.TerminalStore.ListSessions()
	if err != nil {
		t.Fatalf("failed to list terminal sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no terminal sessions to remain after ticket failure, got %d", len(sessions))
	}
}

func TestConnectBookmarkDoesNotMarkAttachedWhenStreamTicketIssueFails(t *testing.T) {
	sut := hubdeps.New(t)
	sut.TerminalBookmarkStore = testutil.NewMemoryTerminalBookmarkStore()
	sut.ResetTerminalDeps()
	bookmark := mustCreateBookmark(t, sut, "actor-a", `{"title":"Lab Host","asset_id":"lab-host-01"}`)

	sut.TerminalDeps().IssueStreamTicket = func(_ context.Context, _ string) (string, time.Time, error) {
		return "", time.Time{}, errors.New("synthetic stream ticket failure")
	}

	connectReq := httptest.NewRequest(http.MethodPost, "/terminal/bookmarks/"+bookmark.ID+"/connect", bytes.NewBufferString(`{}`))
	connectReq = connectReq.WithContext(apiv2.ContextWithUserID(connectReq.Context(), "actor-a"))
	connectRec := httptest.NewRecorder()
	sut.TerminalDeps().HandleBookmarkActions(connectRec, connectReq)

	if connectRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 connecting bookmark when ticket issuance fails, got %d", connectRec.Code)
	}

	persistentSessions, err := sut.TerminalPersistentStore.ListPersistentSessions()
	if err != nil {
		t.Fatalf("failed to list persistent sessions: %v", err)
	}
	if len(persistentSessions) != 1 {
		t.Fatalf("expected one persistent session to remain, got %d", len(persistentSessions))
	}
	if persistentSessions[0].Status != "detached" {
		t.Fatalf("expected bookmark-created persistent session to remain detached, got %q", persistentSessions[0].Status)
	}

	sessions, err := sut.TerminalStore.ListSessions()
	if err != nil {
		t.Fatalf("failed to list terminal sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no terminal sessions to remain after ticket failure, got %d", len(sessions))
	}
}

func TestUpdateBookmarkRejectsEmptyResolvedTarget(t *testing.T) {
	sut := hubdeps.New(t)
	sut.TerminalBookmarkStore = testutil.NewMemoryTerminalBookmarkStore()
	sut.ResetTerminalDeps()
	bookmark := mustCreateBookmark(t, sut, "actor-a", `{"title":"Lab Host","host":"lab-host-01","port":22}`)

	updateReq := httptest.NewRequest(http.MethodPut, "/terminal/bookmarks/"+bookmark.ID, bytes.NewBufferString(`{"asset_id":"","host":""}`))
	updateReq = updateReq.WithContext(apiv2.ContextWithUserID(updateReq.Context(), "actor-a"))
	updateRec := httptest.NewRecorder()
	sut.TerminalDeps().HandleBookmarkActions(updateRec, updateReq)

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 updating bookmark to an empty target, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	reloaded, ok, err := sut.TerminalBookmarkStore.GetBookmark(bookmark.ID)
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
	sut := hubdeps.New(t)
	trackingStore := &trackingPersistentSessionStore{MemoryTerminalStore: persistence.NewMemoryTerminalStore()}
	sut.TerminalPersistentStore = trackingStore
	sut.ResetTerminalDeps()

	mustCreatePersistentSession(t, sut, "actor-a", "lab-host-01")
	mustCreatePersistentSession(t, sut, "actor-b", "lab-host-02")

	listReq := httptest.NewRequest(http.MethodGet, "/terminal/persistent-sessions", nil)
	listReq = listReq.WithContext(apiv2.ContextWithUserID(listReq.Context(), "actor-a"))
	listRec := httptest.NewRecorder()
	sut.TerminalDeps().HandlePersistentSessions(listRec, listReq)

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
