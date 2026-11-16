package agents_test

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnrollRejectsInvalidToken(t *testing.T) {
	sut := hubdeps.New(t)

	enrollPayload := []byte(`{"enrollment_token":"invalid-token","hostname":"bad-node","platform":"linux"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleEnroll(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnrollRejectsRevokedToken(t *testing.T) {
	sut := hubdeps.New(t)
	rawToken, tok := mustCreateEnrollmentToken(t, sut)

	// Revoke
	if err := sut.EnrollmentStore.RevokeEnrollmentToken(tok.ID); err != nil {
		t.Fatalf("revoke error: %v", err)
	}

	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "test-node",
		Platform:        "linux",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleEnroll(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnrollRejectsMissingFields(t *testing.T) {
	sut := hubdeps.New(t)

	tests := []struct {
		name    string
		payload string
	}{
		{"missing enrollment_token", `{"hostname":"test","platform":"linux"}`},
		{"missing hostname", `{"enrollment_token":"tok","platform":"linux"}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader([]byte(tt.payload)))
			req.RemoteAddr = "127.0.0.1:12345"
			rec := httptest.NewRecorder()
			sut.AgentsDeps().HandleEnroll(rec, req)

			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 400 or 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAgentTokenValidationRejectsExpiredToken(t *testing.T) {
	sut := hubdeps.New(t)

	_, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := sut.EnrollmentStore.CreateAgentToken("expired-agent-01", tokenHash, "test", expiredAt); err != nil {
		t.Fatalf("create agent token: %v", err)
	}

	_, valid, err := sut.EnrollmentStore.ValidateAgentToken(tokenHash)
	if err != nil {
		t.Fatalf("validate agent token: %v", err)
	}
	if valid {
		t.Fatalf("expected expired agent token to be invalid")
	}
}

func TestAgentTokenActions_EmptyID(t *testing.T) {
	sut := hubdeps.New(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/", nil)
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleAgentTokenActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
