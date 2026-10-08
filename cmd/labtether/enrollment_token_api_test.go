package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func mustCreateEnrollmentToken(t *testing.T, sut *apiServer) (rawToken string, tok enrollment.EnrollmentToken) {
	return mustCreateEnrollmentTokenWithMaxUses(t, sut, 10)
}

func mustCreateEnrollmentTokenWithMaxUses(t *testing.T, sut *apiServer, maxUses int) (rawToken string, tok enrollment.EnrollmentToken) {
	t.Helper()
	return mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{
		Label: "test-token", TTLHours: 24, MaxUses: maxUses,
		Scope: enrollment.TokenScopeUnrestricted, AcknowledgeUnrestricted: true,
	})
}

func mustCreateEnrollmentTokenWithRequest(t *testing.T, sut *apiServer, createRequest enrollment.CreateTokenRequest) (rawToken string, tok enrollment.EnrollmentToken) {
	t.Helper()

	payload, err := json.Marshal(createRequest)
	if err != nil {
		t.Fatalf("encode enrollment token request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(rec, req)

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

func TestEnrollmentTokenCreateAndList(t *testing.T) {
	sut := newTestAPIServer(t)

	rawToken, tok := mustCreateEnrollmentToken(t, sut)
	if tok.Label != "test-token" {
		t.Fatalf("expected label 'test-token', got %q", tok.Label)
	}
	if tok.MaxUses != 10 {
		t.Fatalf("expected max_uses=10, got %d", tok.MaxUses)
	}
	_ = rawToken

	// List enrollment tokens
	listReq := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	listReq.Host = "localhost:8080"
	listRec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var listResp struct {
		Tokens []enrollment.EnrollmentToken `json:"tokens"`
		HubURL string                       `json:"hub_url"`
		WSURL  string                       `json:"ws_url"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(listResp.Tokens))
	}
	if listResp.Tokens[0].ID != tok.ID {
		t.Fatalf("expected token ID %s, got %s", tok.ID, listResp.Tokens[0].ID)
	}
}

func TestEnrollmentTokenCreationRejectsUnsafeScopesBeforePersisting(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "missing scope", payload: `{"label":"missing"}`},
		{name: "asset missing id", payload: `{"scope":"asset","max_uses":1}`},
		{name: "asset multiple uses", payload: `{"scope":"asset","asset_id":"node","max_uses":2}`},
		{name: "group missing id", payload: `{"scope":"group","max_uses":1}`},
		{name: "group does not exist", payload: `{"scope":"group","allowed_group_id":"missing","max_uses":1}`},
		{name: "unrestricted missing acknowledgement", payload: `{"scope":"unrestricted","max_uses":1}`},
		{name: "unplaced with group", payload: `{"scope":"unplaced","allowed_group_id":"group","max_uses":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewBufferString(tt.payload))
			req = req.WithContext(contextWithPrincipal(req.Context(), "operator-a", "admin"))
			rec := httptest.NewRecorder()
			sut.handleEnrollmentTokens(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "raw_token") {
				t.Fatalf("rejected request returned a raw token: %s", rec.Body.String())
			}
			if tokens, err := sut.enrollmentStore.ListEnrollmentTokens(10); err != nil || len(tokens) != 0 {
				t.Fatalf("rejected request persisted tokens=%+v err=%v", tokens, err)
			}
		})
	}
}

func TestEnrollmentTokenCreationPersistsScopeAndWritesRedactedAudit(t *testing.T) {
	sut := newTestAPIServer(t)
	payload := []byte(`{"label":"one host","ttl_hours":24,"max_uses":1,"scope":"asset","asset_id":"Expected Host"}`)
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	req = req.WithContext(contextWithPrincipal(req.Context(), "operator-a", "admin"))
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Token    enrollment.EnrollmentToken `json:"token"`
		RawToken string                     `json:"raw_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Token.Scope != enrollment.TokenScopeAsset || response.Token.AssetID != "expected-host" || response.Token.CreatedBy != "operator-a" {
		t.Fatalf("unexpected token claims: %+v", response.Token)
	}
	events, err := sut.auditStore.List(10, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("audit events=%+v err=%v", events, err)
	}
	encodedAudit, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Type != "enrollment.token.created" || events[0].ActorID != "operator-a" || events[0].Target != response.Token.ID {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
	if strings.Contains(string(encodedAudit), response.RawToken) || strings.Contains(string(encodedAudit), auth.HashToken(response.RawToken)) {
		t.Fatalf("audit event exposed enrollment secret: %s", encodedAudit)
	}
}

func TestAssetScopedEnrollmentTokenCannotEnrollAnotherHostname(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)
	rawToken, _ := mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{
		Label: "one-host", TTLHours: 24, MaxUses: 1,
		Scope: enrollment.TokenScopeAsset, AssetID: "expected-node",
	})
	enroll := func(hostname string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(enrollment.EnrollRequest{
			EnrollmentToken: rawToken, Hostname: hostname, Platform: "linux",
		})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(payload))
		req.Host = "localhost:8080"
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		sut.handleEnroll(rec, req)
		return rec
	}
	if rec := enroll("other-node"); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid or expired enrollment token") {
		t.Fatalf("wrong hostname status=%d body=%s", rec.Code, rec.Body.String())
	}
	if token, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(rawToken)); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("wrong hostname consumed token: token=%+v valid=%v err=%v", token, valid, err)
	}
	if rec := enroll("expected-node"); rec.Code != http.StatusOK {
		t.Fatalf("expected hostname status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnrollmentTokenDefaultTTL(t *testing.T) {
	sut := newTestAPIServer(t)

	// TTL=0 should default to 24h
	payload := []byte(`{"label":"default-ttl","scope":"unplaced"}`)
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Token enrollment.EnrollmentToken `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Token should expire roughly 24h from now
	expectedMin := time.Now().UTC().Add(23 * time.Hour)
	expectedMax := time.Now().UTC().Add(25 * time.Hour)
	if resp.Token.ExpiresAt.Before(expectedMin) || resp.Token.ExpiresAt.After(expectedMax) {
		t.Fatalf("expected expiry ~24h from now, got %v", resp.Token.ExpiresAt)
	}
}

func TestEnrollmentTokenDefaultMaxUsesIsOne(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"label":"default-max-uses","ttl_hours":24,"scope":"unplaced"}`)
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Token enrollment.EnrollmentToken `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Token.MaxUses != 1 {
		t.Fatalf("expected default max_uses=1, got %d", resp.Token.MaxUses)
	}
}

