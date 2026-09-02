package main

import (
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

func TestSensitiveIdentityMutationsRejectAPIKeys(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"settings:read", "settings:write"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	tests := []struct {
		name       string
		handlerKey string
		method     string
		path       string
		body       string
	}{
		{name: "create API key", handlerKey: "/api/v2/keys", method: http.MethodPost, path: "/api/v2/keys", body: `{"name":"wider","role":"admin","scopes":["*"]}`},
		{name: "widen API key", handlerKey: "/api/v2/keys/", method: http.MethodPatch, path: "/api/v2/keys/key_target", body: `{"scopes":["*"],"allowed_assets":[],"expires_at":null}`},
		{name: "delete API key", handlerKey: "/api/v2/keys/", method: http.MethodDelete, path: "/api/v2/keys/key_target"},
		{name: "create local admin", handlerKey: "/auth/users", method: http.MethodPost, path: "/auth/users", body: `{"username":"wider-admin","password":"LongEnoughPassword123!","role":"admin"}`},
		{name: "reset local admin", handlerKey: "/auth/users/", method: http.MethodPatch, path: "/auth/users/usr_target", body: `{"password":"LongEnoughPassword123!"}`},
		{name: "delete local user", handlerKey: "/auth/users/", method: http.MethodDelete, path: "/auth/users/usr_target"},
		{name: "revoke local sessions", handlerKey: "/auth/users/", method: http.MethodDelete, path: "/auth/users/usr_target/sessions"},
		{name: "change OIDC settings", handlerKey: "/settings/oidc", method: http.MethodPut, path: "/settings/oidc", body: `{"enabled":false}`},
		{name: "apply OIDC settings", handlerKey: "/settings/oidc/apply", method: http.MethodPost, path: "/settings/oidc/apply"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], tc.method, tc.path, key, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSensitiveIdentityReadsRemainAvailableToAPIKeys(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"settings:read", "settings:write"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct {
		name       string
		handlerKey string
		path       string
	}{
		{name: "API keys", handlerKey: "/api/v2/keys", path: "/api/v2/keys"},
		{name: "users", handlerKey: "/auth/users", path: "/auth/users"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], http.MethodGet, tc.path, key, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
