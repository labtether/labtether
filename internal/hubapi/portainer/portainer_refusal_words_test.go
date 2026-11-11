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
		Body: `{"message":"Unable to update stack","details":"Stack deployment is already in progress"}`,
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
