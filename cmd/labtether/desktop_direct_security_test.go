package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/terminal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectDesktopSessionOptionsAreIsolatedAndProtocolCannotBeOverridden(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	sut := newTestAPIServer(t)
	create := func(actor, host, secret, protocol string) terminal.Session {
		t.Helper()
		vncOptIn := ""
		if protocol == "vnc" {
			vncOptIn = `,"allow_insecure_vnc":true`
		}
		req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(
			`{"protocol":"`+protocol+`","direct_target":{"host":"`+host+`","port":3389,"password":"`+secret+`"`+vncOptIn+`}}`,
		))
		req = req.WithContext(contextWithPrincipal(req.Context(), actor, "admin"))
		rec := httptest.NewRecorder()
		sut.handleDesktopSessions(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s status=%d body=%s", actor, rec.Code, rec.Body.String())
		}
		var session terminal.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
			t.Fatalf("decode %s session: %v", actor, err)
		}
		return session
	}
	first := create("actor-a", "192.0.2.60", "secret-a", "vnc")
	second := create("actor-b", "192.0.2.61", "secret-b", "rdp")
	firstOpts, secondOpts := sut.getDesktopSessionOptions(first.ID), sut.getDesktopSessionOptions(second.ID)
	if firstOpts.DirectPassword != "secret-a" || secondOpts.DirectPassword != "secret-b" || firstOpts.DirectHost == secondOpts.DirectHost {
		t.Fatalf("direct session options crossed boundaries: first=%+v second=%+v", firstOpts, secondOpts)
	}

	overrideReq := httptest.NewRequest(http.MethodGet, "/desktop/sessions/"+first.ID+"/stream?protocol=rdp", nil)
	if got := sut.ensureDesktopDeps().ResolveDesktopProtocol(first, overrideReq); got != "vnc" {
		t.Fatalf("creation-time protocol was overridden by stream query: got %q", got)
	}
	crossActorReq := httptest.NewRequest(http.MethodGet, "/desktop/sessions/"+first.ID, nil)
	crossActorReq = crossActorReq.WithContext(contextWithUserID(crossActorReq.Context(), "actor-b"))
	crossActorRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(crossActorRec, crossActorReq)
	if crossActorRec.Code != http.StatusForbidden {
		t.Fatalf("cross-actor direct session access status=%d body=%s", crossActorRec.Code, crossActorRec.Body.String())
	}
}

