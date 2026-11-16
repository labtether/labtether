package desktop_test

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

func TestDirectSPICESecurityDefaultsToTLSAndRejectsUnsafeOptions(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	sut := hubdeps.New(t)

	create := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(body))
		req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-spice-security", "admin"))
		rec := httptest.NewRecorder()
		sut.DesktopDeps().HandleDesktopSessions(rec, req)
		return rec
	}

	secure := create(`{"protocol":"spice","direct_target":{"host":"192.0.2.77","port":5930}}`)
	if secure.Code != http.StatusCreated {
		t.Fatalf("default secure SPICE status=%d body=%s", secure.Code, secure.Body.String())
	}
	var session terminal.Session
	if err := json.Unmarshal(secure.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode secure SPICE session: %v", err)
	}
	if got := sut.DesktopDeps().GetDesktopSessionOptions(session.ID).SPICESecurityMode; got != "tls" {
		t.Fatalf("default SPICE security mode=%q, want tls", got)
	}

	for _, body := range []string{
		`{"protocol":"spice","direct_target":{"host":"192.0.2.78","port":5930,"spice_security_mode":"cleartext"}}`,
		`{"protocol":"spice","direct_target":{"host":"192.0.2.78","port":5930,"spice_ca_pem":"not a certificate"}}`,
		`{"protocol":"rdp","direct_target":{"host":"192.0.2.78","port":3389,"spice_security_mode":"tls"}}`,
	} {
		rejected := create(body)
		if rejected.Code != http.StatusBadRequest {
			t.Fatalf("unsafe SPICE options status=%d body=%s", rejected.Code, rejected.Body.String())
		}
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	cleartext := create(`{"protocol":"spice","direct_target":{"host":"192.0.2.79","port":5930,"spice_security_mode":"cleartext"}}`)
	if cleartext.Code != http.StatusCreated {
		t.Fatalf("double-opt-in cleartext SPICE status=%d body=%s", cleartext.Code, cleartext.Body.String())
	}
	if err := json.Unmarshal(cleartext.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode cleartext SPICE session: %v", err)
	}
	if got := sut.DesktopDeps().GetDesktopSessionOptions(session.ID).SPICESecurityMode; got != "cleartext" {
		t.Fatalf("explicit SPICE security mode=%q, want cleartext", got)
	}
}
