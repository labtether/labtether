package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
)

func TestLegacyRoutesEnforceAPIKeyScopes(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct {
		name       string
		handlerKey string
		method     string
		path       string
		body       string
	}{
		{
			name:       "groups missing scope",
			handlerKey: "/groups",
			method:     http.MethodGet,
			path:       "/groups",
		},
		{
			name:       "connectors missing scope",
			handlerKey: "/connectors",
			method:     http.MethodGet,
			path:       "/connectors",
		},
		{
			name:       "legacy action execute missing scope",
			handlerKey: "/actions/execute",
			method:     http.MethodPost,
			path:       "/actions/execute",
			body:       `{"target":"srv1","command":"uptime"}`,
		},
		{
			name:       "status aggregate missing hub scope",
			handlerKey: "/status/aggregate",
			method:     http.MethodGet,
			path:       "/status/aggregate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], tc.method, tc.path, key, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLegacyAssetRoutesEnforceAllowedAssets(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKey(t, sut, []string{"assets:read"}, []string{"srv1"})
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, id := range []string{"srv1", "srv2"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: id,
			Name:    strings.ToUpper(id),
			Source:  "agent",
			Type:    "host",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed asset %s: %v", id, err)
		}
	}

	listRec := invokeLegacyRoute(t, handlers["/assets"], http.MethodGet, "/assets", key, "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing allowed assets, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp struct {
		Assets []assets.Asset `json:"assets"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode assets response: %v", err)
	}
	if len(listResp.Assets) != 1 || listResp.Assets[0].ID != "srv1" {
		t.Fatalf("expected only srv1 in legacy asset list, got %#v", listResp.Assets)
	}

	deniedRec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodGet, "/assets/srv2", key, "")
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed asset, got %d: %s", deniedRec.Code, deniedRec.Body.String())
	}

	allowedRec := invokeLegacyRoute(t, handlers["/assets/"], http.MethodGet, "/assets/srv1", key, "")
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed asset, got %d: %s", allowedRec.Code, allowedRec.Body.String())
	}
}

func TestAdminRoutesEnforceAPIKeyScopes(t *testing.T) {
	sut := newTestAPIServer(t)
	key := createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleAdmin, []string{"assets:read"}, nil)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	for _, tc := range []struct {
		name       string
		handlerKey string
		path       string
	}{
		{
			name:       "api key management",
			handlerKey: "/api/v2/keys",
			path:       "/api/v2/keys",
		},
		{
			name:       "runtime settings",
			handlerKey: "/settings/runtime",
			path:       "/settings/runtime",
		},
		{
			name:       "auth users",
			handlerKey: "/auth/users",
			path:       "/auth/users",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := invokeLegacyRoute(t, handlers[tc.handlerKey], http.MethodGet, tc.path, key, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func createLegacyRouteAPIKey(t *testing.T, sut *apiServer, scopes []string, allowedAssets []string) string {
	t.Helper()
	return createLegacyRouteAPIKeyWithRole(t, sut, auth.RoleOperator, scopes, allowedAssets)
}

func createLegacyRouteAPIKeyWithRole(t *testing.T, sut *apiServer, role string, scopes []string, allowedAssets []string) string {
	t.Helper()
	generated, err := apikeys.GenerateKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	key := apikeys.APIKey{
		ID:            "key_" + generated.Prefix,
		Name:          "legacy route test",
		Prefix:        generated.Prefix,
		SecretHash:    generated.Hash,
		Role:          role,
		Scopes:        scopes,
		AllowedAssets: allowedAssets,
		CreatedBy:     "owner",
		CreatedAt:     time.Now().UTC(),
	}
	if err := sut.apiKeyStore.CreateAPIKey(context.Background(), key); err != nil {
		t.Fatalf("store api key: %v", err)
	}
	return generated.Raw
}

func invokeLegacyRoute(
	t *testing.T,
	handler http.HandlerFunc,
	method string,
	path string,
	key string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	if handler == nil {
		t.Fatalf("missing handler for %s", path)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}
