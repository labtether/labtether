package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
)

func allowInsecureTransportForConnectorTests(t *testing.T) {
	t.Helper()
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
}

func TestHandlePortainerConnectorTestInvalidPayload(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")
}

func TestHandlePortainerConnectorTestValidationErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	tests := []struct {
		name        string
		payload     string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "unsupported auth method",
			payload:     `{"auth_method":"sso","base_url":"https://portainer.local","token_id":"u@r!id","token_secret":"secret"}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "unsupported auth_method",
		},
		{
			name:        "missing required api key fields",
			payload:     `{"auth_method":"api_key","base_url":"https://portainer.local"}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "base_url and token_secret",
		},
		{
			name:        "missing required password fields",
			payload:     `{"auth_method":"password","base_url":"https://portainer.local","username":"admin"}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "base_url, username, and password are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(tt.payload))
			rec := httptest.NewRecorder()
			sut.handlePortainerConnectorTest(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			assertErrorBodyContains(t, rec.Body.Bytes(), tt.wantMessage)
		})
	}
}

func TestHandlePortainerConnectorTestCredentialResolutionErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := &apiServer{}

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{
			"auth_method":"api_key",
			"credential_id":"cred-1"
		}`))
		rec := httptest.NewRecorder()
		sut.handlePortainerConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := newTestAPIServer(t)

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{
			"auth_method":"api_key",
			"credential_id":"does-not-exist"
		}`))
		rec := httptest.NewRecorder()
		sut.handlePortainerConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})
}

func TestHandlePortainerConnectorTestSuccessWithCredentialFallback(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	var sawAPIKey string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawAPIKey = r.Header.Get("X-API-Key")
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	credentialID := seedPortainerCredentialProfile(t, sut, "svc@local!automation", "ptr-secret-value", mock.URL)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"credential_id":"%s"
	}`, credentialID)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawAPIKey != "ptr-secret-value" {
		t.Fatalf("expected API key from credential secret, got %q", sawAPIKey)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", payload["status"])
	}
}

func TestHandlePortainerConnectorTestDefaultsAuthMethodAndHonorsSkipVerify(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	var sawAPIKey string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.23.0"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawAPIKey = r.Header.Get("X-API-Key")
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret",
		"skip_verify":false
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawAPIKey != "ptr-secret" {
		t.Fatalf("expected API key from inline secret, got %q", sawAPIKey)
	}
}

func TestHandlePortainerConnectorTestAPIKeyDoesNotRequireTokenID(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	var sawAPIKey string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.23.1"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawAPIKey = r.Header.Get("X-API-Key")
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_secret":"ptr-secret-no-token-id"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without token_id, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawAPIKey != "ptr-secret-no-token-id" {
		t.Fatalf("expected API key from inline secret, got %q", sawAPIKey)
	}
}

func TestHandlePortainerConnectorTestFailsWhenNoEndpointsVisible(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.23.1"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_secret":"ptr-secret-no-endpoints"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when no endpoints are visible, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestHandlePortainerConnectorTestDecryptFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	const credentialID = "cred-portainer-bad-cipher"
	_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "Portainer Bad Cipher",
		Kind:             credentials.KindPortainerAPIKey,
		Username:         "svc@local!automation",
		SecretCiphertext: "not-valid-ciphertext",
		Metadata: map[string]string{
			"base_url": "https://portainer.local:9443",
		},
	})
	if err != nil {
		t.Fatalf("create credential profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"credential_id":"%s"
	}`, credentialID)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
}

func TestHandlePortainerConnectorTestSuccessWithPasswordAuth(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	var authBody struct {
		Username string `json:"Username"`
		Password string `json:"Password"`
	}
	var authCalls int

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authCalls++
			if err := json.NewDecoder(r.Body).Decode(&authBody); err != nil {
				t.Fatalf("decode auth body: %v", err)
			}
			_, _ = w.Write([]byte(`{"jwt":"header.payload.sig"}`))
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.22.1"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	credentialID := seedPortainerCredentialProfile(t, sut, "admin", "password-secret", mock.URL)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"password",
		"credential_id":"%s"
	}`, credentialID)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if authCalls != 1 {
		t.Fatalf("expected one auth call, got %d", authCalls)
	}
	if authBody.Username != "admin" || authBody.Password != "password-secret" {
		t.Fatalf("unexpected auth body: %+v", authBody)
	}
}