func TestDirectVNCRequiresSessionAndGlobalOptIn(t *testing.T) {
	sut := newTestAPIServer(t)
	create := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(body))
		req = req.WithContext(contextWithPrincipal(req.Context(), "actor-vnc-security", "admin"))
		rec := httptest.NewRecorder()
		sut.handleDesktopSessions(rec, req)
		return rec
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	missingLocal := create(`{"protocol":"vnc","direct_target":{"host":"192.0.2.62","port":5900}}`)
	if missingLocal.Code != http.StatusBadRequest || !strings.Contains(missingLocal.Body.String(), "allow_insecure_vnc") {
		t.Fatalf("missing local VNC opt-in status=%d body=%s", missingLocal.Code, missingLocal.Body.String())
	}
	missingGlobal := create(`{"protocol":"vnc","direct_target":{"host":"192.0.2.62","port":5900,"allow_insecure_vnc":true}}`)
	if missingGlobal.Code != http.StatusBadRequest || !strings.Contains(missingGlobal.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("missing global VNC opt-in status=%d body=%s", missingGlobal.Code, missingGlobal.Body.String())
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	allowed := create(`{"protocol":"vnc","direct_target":{"host":"192.0.2.62","port":5900,"allow_insecure_vnc":true}}`)
	if allowed.Code != http.StatusCreated {
		t.Fatalf("double-opt-in VNC status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	var session terminal.Session
	if err := json.Unmarshal(allowed.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode VNC session: %v", err)
	}
	if !sut.getDesktopSessionOptions(session.ID).VNCAllowInsecureTransport {
		t.Fatal("VNC session did not retain its explicit transport opt-in")
	}

	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	ticketReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+session.ID+"/stream-ticket", nil)
	ticketReq = ticketReq.WithContext(contextWithPrincipal(ticketReq.Context(), "actor-vnc-security", "admin"))
	ticketRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(ticketRec, ticketReq)
	if ticketRec.Code != http.StatusBadRequest || !strings.Contains(ticketRec.Body.String(), "LABTETHER_ALLOW_INSECURE_TRANSPORT") {
		t.Fatalf("VNC ticket did not recheck global opt-in: status=%d body=%s", ticketRec.Code, ticketRec.Body.String())
	}
}

func TestDirectSPICETicketUsesEphemeralPasswordWithoutURLLeak(t *testing.T) {
	sut := newTestAPIServer(t)
	secret := "synthetic-direct-spice-secret"
	createReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(
		`{"protocol":"spice","direct_target":{"host":"192.0.2.75","port":5930,"password":"`+secret+`"}}`,
	))
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "actor-spice", "admin"))
	createRec := httptest.NewRecorder()
	sut.handleDesktopSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("direct SPICE create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created terminal.Session
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode direct SPICE session: %v", err)
	}

	ticketReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+created.ID+"/spice-ticket", nil)
	ticketReq = ticketReq.WithContext(contextWithPrincipal(ticketReq.Context(), "actor-spice", "admin"))
	ticketRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(ticketRec, ticketReq)
	if ticketRec.Code != http.StatusCreated {
		t.Fatalf("direct SPICE ticket status=%d body=%s", ticketRec.Code, ticketRec.Body.String())
	}
	var payload struct {
		StreamPath string `json:"stream_path"`
		Password   string `json:"password"`
		Type       string `json:"type"`
	}
	if err := json.Unmarshal(ticketRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode direct SPICE ticket: %v", err)
	}
	if payload.Password != secret || payload.Type != "spice" {
		t.Fatalf("unexpected direct SPICE ticket payload: %+v", payload)
	}
	if !strings.Contains(payload.StreamPath, "/desktop/sessions/"+created.ID+"/stream?") {
		t.Fatalf("unexpected direct SPICE stream path: %q", payload.StreamPath)
	}
	if strings.Contains(payload.StreamPath, secret) {
		t.Fatal("direct SPICE password leaked into stream URL")
	}
}

func TestDirectSPICETicketAllowsPasswordlessEndpoint(t *testing.T) {
	sut := newTestAPIServer(t)
	createReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(
		`{"protocol":"spice","direct_target":{"host":"192.0.2.76","port":5930}}`,
	))
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "actor-spice-empty", "admin"))
	createRec := httptest.NewRecorder()
	sut.handleDesktopSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("passwordless direct SPICE create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created terminal.Session
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode passwordless direct SPICE session: %v", err)
	}

	ticketReq := httptest.NewRequest(http.MethodPost, "/desktop/sessions/"+created.ID+"/spice-ticket", nil)
	ticketReq = ticketReq.WithContext(contextWithPrincipal(ticketReq.Context(), "actor-spice-empty", "admin"))
	ticketRec := httptest.NewRecorder()
	sut.handleDesktopSessionActions(ticketRec, ticketReq)
	if ticketRec.Code != http.StatusCreated {
		t.Fatalf("passwordless direct SPICE ticket status=%d body=%s", ticketRec.Code, ticketRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(ticketRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode passwordless direct SPICE ticket: %v", err)
	}
	password, exists := payload["password"]
	if !exists || password != "" {
		t.Fatalf("passwordless direct SPICE response must carry an explicit empty password, got %#v", payload)
	}
}

func TestDirectSPICESecurityDefaultsToTLSAndRejectsUnsafeOptions(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "")
	sut := newTestAPIServer(t)

	create := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/desktop/sessions", bytes.NewBufferString(body))
		req = req.WithContext(contextWithPrincipal(req.Context(), "actor-spice-security", "admin"))
		rec := httptest.NewRecorder()
		sut.handleDesktopSessions(rec, req)
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
	if got := sut.getDesktopSessionOptions(session.ID).SPICESecurityMode; got != "tls" {
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
	if got := sut.getDesktopSessionOptions(session.ID).SPICESecurityMode; got != "cleartext" {
		t.Fatalf("explicit SPICE security mode=%q, want cleartext", got)
	}
}
