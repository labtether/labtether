package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/terminal"
)

func TestStartRecordingRequestRejectsNonBridgeEntry(t *testing.T) {
	sut := newTestAPIServer(t)
	session, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "owner",
		Target:  "desktop-node-01",
		Mode:    "desktop",
	})
	if err != nil {
		t.Fatalf("create desktop session: %v", err)
	}
	sut.desktopBridges.Store(session.ID, "ignore-me")

	req := httptest.NewRequest(http.MethodPost, "/recordings", strings.NewReader(`{"session_id":"`+session.ID+`"}`))
	req = req.WithContext(contextWithPrincipal(context.Background(), "owner", "owner"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	sut.startRecordingRequest(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "desktop session bridge not found") {
		t.Fatalf("expected desktop bridge not found error, got %q", rr.Body.String())
	}
}

func TestStopRecordingBySessionIgnoresNonBridgeEntry(t *testing.T) {
	var sut apiServer
	sut.desktopBridges.Store("sess-non-bridge", "ignore-me")

	if stopped := sut.stopRecordingBySession("sess-non-bridge"); stopped {
		t.Fatal("expected stopRecordingBySession to return false for non-bridge entries")
	}
}

func TestAuthorizeRecordingSessionAccessHonorsOwnerAndAdminSemantics(t *testing.T) {
	sut := newTestAPIServer(t)
	session, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "actor-a",
		Target:  "desktop-node-01",
		Mode:    "desktop",
	})
	if err != nil {
		t.Fatalf("create desktop session: %v", err)
	}

	operatorReq := httptest.NewRequest(http.MethodPost, "/recordings/"+session.ID, nil)
	operatorReq = operatorReq.WithContext(contextWithPrincipal(operatorReq.Context(), "actor-b", "operator"))
	operatorRec := httptest.NewRecorder()
	if sut.authorizeRecordingSessionAccess(operatorRec, operatorReq, session.ID) {
		t.Fatal("expected cross-actor operator access to be denied")
	}
	if operatorRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-actor operator access, got %d", operatorRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodPost, "/recordings/"+session.ID, nil)
	adminReq = adminReq.WithContext(contextWithPrincipal(adminReq.Context(), "actor-b", "admin"))
	adminRec := httptest.NewRecorder()
	if !sut.authorizeRecordingSessionAccess(adminRec, adminReq, session.ID) {
		t.Fatal("expected admin recording access to be allowed")
	}
}

func TestCanAccessRecordingMetadataUsesDesktopSessionOwnership(t *testing.T) {
	sut := newTestAPIServer(t)
	session, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "actor-a",
		Target:  "desktop-node-01",
		Mode:    "desktop",
	})
	if err != nil {
		t.Fatalf("create desktop session: %v", err)
	}

	ownerCtx := contextWithPrincipal(context.Background(), "actor-a", "operator")
	if !sut.canAccessRecordingMetadata(ownerCtx, session.ID, "admin-user") {
		t.Fatal("expected session owner to access recording metadata even when another actor started the recording")
	}

	otherCtx := contextWithPrincipal(context.Background(), "actor-b", "operator")
	if sut.canAccessRecordingMetadata(otherCtx, session.ID, "admin-user") {
		t.Fatal("expected unrelated operator to be denied recording metadata")
	}
}

func TestRecordingResponsePayloadOmitsSensitiveMetadata(t *testing.T) {
	payload := recordingResponsePayload(recordingMetadata{
		ID:        "rec-1",
		SessionID: "sess-1",
		AssetID:   "asset-1",
		ActorID:   "actor-a",
		Protocol:  "vnc",
		FilePath:  "/srv/data/recordings/rec-1.bin",
	})

	if _, ok := payload["file_path"]; ok {
		t.Fatal("expected recording payload to omit file_path")
	}
	if _, ok := payload["actor_id"]; ok {
		t.Fatal("expected recording payload to omit actor_id")
	}
}
