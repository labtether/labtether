package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// "Allow http on my network" may open plain http only to a local-network
// address; a public address stays refused even with the switch on, and
// without the switch plain http stays https-only.
func TestHomeAssistantLocalHTTPSwitchStaysOnTheLocalNetwork(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	sut := newTestAPIServer(t)
	run := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(body)))
		return rec
	}

	rec := run(`{"base_url":"http://8.8.8.8:8123","token":"ha-token","allow_local_http":true}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("public http with the switch: %d %s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "only for an address on your own network")

	rec = run(`{"base_url":"http://192.168.1.20:8123","token":"ha-token"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("local http without the switch: %d %s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "https only")
}
