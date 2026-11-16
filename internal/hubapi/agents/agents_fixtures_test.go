package agents_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/enrollment"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

// newAgentCacheHub is a bare Hub with an agent cache, like cmd's
// &apiServer{agentCache: cache}.
func newAgentCacheHub(t *testing.T, cache *agentspkg.AgentCache) *hubdeps.Hub {
	h := hubdeps.NewEmpty(t)
	h.AgentCache = cache
	return h
}

// The mustCreateEnrollmentToken helpers are copies of cmd's
// (enrollment_token_api_test.go), which many cmd tests still use.

func mustCreateEnrollmentToken(t *testing.T, sut *hubdeps.Hub) (rawToken string, tok enrollment.EnrollmentToken) {
	return mustCreateEnrollmentTokenWithMaxUses(t, sut, 10)
}

func mustCreateEnrollmentTokenWithMaxUses(t *testing.T, sut *hubdeps.Hub, maxUses int) (rawToken string, tok enrollment.EnrollmentToken) {
	t.Helper()
	return mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{
		Label: "test-token", TTLHours: 24, MaxUses: maxUses,
		Scope: enrollment.TokenScopeUnrestricted, AcknowledgeUnrestricted: true,
	})
}

func mustCreateEnrollmentTokenWithRequest(t *testing.T, sut *hubdeps.Hub, createRequest enrollment.CreateTokenRequest) (rawToken string, tok enrollment.EnrollmentToken) {
	t.Helper()

	payload, err := json.Marshal(createRequest)
	if err != nil {
		t.Fatalf("encode enrollment token request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleEnrollmentTokens(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating enrollment token, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Token    enrollment.EnrollmentToken `json:"token"`
		RawToken string                     `json:"raw_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode enrollment token response: %v", err)
	}
	if resp.RawToken == "" {
		t.Fatalf("expected raw_token in response")
	}
	if resp.Token.ID == "" {
		t.Fatalf("expected token ID in response")
	}
	return resp.RawToken, resp.Token
}
