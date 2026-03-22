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

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess-1/stream", nil)
	rec := httptest.NewRecorder()
	sut.handleSessionStream(rec, req, terminal.Session{ID: "sess-1", Target: "__hub__"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when hub local terminal is disabled, got %d", rec.Code)
	}
}

func TestHandleSessionStreamRejectsHubLocalForNonOwner(t *testing.T) {
	t.Setenv(envEnableHubLocalTerminal, "true")
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/terminal/sessions/sess-2/stream", nil)
	req = req.WithContext(contextWithUserID(req.Context(), "analyst-user"))
	rec := httptest.NewRecorder()
	sut.handleSessionStream(rec, req, terminal.Session{ID: "sess-2", Target: "__hub__"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner hub local terminal access, got %d", rec.Code)
	}
}
