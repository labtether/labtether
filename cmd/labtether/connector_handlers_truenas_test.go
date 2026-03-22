package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/credentials"
)

func TestHandleTrueNASConnectorTestInvalidPayloadAndValidation(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.handleTrueNASConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")

	req = httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
	rec = httptest.NewRecorder()
	sut.handleTrueNASConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing api key, got %d", rec.Code)
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "base_url and api_key")
}

func TestHandleTrueNASConnectorTestRejectsDisallowedOutboundURL(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{
		"base_url":"https://blocked.example.net",
		"api_key":"test-key"
	}`))
	rec := httptest.NewRecorder()
	sut.handleTrueNASConnectorTest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disallowed outbound URL, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "not allowlisted")
}

func TestHandleConnectorActionsTrueNASTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(truenas.New())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
		req.RemoteAddr = "203.0.113.42:4402"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
	req.RemoteAddr = "203.0.113.42:4402"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

func TestHandleTrueNASConnectorTestCredentialResolutionErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := &apiServer{}

		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"cred-1"}`))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential store unavailable")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"missing"}`))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})

	t.Run("credential decrypt failure", func(t *testing.T) {
		sut := newTestAPIServer(t)
		const credentialID = "cred-truenas-bad-cipher"
		_, err := sut.credentialStore.CreateCredentialProfile(credentials.Profile{
			ID:               credentialID,
			Name:             "bad cipher",
			Kind:             credentials.KindTrueNASAPIKey,
			Status:           "active",
			SecretCiphertext: "not-valid-ciphertext",
			Metadata:         map[string]string{"base_url": "https://truenas.local"},
			CreatedAt:        time.Now().UTC(),
			UpdatedAt:        time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("create credential profile: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(fmt.Sprintf(`{"credential_id":"%s"}`, credentialID)))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
	})
}

func TestHandleTrueNASConnectorTestSuccessAndFailure(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("success with credential fallback", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			if method == "system.info" {
				return map[string]any{"hostname": "OmegaNAS", "version": "25.04.0"}, nil
			}
			return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", server.URL)

		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"cred-truenas-1","skip_verify":false}`))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), "connected to OmegaNAS") {
			t.Fatalf("unexpected success payload: %s", rec.Body.String())
		}
	})

	t.Run("upstream health failure", func(t *testing.T) {
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"api_key":"bad-key"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "permission denied")
	})

	t.Run("upstream health failure redacts api key", func(t *testing.T) {
		const leakedSecret = "truenas-api-key-should-not-leak"
		server := newTrueNASRPCServer(t, func(method string, params []any) (any, *trueNASRPCError) {
			return nil, &trueNASRPCError{Code: -32000, Message: "api_key=" + leakedSecret}
		})
		defer server.Close()

		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"api_key":"%s"
		}`, server.URL, leakedSecret)))
		rec := httptest.NewRecorder()
		sut.handleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "truenas rpc error")
		if strings.Contains(rec.Body.String(), leakedSecret) {
			t.Fatalf("expected response to redact leaked api key, got %s", rec.Body.String())
		}
	})
}
