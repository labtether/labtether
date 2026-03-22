package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/credentials"
)

func TestHandleHomeAssistantConnectorTestInvalidPayloadAndValidation(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.handleHomeAssistantConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")

	req = httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{"base_url":"https://ha.local"}`))
	rec = httptest.NewRecorder()
	sut.handleHomeAssistantConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing token, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "base_url and token")
}

func TestHandleHomeAssistantConnectorTestRejectsDisallowedOutboundURL(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{
		"base_url":"https://blocked.example.net",
		"token":"ha-token"
	}`))
	rec := httptest.NewRecorder()
	sut.handleHomeAssistantConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disallowed outbound URL, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "not allowlisted")
}

func TestHandleConnectorActionsHomeAssistantTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(homeassistant.New())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/homeassistant/test", strings.NewReader(`{"base_url":"https://ha.local"}`))
		req.RemoteAddr = "203.0.113.42:4403"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/homeassistant/test", strings.NewReader(`{"base_url":"https://ha.local"}`))
	req.RemoteAddr = "203.0.113.42:4403"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

func TestHandleConnectorActionsHomeAssistantTestRateLimitSharesAliasBuckets(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(homeassistant.New())

	for i := 0; i < 12; i++ {
		path := "/connectors/homeassistant/test"
		if i%2 == 1 {
			path = "/connectors/home-assistant/test"
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"base_url":"https://ha.local"}`))
		req.RemoteAddr = "203.0.113.43:4403"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d (%s): expected 400 before rate limit, got %d", i+1, path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{"base_url":"https://ha.local"}`))
	req.RemoteAddr = "203.0.113.43:4403"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after mixed-alias connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

func TestHandleHomeAssistantConnectorTestCredentialResolutionErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := &apiServer{}

		req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{"credential_id":"cred-1"}`))
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential store unavailable")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{"credential_id":"missing"}`))
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})

	t.Run("credential decrypt failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		const credentialID = "cred-ha-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               credentialID,
			Name:             "bad cipher",
			Kind:             credentials.KindHomeAssistantToken,
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://ha.local"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create credential profile: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(fmt.Sprintf(`{"credential_id":"%s"}`, credentialID)))
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
	})
}

func TestHandleHomeAssistantConnectorTestSuccessAndFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("success with credential fallback", func(t *testing.T) {
		var sawAuthHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			sawAuthHeader = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"message":"ok"}`))
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		createHomeAssistantCredentialProfile(t, sut, "cred-ha-1", "ha-token-1", server.URL)

		req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(`{"credential_id":"cred-ha-1","skip_verify":false}`))
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if sawAuthHeader != "Bearer ha-token-1" {
			t.Fatalf("expected bearer token from credential secret, got %q", sawAuthHeader)
		}
		if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), "home assistant API reachable") {
			t.Fatalf("unexpected success payload: %s", rec.Body.String())
		}
	})

	t.Run("upstream health failure redacts token", func(t *testing.T) {
		const leakedSecret = "ha-token-should-not-leak"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"token=` + leakedSecret + `"}`))
		}))
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/home-assistant/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"token":"%s"
		}`, server.URL, leakedSecret)))
		rec := httptest.NewRecorder()
		sut.handleHomeAssistantConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "home assistant api returned 502")
		if strings.Contains(rec.Body.String(), leakedSecret) {
			t.Fatalf("expected response to redact leaked token, got %s", rec.Body.String())
		}
	})
}

func createHomeAssistantCredentialProfile(t *testing.T, sut *apiServer, credentialID, secret, baseURL string) {
	t.Helper()

	secretCiphertext, err := sut.secretsManager.EncryptString(secret, credentialID)
	if err != nil {
		t.Fatalf("failed to encrypt home assistant credential: %v", err)
	}
	_, err = sut.credentialStore.CreateCredentialProfile(credentials.Profile{
		ID:               credentialID,
		Name:             "homeassistant " + credentialID,
		Kind:             credentials.KindHomeAssistantToken,
		Status:           "active",
		SecretCiphertext: secretCiphertext,
		Metadata:         map[string]string{"base_url": baseURL},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to store home assistant credential profile: %v", err)
	}
}
