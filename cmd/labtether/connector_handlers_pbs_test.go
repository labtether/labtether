package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/credentials"
)

func TestHandlePBSConnectorTestInvalidPayloadAndValidation(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.handlePBSConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")

	req = httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"base_url":"https://pbs.local:8007"}`))
	rec = httptest.NewRecorder()
	sut.handlePBSConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing token credentials, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "base_url, token_id, and token_secret")
}

func TestHandleConnectorActionsPBSTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(pbs.New())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"base_url":"https://pbs.local:8007"}`))
		req.RemoteAddr = "203.0.113.40:4400"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"base_url":"https://pbs.local:8007"}`))
	req.RemoteAddr = "203.0.113.40:4400"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

func TestHandlePBSConnectorTestCredentialResolutionErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := &apiServer{}

		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"credential_id":"cred-1"}`))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"credential_id":"missing"}`))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})

	t.Run("credential decrypt failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		const credentialID = "cred-pbs-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               credentialID,
			Name:             "bad cipher",
			Kind:             credentials.KindPBSAPIToken,
			Username:         "root@pam!labtether",
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://pbs.local:8007"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create credential profile: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{"credential_id":"%s"}`, credentialID)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
	})
}

func TestHandlePBSConnectorTestSuccessAndFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("success with credential fallback", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/ping":
				_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.4-1","version":"3.4"}}`))
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[{"store":"backup"}]}`))
			default:
				t.Fatalf("unexpected pbs request path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		createPBSCredentialProfile(t, sut, "cred-pbs-1", "root@pam!labtether", "secret-1", server.URL)

		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{"credential_id":"cred-pbs-1","skip_verify":false}`))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), "pbs API reachable") {
			t.Fatalf("unexpected success payload: %s", rec.Body.String())
		}
	})

	t.Run("upstream health failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"errors":"permission denied"}`, http.StatusBadGateway)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"bad-secret"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("upstream health failure redacts token secret", func(t *testing.T) {
		const leakedSecret = "pbs-token-secret-should-not-leak"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, fmt.Sprintf(`{"errors":"token_secret=%s"}`, leakedSecret), http.StatusBadGateway)
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"%s"
		}`, server.URL, leakedSecret)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
		if strings.Contains(rec.Body.String(), leakedSecret) {
			t.Fatalf("expected response to redact leaked token secret, got %s", rec.Body.String())
		}
	})
}

func TestHandlePBSConnectorTestAdditionalBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("invalid ca pem", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(`{
			"base_url":"https://pbs.local:8007",
			"token_id":"root@pam!labtether",
			"token_secret":"token-secret",
			"ca_pem":"not-a-certificate"
		}`))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid PBS CA PEM")
	})

	t.Run("ping without pong", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/ping":
				_, _ = w.Write([]byte(`{"data":{"pong":false}}`))
			default:
				t.Fatalf("unexpected pbs request path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"token-secret"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("version request failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/ping":
				_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
			case "/api2/json/version":
				http.Error(w, `{"errors":"version unavailable"}`, http.StatusBadGateway)
			default:
				t.Fatalf("unexpected pbs request path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"token-secret"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("release falls back to version field", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/ping":
				_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"","version":"3.6"}}`))
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[{"store":"archive"}]}`))
			default:
				t.Fatalf("unexpected pbs request path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"token-secret"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"release":"3.6"`) {
			t.Fatalf("expected release fallback to version, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "pbs API reachable (3.6)") {
			t.Fatalf("expected message with fallback release, got %s", rec.Body.String())
		}
	})

	t.Run("zero visible datastores returns warning payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/ping":
				_, _ = w.Write([]byte(`{"data":{"pong":true}}`))
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"3.6-1","version":"3.6"}}`))
			case "/api2/json/admin/datastore":
				_, _ = w.Write([]byte(`{"data":[]}`))
			default:
				t.Fatalf("unexpected pbs request path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/pbs/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token_id":"root@pam!labtether",
			"token_secret":"token-secret"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handlePBSConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"warning":"Connected, but the token can see 0 datastores."`) {
			t.Fatalf("expected warning payload, got %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"visible_datastores":0`) {
			t.Fatalf("expected visible_datastores=0, got %s", rec.Body.String())
		}
	})
}

func createPBSCredentialProfile(t *testing.T, sut *apiServer, credentialID, username, secret, baseURL string) {
	t.Helper()
	allowInsecureTransportForConnectorTests(t)

	ciphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("encrypt pbs secret: %v", err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "pbs " + credentialID,
		Kind:             credentials.KindPBSAPIToken,
		Username:         username,
		Status:           "active",
		SecretCiphertext: ciphertext,
		Metadata: map[string]string{
			"base_url": baseURL,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create pbs credential profile: %v", err)
	}
}
