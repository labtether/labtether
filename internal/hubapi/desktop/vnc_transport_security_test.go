package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

func TestInsecureVNCTransportRequiresBothOptIns(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	if err := validateInsecureVNCTransport(false); err == nil || !strings.Contains(err.Error(), "allow_insecure_vnc") {
		t.Fatalf("missing local opt-in error=%v", err)
	}
	if err := validateInsecureVNCTransport(true); err == nil || !strings.Contains(err.Error(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("missing global opt-in error=%v", err)
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if err := validateInsecureVNCTransport(true); err != nil {
		t.Fatalf("double opt-in rejected: %v", err)
	}
}

func TestProtocolConfigVNCTransportOptIn(t *testing.T) {
	for _, protocol := range []string{protocols.ProtocolVNC, protocols.ProtocolARD} {
		allowed, err := protocolConfigAllowsInsecureVNC(protocol, json.RawMessage(`{"allow_insecure_vnc":true}`))
		if err != nil || !allowed {
			t.Fatalf("protocol=%s allowed=%v err=%v", protocol, allowed, err)
		}
	}
	if _, err := protocolConfigAllowsInsecureVNC(protocols.ProtocolRDP, nil); err == nil {
		t.Fatal("non-VNC protocol was accepted")
	}
}

func TestDirectVNCStreamRechecksGlobalOptIn(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/desktop/sessions/test/stream", nil)
	(&Deps{}).handleDirectVNCProxyWithConfig(
		recorder,
		request,
		terminal.Session{ID: "test"},
		"192.0.2.1",
		5900,
		false,
		true,
	)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("stream gate status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
