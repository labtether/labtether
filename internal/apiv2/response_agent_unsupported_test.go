package apiv2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/servicehttp"
)

// Fault: a v2 client gets the v1 words as its error code (as "agent is not
// connected" does) instead of a stable code it can branch on.
func TestWrapV1HandlerGivesAgentUnsupportedItsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	WrapV1Handler(func(w http.ResponseWriter, _ *http.Request) { servicehttp.WriteAgentUnsupported(w) })(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var body struct{ Error, Message string }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnprocessableEntity || body.Error != AgentUnsupportedCode || body.Message != servicehttp.AgentUnsupportedMessage {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
