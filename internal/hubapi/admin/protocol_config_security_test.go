package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/protocols"
)

func TestValidateRDPTransportSecurityRequiresGlobalGate(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	recorder := httptest.NewRecorder()
	if validateRDPTransportSecurity(recorder, protocols.ProtocolRDP, json.RawMessage(`{"ignore_certificate":true}`)) {
		t.Fatal("expected insecure RDP config to be rejected")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("response did not explain the required global gate: %s", recorder.Body.String())
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if !validateRDPTransportSecurity(httptest.NewRecorder(), protocols.ProtocolRDP, json.RawMessage(`{"ignore_certificate":true}`)) {
		t.Fatal("expected explicit local and global opt-ins to be accepted")
	}
	if !validateRDPTransportSecurity(httptest.NewRecorder(), protocols.ProtocolRDP, json.RawMessage(`{"allow_legacy_security":true}`)) {
		t.Fatal("expected explicit local and global legacy opt-ins to be accepted")
	}
}

func TestValidateRDPTransportSecurityKeepsVerificationEnabledByDefault(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	if !validateRDPTransportSecurity(httptest.NewRecorder(), protocols.ProtocolRDP, json.RawMessage(`{}`)) {
		t.Fatal("secure RDP defaults should not require the insecure transport gate")
	}
	if !validateRDPTransportSecurity(httptest.NewRecorder(), protocols.ProtocolVNC, json.RawMessage(`{"anything":true}`)) {
		t.Fatal("non-RDP configs should not be handled by the RDP transport check")
	}
}

func TestValidateVNCTransportSecurityRequiresGlobalGateForOptIn(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	recorder := httptest.NewRecorder()
	if validateVNCTransportSecurity(recorder, protocols.ProtocolVNC, json.RawMessage(`{"allow_insecure_vnc":true}`), true) {
		t.Fatal("expected unsafe VNC config to be rejected")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("response did not explain the global VNC gate: %s", recorder.Body.String())
	}
	missingLocal := httptest.NewRecorder()
	if validateVNCTransportSecurity(missingLocal, protocols.ProtocolVNC, json.RawMessage(`{}`), true) {
		t.Fatal("enabled VNC config without a local opt-in was accepted")
	}
	if !strings.Contains(missingLocal.Body.String(), "allow_insecure_vnc") {
		t.Fatalf("response did not explain the local VNC opt-in: %s", missingLocal.Body.String())
	}
	if !validateVNCTransportSecurity(httptest.NewRecorder(), protocols.ProtocolVNC, json.RawMessage(`{}`), false) {
		t.Fatal("disabled VNC config should not require an unsafe transport opt-in")
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	if !validateVNCTransportSecurity(httptest.NewRecorder(), protocols.ProtocolARD, json.RawMessage(`{"allow_insecure_vnc":true}`), true) {
		t.Fatal("expected explicit local and global VNC opt-ins to be accepted")
	}
}
