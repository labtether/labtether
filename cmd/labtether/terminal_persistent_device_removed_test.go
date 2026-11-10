package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/terminal"
)

// attachedSavedSessionOnDevice registers nas01 as a device and gives actor-a a
// saved session on it that has been attached once, so removing it means
// ending a shell on the device. No agent is connected and no SSH login exists.
func attachedSavedSessionOnDevice(t *testing.T, sut *apiServer) terminal.PersistentSession {
	t.Helper()
	t.Setenv("SSH_USERNAME", "")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "nas01", Type: "server", Name: "NAS 01", Source: "agent",
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader([]byte(`{"target":"nas01","title":"Ops Shell"}`)))
	createReq = createReq.WithContext(contextWithUserID(createReq.Context(), "actor-a"))
	createRec := httptest.NewRecorder()
	sut.handlePersistentSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create saved session: got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	attachReq := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions/"+created.PersistentSession.ID+"/attach", nil)
	attachReq = attachReq.WithContext(contextWithUserID(attachReq.Context(), "actor-a"))
	attachRec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(attachRec, attachReq)
	if attachRec.Code != http.StatusCreated {
		t.Fatalf("attach saved session: got %d: %s", attachRec.Code, attachRec.Body.String())
	}
	return created.PersistentSession
}

func deleteSavedSession(sut *apiServer, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/terminal/persistent-sessions/"+id, nil)
	req = req.WithContext(contextWithUserID(req.Context(), "actor-a"))
	rec := httptest.NewRecorder()
	sut.handlePersistentSessionActions(rec, req)
	return rec
}

func savedSessionRemains(t *testing.T, sut *apiServer, id string) bool {
	t.Helper()
	_, ok, err := sut.terminalPersistentStore.GetPersistentSession(id)
	if err != nil {
		t.Fatalf("reload saved session: %v", err)
	}
	return ok
}

// Fault: a saved session left behind by a deleted device answered 409 for
// good, because the Hub waited for a device that could never connect again.
func TestDeleteSavedSessionSucceedsWhenItsDeviceIsGone(t *testing.T) {
	sut := newTestAPIServer(t)
	saved := attachedSavedSessionOnDevice(t, sut)
	// Remove only the device record, as a Hub without the delete cascade did.
	if err := sut.assetStore.DeleteAsset("nas01"); err != nil {
		t.Fatalf("remove device record: %v", err)
	}

	rec := deleteSavedSession(sut, saved.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a saved session whose device is gone, got %d: %s", rec.Code, rec.Body.String())
	}
	if savedSessionRemains(t, sut, saved.ID) {
		t.Fatal("saved session record remains after its delete succeeded")
	}
}

// Promise kept: while the device is still in the Hub but unreachable, the Hub
// does not claim the shell is gone, and says what to do in plain words.
func TestDeleteSavedSessionStillRefusedWhileDeviceExistsButIsAway(t *testing.T) {
	sut := newTestAPIServer(t)
	saved := attachedSavedSessionOnDevice(t, sut)

	rec := deleteSavedSession(sut, saved.ID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 while the device exists but is away, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	const want = "This session runs on NAS 01, which is not connected. Connect it, then remove the session."
	if body.Error != want {
		t.Fatalf("refusal text = %q, want %q", body.Error, want)
	}
	if !savedSessionRemains(t, sut, saved.ID) {
		t.Fatal("saved session was removed although its device may still run the shell")
	}
}

// Fault: deleting a device left its saved sessions in people's lists.
func TestDeleteDeviceRemovesItsSavedSessions(t *testing.T) {
	sut := newTestAPIServer(t)
	saved := attachedSavedSessionOnDevice(t, sut)

	req := httptest.NewRequest(http.MethodDelete, "/assets/nas01", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete device: got %d: %s", rec.Code, rec.Body.String())
	}

	if savedSessionRemains(t, sut, saved.ID) {
		t.Fatal("saved session remains after its device was deleted")
	}
	sessions, err := sut.terminalStore.ListSessions()
	if err != nil {
		t.Fatalf("list terminal sessions: %v", err)
	}
	for _, session := range sessions {
		if session.PersistentSessionID == saved.ID {
			t.Fatalf("attached terminal session %s remains after its device was deleted", session.ID)
		}
	}
}
