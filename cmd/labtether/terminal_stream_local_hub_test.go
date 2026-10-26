package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/terminal"
)

func TestIsHubLocalTerminalTarget(t *testing.T) {
	if !isHubLocalTerminalTarget("__hub__") {
		t.Fatal("expected __hub__ to resolve as hub local terminal target")
	}
	if isHubLocalTerminalTarget("lab-host-01") {
		t.Fatal("expected regular asset target to not resolve as hub local terminal target")
	}
}

func TestHandleSessionStreamRejectsHubLocalWhenDisabled(t *testing.T) {
	t.Setenv(envEnableHubLocalTerminal, "false")
	sut := newTestAPIServer(t)
	session := createHubLocalSession(t, sut)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+session.ID+"/stream", nil)
	rec := httptest.NewRecorder()
	sut.handleSessionStream(rec, req, session)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when hub local terminal is disabled, got %d", rec.Code)
	}
}

func TestHandleSessionStreamRejectsHubLocalForNonOwner(t *testing.T) {
	t.Setenv(envEnableHubLocalTerminal, "true")
	sut := newTestAPIServer(t)
	session := createHubLocalSession(t, sut)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/"+session.ID+"/stream", nil)
	req = req.WithContext(contextWithUserID(req.Context(), "analyst-user"))
	rec := httptest.NewRecorder()
	sut.handleSessionStream(rec, req, session)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner hub local terminal access, got %d", rec.Code)
	}
}

// Streams open only for stored sessions (unknown IDs get 404), so the
// hub-local policy checks need a real session row to reach them.
func createHubLocalSession(t *testing.T, sut *apiServer) terminal.Session {
	t.Helper()
	session, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "owner",
		Target:  "__hub__",
		Mode:    "interactive",
	})
	if err != nil {
		t.Fatalf("create hub local session: %v", err)
	}
	return session
}
