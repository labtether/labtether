package collectors_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

func TestHandleTrueNASConnectorTestInvalidPayloadAndValidation(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "invalid test payload")

	req = httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
	rec = httptest.NewRecorder()
	sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing api key, got %d", rec.Code)
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "base_url and api_key")
}

func TestHandleTrueNASConnectorTestRejectsDisallowedOutboundURL(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	sut := hubdeps.New(t)
	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{
		"base_url":"https://blocked.example.net",
		"api_key":"test-key"
	}`))
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

	// A refused address is a failed test in words (422), like every other test failure.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for disallowed outbound URL, got %d body=%s", rec.Code, rec.Body.String())
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "allowed outbound addresses")
}

func TestHandleConnectorActionsTrueNASTestRateLimit(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	sut := hubdeps.New(t)
	sut.ConnectorRegistry.Register(truenas.New())

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
		req.RemoteAddr = "203.0.113.42:4402"
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"base_url":"https://truenas.local"}`))
	req.RemoteAddr = "203.0.113.42:4402"
	rec := httptest.NewRecorder()
	sut.CollectorsDeps().HandleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

func TestHandleTrueNASConnectorTestCredentialResolutionErrors(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Run("credential store unavailable", func(t *testing.T) {
		sut := hubdeps.NewEmpty(t)

		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"cred-1"}`))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("credential not found", func(t *testing.T) {
		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"missing"}`))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "credential_id not found")
	})

	t.Run("credential decrypt failure", func(t *testing.T) {
		sut := hubdeps.New(t)
		const credentialID = "cred-truenas-bad-cipher"
		_, err := sut.CredentialStore.CreateCredentialProfile(credentials.Profile{
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
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "failed to decrypt credential secret")
	})
}

func TestHandleTrueNASConnectorTestSuccessAndFailure(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)
	t.Run("success with credential fallback", func(t *testing.T) {
		server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
			if method == "system.info" {
				return map[string]any{"hostname": "VaultNAS", "version": "25.04.0"}, nil
			}
			return nil, &testutil.TrueNASRPCError{Code: -32601, Message: "Method not found"}
		})
		defer server.Close()

		sut := hubdeps.New(t)
		testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, "cred-truenas-1", "api-key-1", server.URL)

		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(`{"credential_id":"cred-truenas-1","skip_verify":false}`))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"status":"ok"`) || !strings.Contains(rec.Body.String(), "connected to VaultNAS") {
			t.Fatalf("unexpected success payload: %s", rec.Body.String())
		}
	})

	t.Run("upstream health failure", func(t *testing.T) {
		server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "permission denied"}
		})
		defer server.Close()

		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"api_key":"bad-key"
		}`, server.URL)))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
		// A plain reason that names the product, never the generic 5xx text.
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "TrueNAS")
	})

	t.Run("upstream health failure redacts api key", func(t *testing.T) {
		const leakedSecret = "truenas-api-key-should-not-leak"
		server := testutil.NewTrueNASRPCServer(t, func(method string, params []any) (any, *testutil.TrueNASRPCError) {
			return nil, &testutil.TrueNASRPCError{Code: -32000, Message: "api_key=" + leakedSecret}
		})
		defer server.Close()

		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodPost, "/connectors/truenas/test", strings.NewReader(fmt.Sprintf(`{
			"base_url":"%s",
			"api_key":"%s"
		}`, server.URL, leakedSecret)))
		rec := httptest.NewRecorder()
		sut.CollectorsDeps().HandleTrueNASConnectorTest(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
		}
		// A plain reason that names the product, never the generic 5xx text.
		testutil.AssertErrorBodyContains(t, rec.Body.Bytes(), "TrueNAS")
		if strings.Contains(rec.Body.String(), leakedSecret) {
			t.Fatalf("expected response to redact leaked api key, got %s", rec.Body.String())
		}
	})
}
