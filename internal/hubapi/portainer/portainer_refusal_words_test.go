package portainer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	portainerconnector "github.com/labtether/labtether/internal/connectors/portainer"
)

// A compose edit while Portainer was still deploying answered 502 "An internal
// error occurred." although Portainer said why ("Stack deployment is already
// in progress").
func TestPortainerRefusalKeepsPortainersWords(t *testing.T) {
	refused := &portainerconnector.APIError{StatusCode: http.StatusConflict,
		Body:    `{"message":"Unable to update stack","details":"Stack deployment is already in progress"}`,
		Message: "Unable to update stack", Details: "Stack deployment is already in progress"}
	rec := httptest.NewRecorder()
	writePortainerUpstreamError(rec, refused)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Stack deployment is already in progress") {
		t.Fatalf("refusal = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	writePortainerUpstreamError(rec, errors.New("portainer api returned 500: boom"))
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("server error = %d %s, want a sanitized 502", rec.Code, rec.Body.String())
	}
}

// Portainer's 4xx details can carry compose output with registry URLs that
// hold a user:token, socket paths and several lines; none may reach the user.
func TestPortainerRefusalReasonIsSanitizedOneLine(t *testing.T) {
	refused := &portainerconnector.APIError{StatusCode: http.StatusBadRequest,
		Details: "pull access denied for https://user:secret@registry.example/v2/app\nunix:///var/run/docker.sock refused " + strings.Repeat("x", 400)}
	rec := httptest.NewRecorder()
	writePortainerUpstreamError(rec, refused)
	body := rec.Body.String()
	if rec.Code != http.StatusConflict || strings.Contains(body, "secret") || strings.Contains(body, "registry.example") ||
		strings.Contains(body, "docker.sock") || strings.Contains(body, `\n`) || strings.Contains(body, "\n\"") {
		t.Fatalf("refusal = %d %s", rec.Code, body)
	}
	if !strings.Contains(body, "…") || len([]rune(body)) > 320 {
		t.Fatalf("long reason not cut to one short line: %s", body)
	}
}
