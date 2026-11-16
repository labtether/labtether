package apiv2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/servicehttp"
)

// A device that did not answer in time reaches a v2 client as 504
// "agent_timeout" with the fixed words, while any other 5xx text stays
// hidden. The risk: a v2 client sees "An internal error occurred." for a
// device timeout, or the pass-through leaks another handler's 5xx text.
func TestWrapV1HandlerKeepsOnlyTheAgentTimeoutWords(t *testing.T) {
	tests := []struct {
		name               string
		v1                 http.HandlerFunc
		wantCode, wantText string
	}{
		{"agent timeout", func(w http.ResponseWriter, _ *http.Request) { servicehttp.WriteAgentTimeout(w) },
			"agent_timeout", servicehttp.AgentTimeoutMessage},
		{"other 504 text", func(w http.ResponseWriter, _ *http.Request) {
			servicehttp.WriteJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "dial tcp 192.0.2.10:22: i/o timeout"})
		}, "dial tcp 192.0.2.10:22: i/o timeout", "An internal error occurred."},
		{"agent words on a 500", func(w http.ResponseWriter, _ *http.Request) {
			servicehttp.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": servicehttp.AgentTimeoutMessage})
		}, servicehttp.AgentTimeoutMessage, "An internal error occurred."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WrapV1Handler(tc.v1)(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			var body struct {
				Error, Message string
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Error != tc.wantCode || body.Message != tc.wantText {
				t.Fatalf("got %d %s, want error %q message %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantText)
			}
		})
	}
}
