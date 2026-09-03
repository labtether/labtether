package terminal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

func TestInsecureTelnetTransportRequiresBothOptIns(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	if err := validateInsecureTelnetTransport(false); err == nil || !strings.Contains(err.Error(), "allow_insecure_telnet") {
		t.Fatalf("missing local opt-in error=%v", err)
	}
	if err := validateInsecureTelnetTransport(true); err == nil || !strings.Contains(err.Error(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("missing global opt-in error=%v", err)
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if err := validateInsecureTelnetTransport(true); err != nil {
		t.Fatalf("double opt-in rejected: %v", err)
	}
}

func TestTelnetStreamRechecksGlobalOptInBeforeDial(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/terminal/sessions/test/stream", nil)
	(&Deps{}).HandleTelnetStream(
		recorder,
		request,
		terminalmodel.Session{ID: "test"},
		"192.0.2.1",
		23,
		true,
	)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("stream gate status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
