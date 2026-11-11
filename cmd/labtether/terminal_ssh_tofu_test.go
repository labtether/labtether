package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
)

func tofuTestKey(t *testing.T) (string, string) {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key)
}

// tofuServer is a test Hub whose SSH host key probe answers with *shown.
func tofuServer(t *testing.T) (*apiServer, *terminalpkg.Deps, *string) {
	t.Helper()
	t.Setenv(shared.EnvSSHKnownHostsPaths, "")
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	sut := newTestAPIServer(t)
	deps := sut.ensureTerminalDeps()
	shown := new(string)
	deps.ProbeSSHHostKey = func(context.Context, string, int) (string, string, error) {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(*shown))
		if err != nil {
			return "", "", err
		}
		return *shown, ssh.FingerprintSHA256(key), nil
	}
	return sut, deps, shown
}

func tofuCall(t *testing.T, handler http.HandlerFunc, path string, body map[string]any, role string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req = req.WithContext(contextWithPrincipal(req.Context(), "usr-tofu", role))
	rec := httptest.NewRecorder()
	handler(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func tofuAuditTypes(t *testing.T, sut *apiServer) map[string]map[string]any {
	t.Helper()
	events, err := sut.auditStore.List(200, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, event := range events {
		if strings.HasPrefix(event.Type, "terminal.ssh_host_key.") {
			out[event.Type] = event.Details
		}
	}
	return out
}

// Quick connect to a host the Hub has never seen: refused until the person
// trusts the key a check showed, then pinned per host and port; a changed key
// is reported, never trusted over the pin, and only replaced after removal.
func TestQuickConnectTrustsSSHHostKeyOnFirstUse(t *testing.T) {
	sut, deps, shown := tofuServer(t)
	keyA, fpA := tofuTestKey(t)
	keyB, fpB := tofuTestKey(t)
	*shown = keyA
	host := map[string]any{"host": "192.0.2.10", "port": 22}
	quick := map[string]any{"host": "192.0.2.10", "port": 22, "username": "demo", "auth_method": "password", "password": "pw-not-sent", "strict_host_key": true}

	if code, out := tofuCall(t, deps.HandleQuickSession, "/terminal/quick-session", quick, auth.RoleOperator); code != http.StatusConflict || out["requires_host_key_confirmation"] != true {
		t.Fatalf("quick connect before trust: %d %v, want 409 asking to confirm the host key", code, out)
	}
	code, out := tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/check", host, auth.RoleOperator)
	if code != http.StatusOK || out["status"] != "unknown" || out["fingerprint"] != fpA || out["host_key"] != keyA {
		t.Fatalf("first check: %d %v", code, out)
	}
	if code, out = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/trust", map[string]any{"host": "192.0.2.10", "port": 22, "host_key": keyA}, auth.RoleOperator); code != http.StatusOK {
		t.Fatalf("trust: %d %v", code, out)
	}
	if row := tofuAuditTypes(t, sut)["terminal.ssh_host_key.trusted"]; row == nil || row["fingerprint"] != fpA {
		t.Fatalf("trust audit row: %v", row)
	}
	code, out = tofuCall(t, deps.HandleQuickSession, "/terminal/quick-session", quick, auth.RoleOperator)
	if code != http.StatusCreated {
		t.Fatalf("quick connect after trust: %d %v", code, out)
	}
	sessionID, _ := out["session"].(map[string]any)["id"].(string)
	cfg, ok := deps.EphemeralSSHConfigs.Get(sessionID)
	if !ok || cfg.HostKey != keyA || !cfg.StrictHostKey {
		t.Fatalf("the session must check strictly against the trusted key: %+v", cfg)
	}
	if _, out = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/check", map[string]any{"host": "192.0.2.10", "port": 2222}, auth.RoleOperator); out["status"] != "unknown" {
		t.Fatalf("a pin for port 22 must not cover port 2222: %v", out)
	}
	*shown = keyB
	_, out = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/check", host, auth.RoleOperator)
	if out["status"] != "changed" || out["fingerprint"] != fpB || out["saved_fingerprint"] != fpA {
		t.Fatalf("changed key: %v", out)
	}
	if code, _ = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/trust", map[string]any{"host": "192.0.2.10", "port": 22, "host_key": keyB}, auth.RoleOperator); code != http.StatusConflict {
		t.Fatalf("trusting a changed key over the pin: %d, want 409", code)
	}
	if code, out = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/forget", host, auth.RoleOperator); code != http.StatusOK || out["removed"] != true {
		t.Fatalf("forget: %d %v", code, out)
	}
	if row := tofuAuditTypes(t, sut)["terminal.ssh_host_key.removed"]; row == nil || row["fingerprint"] != fpA {
		t.Fatalf("removal audit row: %v", row)
	}
	if _, out = tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/check", host, auth.RoleOperator); out["status"] != "unknown" {
		t.Fatalf("after removal the new key is asked about again: %v", out)
	}
}