func TestEnrollmentTokenZeroMaxUsesIsClampedToOne(t *testing.T) {
	sut := newTestAPIServer(t)

	payload := []byte(`{"label":"zero-max-uses","ttl_hours":24,"max_uses":0,"scope":"unplaced"}`)
	req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokens(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Token enrollment.EnrollmentToken `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Token.MaxUses != 1 {
		t.Fatalf("expected clamped max_uses=1, got %d", resp.Token.MaxUses)
	}
}

func TestEnrollmentTokenMaxUsesRejectsConfiguredCeilingAndNegativeValues(t *testing.T) {
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN_MAX_USES", "2")
	for _, payload := range []string{
		`{"label":"too-many","ttl_hours":24,"max_uses":3}`,
		`{"label":"negative","ttl_hours":24,"max_uses":-1}`,
	} {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodPost, "/settings/enrollment", bytes.NewBufferString(payload))
		rec := httptest.NewRecorder()
		sut.handleEnrollmentTokens(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("payload=%s status=%d body=%s", payload, rec.Code, rec.Body.String())
		}
		if tokens, err := sut.enrollmentStore.ListEnrollmentTokens(10); err != nil || len(tokens) != 0 {
			t.Fatalf("rejected request persisted tokens=%+v err=%v", tokens, err)
		}
	}
}

func TestRevokeEnrollmentToken(t *testing.T) {
	sut := newTestAPIServer(t)
	_, tok := mustCreateEnrollmentToken(t, sut)

	// Revoke it
	req := httptest.NewRequest(http.MethodDelete, "/settings/enrollment/"+tok.ID, nil)
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokenActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["status"] != "revoked" {
		t.Fatalf("expected status=revoked, got %q", resp["status"])
	}
}

func TestEnrollmentTokenActions_EmptyID(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/enrollment/", nil)
	rec := httptest.NewRecorder()
	sut.handleEnrollmentTokenActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
