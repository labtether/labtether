package collectors_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePortainerConnectorTestInvalidPayload(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")
}

func TestHandlePortainerConnectorTestValidationErrors(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)

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
			sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), tt.wantMessage)
		})
	}
}

func TestHandlePortainerConnectorTestCredentialResolutionErrors(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := hubdeps.NewEmpty(t)

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{
			"auth_method":"api_key",
			"credential_id":"cred-1"
		}`))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := hubdeps.New(t)

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{
			"auth_method":"api_key",
			"credential_id":"does-not-exist"
		}`))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})
}

func TestHandlePortainerConnectorTestSuccessWithCredentialFallback(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
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

	sut := hubdeps.New(t)
	credentialID := seedPortainerCredentialProfile(t, sut, "svc@local!automation", "ptr-secret-value", mock.URL)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"credential_id":"%s"
	}`, credentialID)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

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
	testutil.AllowInsecureTransportForConnectorTests(t)
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

	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret",
		"skip_verify":false
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawAPIKey != "ptr-secret" {
		t.Fatalf("expected API key from inline secret, got %q", sawAPIKey)
	}
}

func TestHandlePortainerConnectorTestAPIKeyDoesNotRequireTokenID(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
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

	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_secret":"ptr-secret-no-token-id"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without token_id, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sawAPIKey != "ptr-secret-no-token-id" {
		t.Fatalf("expected API key from inline secret, got %q", sawAPIKey)
	}
}

func TestHandlePortainerConnectorTestFailsWhenNoEndpointsVisible(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
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

	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_secret":"ptr-secret-no-endpoints"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when no endpoints are visible, got %d body=%s", rec.Code, rec.Body.String())
	}
	// A plain reason that names the product, never the generic 5xx text.
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "Portainer")
}

func TestHandlePortainerConnectorTestDecryptFailure(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)

	const credentialID = "cred-portainer-bad-cipher"
	_, err := sut.CredentialStore.CreateCredentialProfile(credentials.Profile{
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
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
}

func TestHandlePortainerConnectorTestSuccessWithPasswordAuth(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
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

	sut := hubdeps.New(t)
	credentialID := seedPortainerCredentialProfile(t, sut, "admin", "password-secret", mock.URL)

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"password",
		"credential_id":"%s"
	}`, credentialID)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

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
	testutil.AllowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"downstream unavailable"}`))
	}))
	defer mock.Close()

	sut := hubdeps.New(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret"
	}`, mock.URL)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	// A plain reason that names the product, never the generic 5xx text.
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "Portainer")
}

func TestHandlePortainerConnectorTestUpstreamFailureRedactsSecret(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	const leakedSecret = "ptr-secret-should-not-leak"

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"token_secret=%s"}`, leakedSecret)))
	}))
	defer mock.Close()

	sut := hubdeps.New(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"%s"
	}`, mock.URL, leakedSecret)))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandlePortainerConnectorTest(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	// A plain reason that names the product, never the generic 5xx text.
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "Portainer")
	if strings.Contains(rec.Body.String(), leakedSecret) {
		t.Fatalf("expected response to redact leaked secret, got %s", rec.Body.String())
	}
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

func seedPortainerCredentialProfile(t *testing.T, sut *hubdeps.Hub, username, secret, baseURL string) string {
	t.Helper()

	const credentialID = "cred-portainer-test"
	secretCiphertext, err := sut.SecretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	_, err = sut.CredentialStore.CreateCredentialProfile(credentials.Profile{
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