// Pinning needs the rights to start a terminal session: a viewer is refused
// by the route, and a policy without interactive sessions refuses it too.
func TestSSHHostKeyTrustNeedsTerminalStartRights(t *testing.T) {
	sut, deps, shown := tofuServer(t)
	keyA, _ := tofuTestKey(t)
	*shown = keyA
	trust := map[string]any{"host": "192.0.2.10", "port": 22, "host_key": keyA}

	viewer := createRoleSessionToken(t, sut, "viewer-tofu", auth.RoleViewer)
	raw, _ := json.Marshal(trust)
	req := httptest.NewRequest(http.MethodPost, "/terminal/host-keys/trust", bytes.NewReader(raw))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: viewer})
	rec := httptest.NewRecorder()
	sut.buildHTTPHandlers(nil, nil, nil)["/terminal/host-keys/"](rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer trust: %d %s, want 403", rec.Code, rec.Body.String())
	}

	cfg := policy.DefaultEvaluatorConfig()
	cfg.InteractiveEnabled = false
	deps.PolicyState = newPolicyRuntimeState(cfg)
	if code, _ := tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/trust", trust, auth.RoleOperator); code != http.StatusForbidden {
		t.Fatalf("trust with interactive sessions off: %d, want 403", code)
	}
	if known, found, _ := sut.sshKnownHostStore.GetSSHKnownHost("192.0.2.10", 22); found {
		t.Fatalf("a refused trust stored a key: %+v", known)
	}
}

// A bookmark to a host keeps its own pin, for the host and port it pointed to
// when trusted; its saved session logs in with the bookmark's login, strictly
// against that pin.
func TestBookmarkToHostPinsSSHHostKeyOnTheBookmark(t *testing.T) {
	sut, deps, shown := tofuServer(t)
	bookmarks := newMemoryTerminalBookmarkStore()
	deps.TerminalBookmarkStore = bookmarks
	keyA, _ := tofuTestKey(t)
	*shown = keyA
	profile := seedFileConnectionCredential(t, sut, "demo", credentials.KindSSHPassword, "pw-bookmark", "")
	port := 22
	bookmark, _ := bookmarks.CreateBookmark(terminal.CreateBookmarkRequest{ActorID: "usr-tofu", Title: "nas01", Host: "192.0.2.10", Port: &port, Username: "demo", CredentialProfileID: profile.ID})
	connect := "/terminal/bookmarks/" + bookmark.ID + "/connect"

	if code, out := tofuCall(t, deps.HandleBookmarkActions, connect, map[string]any{}, auth.RoleOperator); code != http.StatusConflict || out["requires_host_key_confirmation"] != true {
		t.Fatalf("bookmark connect before trust: %d %v, want 409", code, out)
	}
	if code, out := tofuCall(t, deps.HandleSSHHostKeys, "/terminal/host-keys/trust", map[string]any{"bookmark_id": bookmark.ID, "host_key": keyA}, auth.RoleOperator); code != http.StatusOK {
		t.Fatalf("trust for the bookmark: %d %v", code, out)
	}
	if known, found, _ := sut.sshKnownHostStore.GetSSHKnownHost("192.0.2.10", 22); found {
		t.Fatalf("a bookmark's pin went to the Quick connect hosts: %+v", known)
	}
	code, out := tofuCall(t, deps.HandleBookmarkActions, connect, map[string]any{}, auth.RoleOperator)
	if code != http.StatusCreated {
		t.Fatalf("bookmark connect after trust: %d %v", code, out)
	}
	session, ok, _ := sut.terminalStore.GetSession(out["session_id"].(string))
	if !ok {
		t.Fatal("bookmark connect made no session")
	}
	resolved, err := deps.ResolveSessionSSHConfig(session)
	if err != nil || resolved == nil || resolved.HostKey != keyA || !resolved.StrictHostKey || resolved.Password != "pw-bookmark" || resolved.User != "demo" {
		t.Fatalf("bookmark session must log in with its saved login against the pin: %+v err=%v", resolved, err)
	}
	// The saved login is used at stream time, so a key without credentials:use is refused there too.
	ticket := func(scopes []string) int {
		req := httptest.NewRequest(http.MethodPost, "/terminal/sessions/"+session.ID+"/stream-ticket", bytes.NewBufferString(`{}`))
		ctx := contextWithScopes(contextWithPrincipal(req.Context(), "usr-tofu", auth.RoleOperator), scopes)
		rec := httptest.NewRecorder()
		deps.HandleSessionActions(rec, req.WithContext(ctx))
		return rec.Code
	}
	if code := ticket([]string{"terminal:write"}); code != http.StatusForbidden {
		t.Fatalf("stream ticket without credentials:use: %d, want 403", code)
	}
	if code := ticket([]string{"terminal:write", "credentials:use"}); code == http.StatusForbidden {
		t.Fatalf("stream ticket with credentials:use: %d", code)
	}
	other := 2222
	stored, _, _ := bookmarks.GetBookmark(bookmark.ID)
	stored.Port = &other
	if stored.PinnedHostKey() != "" {
		t.Fatal("a pin must not follow the bookmark to another port")
	}
}