func TestHandlePortainerConnectorTestUpstreamFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"downstream unavailable"}`))
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
}

func TestHandlePortainerConnectorTestUpstreamFailureRedactsSecret(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	const leakedSecret = "ptr-secret-should-not-leak"

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"token_secret=%s"}`, leakedSecret)))
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"%s"
	}`, mock.URL, leakedSecret)))
	rec := httptest.NewRecorder()
	sut.handlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	if strings.Contains(rec.Body.String(), leakedSecret) {
		t.Fatalf("expected response to redact leaked secret, got %s", rec.Body.String())
	}
}

func TestHandleConnectorActionsPortainerRouteErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("portainer test method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/test", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("connector path not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "connector path not found")
	})

	t.Run("invalid connector path", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid connector path")
	})

	t.Run("connector not registered", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "connector not registered")
	})

	t.Run("unknown connector action", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/not-real", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "unknown connector action")
	})
}

func TestHandleConnectorActionsPortainerDiscover(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("discover failed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			discoverFn: func(context.Context) ([]connectorsdk.Asset, error) {
				return nil, errors.New("discover boom")
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("discover success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			discoverFn: func(context.Context) ([]connectorsdk.Asset, error) {
				return []connectorsdk.Asset{
					{ID: "portainer-endpoint-1", Type: "container-host", Name: "endpoint-1", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1"}},
					{ID: "portainer-container-1-abcd", Type: "container", Name: "nginx", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "container_id": "abcd"}},
				}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Assets         []connectorsdk.Asset `json:"assets"`
			Relationships  []map[string]any     `json:"relationships"`
			CapabilitySets []map[string]any     `json:"capability_sets"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.Assets) != 2 || payload.Assets[0].ID != "portainer-endpoint-1" {
			t.Fatalf("unexpected discover payload: %#v", payload.Assets)
		}
		if len(payload.Relationships) == 0 {
			t.Fatalf("expected synthesized relationships in discover payload")
		}
		if len(payload.CapabilitySets) == 0 {
			t.Fatalf("expected synthesized capability_sets in discover payload")
		}
	})
}

func TestHandleConnectorActionsPortainerHealth(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("health failed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			testFn: func(context.Context) (connectorsdk.Health, error) {
				return connectorsdk.Health{}, errors.New("health boom")
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("health success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			testFn: func(context.Context) (connectorsdk.Health, error) {
				return connectorsdk.Health{Status: "ok", Message: "healthy"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload connectorsdk.Health
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Status != "ok" || payload.Message != "healthy" {
			t.Fatalf("unexpected health payload: %#v", payload)
		}
	})
}

func TestHandleConnectorActionsPortainerActionsAndExecute(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("actions list method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("actions list success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			actionsFn: func() []connectorsdk.ActionDescriptor {
				return []connectorsdk.ActionDescriptor{
					{ID: "container.restart", Name: "Restart Container", RequiresTarget: true},
				}
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Actions []connectorsdk.ActionDescriptor `json:"actions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.Actions) != 1 || payload.Actions[0].ID != "container.restart" {
			t.Fatalf("unexpected actions payload: %#v", payload.Actions)
		}
	})

	t.Run("execute method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions/container.restart/execute", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("execute invalid payload", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader("{"))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid action payload")
	})

	t.Run("execute error", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return connectorsdk.ActionResult{}, errors.New("execute boom")
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("execute success and eof decode path", func(t *testing.T) {
		var gotActionID string
		var gotReq connectorsdk.ActionRequest

		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(_ context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				gotActionID = actionID
				gotReq = req
				return connectorsdk.ActionResult{Status: "ok", Message: "done"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if gotActionID != "container.restart" {
			t.Fatalf("expected actionID container.restart, got %q", gotActionID)
		}
		if gotReq.TargetID != "" || gotReq.Params != nil || gotReq.DryRun {
			t.Fatalf("expected zero action request from EOF body, got %#v", gotReq)
		}
	})

	t.Run("execute failed status maps to bad request", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return connectorsdk.ActionResult{Status: "failed", Message: "bad target"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{"target_id":"container-1"}`))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleConnectorActionsDispatchesPortainerTest(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "ptr-secret" {
			t.Fatalf("expected API key header")
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(portainer.New())

	payload := []byte(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret"
	}`, mock.URL))
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectorActionsPortainerTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(&mockPortainerConnector{})

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
		req.RemoteAddr = "203.0.113.41:4401"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
	req.RemoteAddr = "203.0.113.41:4401"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

type mockPortainerConnector struct {
	discoverFn func(ctx context.Context) ([]connectorsdk.Asset, error)
	testFn     func(ctx context.Context) (connectorsdk.Health, error)
	actionsFn  func() []connectorsdk.ActionDescriptor
	executeFn  func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)
}

func (m *mockPortainerConnector) ID() string { return "portainer" }

func (m *mockPortainerConnector) DisplayName() string { return "Portainer (mock)" }

func (m *mockPortainerConnector) Capabilities() connectorsdk.Capabilities {
	return connectorsdk.Capabilities{
		DiscoverAssets: true,
		CollectMetrics: true,
		CollectEvents:  true,
		ExecuteActions: true,
	}
}

func (m *mockPortainerConnector) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx)
	}
	return nil, nil
}

func (m *mockPortainerConnector) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	if m.testFn != nil {
		return m.testFn(ctx)
	}
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}

func (m *mockPortainerConnector) Actions() []connectorsdk.ActionDescriptor {
	if m.actionsFn != nil {
		return m.actionsFn()
	}
	return nil
}

func (m *mockPortainerConnector) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, actionID, req)
	}
	return connectorsdk.ActionResult{Status: "ok", Message: "ok"}, nil
}

func seedPortainerCredentialProfile(t *testing.T, sut *apiServer, username, secret, baseURL string) string {
	t.Helper()

	const credentialID = "cred-portainer-test"
	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "Portainer Test Credential",
		Kind:             credentials.KindPortainerAPIKey,
		Username:         username,
		SecretCiphertext: secretCiphertext,
		Metadata: map[string]string{
			"base_url": baseURL,
		},
	})
	if err != nil {
		t.Fatalf("create credential profile: %v", err)
	}

	return credentialID
}

func assertErrorBodyContains(t *testing.T, body []byte, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	raw, _ := payload["error"].(string)
	if !strings.Contains(raw, want) {
		t.Fatalf("expected error containing %q, got %q", want, raw)
	}
}
