package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/terminal"
)

func TestBootstrapOwnerCookieGetsOwnerTerminalScopeWhileAdminDoesNot(t *testing.T) {
	sut := newTestAPIServer(t)
	owner, created, err := sut.authStore.BootstrapFirstUser("bootstrap-owner", "unused-test-hash")
	if err != nil || !created {
		t.Fatalf("bootstrap owner: created=%t err=%v", created, err)
	}
	if owner.ID == "owner" {
		t.Fatal("test requires a generated owner user ID, not the legacy owner sentinel")
	}
	admin, err := sut.authStore.CreateUserWithRole("secondary-admin", "unused-test-hash", auth.RoleAdmin, "local", "")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	for _, actorID := range []string{owner.ID, admin.ID, "another-operator"} {
		if _, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
			ActorID: actorID,
			Target:  "asset-" + actorID,
			Mode:    "interactive",
		}); err != nil {
			t.Fatalf("create terminal session for %s: %v", actorID, err)
		}
	}

	ownerToken := createTestCookieSession(t, sut, owner.ID, "owner-cookie-token")
	ownerReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil)
	ownerReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: ownerToken})
	ownerRec := httptest.NewRecorder()
	sut.withAuth(sut.handleSessions)(ownerRec, ownerReq)
	ownerSessions := decodeTerminalSessions(t, ownerRec)
	if len(ownerSessions) != 3 {
		t.Fatalf("bootstrap owner saw %d sessions, want all 3", len(ownerSessions))
	}

	adminToken := createTestCookieSession(t, sut, admin.ID, "admin-cookie-token")
	adminReq := httptest.NewRequest(http.MethodGet, "/terminal/sessions", nil)
	adminReq.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminToken})
	adminRec := httptest.NewRecorder()
	sut.withAuth(sut.handleSessions)(adminRec, adminReq)
	adminSessions := decodeTerminalSessions(t, adminRec)
	if len(adminSessions) != 1 || adminSessions[0].ActorID != admin.ID {
		t.Fatalf("admin terminal scope = %+v, want only its own session", adminSessions)
	}
}

func TestStatusAggregateUsesOwnerRoleAndIsolatesAdminByActor(t *testing.T) {
	sut := newTestAPIServer(t)
	for _, actorID := range []string{"usr-generated-owner", "usr-admin", "usr-operator"} {
		if _, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
			ActorID: actorID,
			Target:  "asset-" + actorID,
			Mode:    "interactive",
		}); err != nil {
			t.Fatalf("create terminal session for %s: %v", actorID, err)
		}
	}

	ownerCtx := contextWithPrincipal(context.Background(), "usr-generated-owner", auth.RoleOwner)
	ownerResponse := sut.buildStatusAggregateResponse(ownerCtx, "")
	if len(ownerResponse.Sessions) != 3 {
		t.Fatalf("owner aggregate exposed %d sessions, want all 3", len(ownerResponse.Sessions))
	}

	adminCtx := contextWithPrincipal(context.Background(), "usr-admin", auth.RoleAdmin)
	adminResponse := sut.buildStatusAggregateResponse(adminCtx, "")
	if len(adminResponse.Sessions) != 1 || adminResponse.Sessions[0].ActorID != "usr-admin" {
		t.Fatalf("admin aggregate scope = %+v, want only usr-admin", adminResponse.Sessions)
	}
}

func TestAPIKeyOwnerRoleIsReservedAtCreateAndPatch(t *testing.T) {
	sut := newTestAPIServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/keys", bytes.NewBufferString(
		`{"name":"forbidden-owner-key","role":"owner","scopes":["assets:read"]}`,
	))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(contextWithPrincipal(createReq.Context(), "usr-owner", auth.RoleOwner))
	createRec := httptest.NewRecorder()
	sut.handleAPIKeys(createRec, createReq)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("owner API key create status = %d, want 400: %s", createRec.Code, createRec.Body.String())
	}
	keys, err := sut.apiKeyStore.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("list API keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("reserved owner key was persisted: %+v", keys)
	}

	generated, err := apikeys.GenerateKey()
	if err != nil {
		t.Fatalf("generate API key: %v", err)
	}
	key := apikeys.APIKey{
		ID:         "key-role-immutable",
		Name:       "operator key",
		Prefix:     generated.Prefix,
		SecretHash: generated.Hash,
		Role:       auth.RoleOperator,
		Scopes:     []string{"assets:read"},
		CreatedAt:  time.Now().UTC(),
	}
	if err := sut.apiKeyStore.CreateAPIKey(context.Background(), key); err != nil {
		t.Fatalf("seed API key: %v", err)
	}
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v2/keys/"+key.ID, bytes.NewBufferString(`{"role":"owner"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = patchReq.WithContext(contextWithPrincipal(patchReq.Context(), "usr-owner", auth.RoleOwner))
	patchRec := httptest.NewRecorder()
	sut.handleAPIKeyActions(patchRec, patchReq)
	if patchRec.Code != http.StatusBadRequest {
		t.Fatalf("owner API key patch status = %d, want 400: %s", patchRec.Code, patchRec.Body.String())
	}
	stored, ok, err := sut.apiKeyStore.GetAPIKey(context.Background(), key.ID)
	if err != nil || !ok {
		t.Fatalf("reload API key: ok=%t err=%v", ok, err)
	}
	if stored.Role != auth.RoleOperator {
		t.Fatalf("API key role changed to %q, want operator", stored.Role)
	}
}

func TestWithAuthRejectsPersistedOwnerRoleAPIKey(t *testing.T) {
	sut := newTestAPIServer(t)
	generated, err := apikeys.GenerateKey()
	if err != nil {
		t.Fatalf("generate API key: %v", err)
	}
	if err := sut.apiKeyStore.CreateAPIKey(context.Background(), apikeys.APIKey{
		ID:         "key-legacy-owner",
		Name:       "legacy owner key",
		Prefix:     generated.Prefix,
		SecretHash: generated.Hash,
		Role:       auth.RoleOwner,
		Scopes:     []string{"assets:read"},
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed legacy owner key: %v", err)
	}

	called := false
	handler := sut.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if apiv2.IsOwnerPrincipal(r.Context()) {
			t.Fatal("API key reached handler with owner authority")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "https://hub.local/api/v2/assets", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+generated.Raw)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy owner API key status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("legacy owner API key reached protected handler")
	}
}

func createTestCookieSession(t *testing.T, sut *apiServer, userID, rawToken string) string {
	t.Helper()
	if _, err := sut.authStore.CreateAuthSession(userID, auth.HashToken(rawToken), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create auth session for %s: %v", userID, err)
	}
	return rawToken
}

func decodeTerminalSessions(t *testing.T, rec *httptest.ResponseRecorder) []terminal.Session {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("terminal sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Sessions []terminal.Session `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode terminal sessions: %v", err)
	}
	return response.Sessions
}
