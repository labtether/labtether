package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
)

type discoverResponse struct {
	Hub           string                   `json:"hub"`
	APIURL        string                   `json:"api_url"`
	WSURL         string                   `json:"ws_url"`
	EnrollURL     string                   `json:"enroll_url"`
	HubURL        string                   `json:"hub_url"`
	HubWSURL      string                   `json:"hub_ws_url"`
	HubCandidates []hubConnectionCandidate `json:"hub_candidates"`
}

func disableTailscaleResolutionForTest(t *testing.T) {
	t.Helper()

	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(string) (string, error) {
		return "", net.ErrClosed
	}
	tailscaleRunner = func(time.Duration, string, ...string) ([]byte, error) {
		t.Fatalf("tailscale CLI should not run when disabled for this test")
		return nil, nil
	}
	tailscaleFallbackPaths = func() []string { return nil }
}

func TestDiscoverEndpoint(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.Hub != "labtether" {
		t.Fatalf("expected hub=labtether, got %q", resp.Hub)
	}
	if resp.APIURL != "http://localhost:8080" {
		t.Fatalf("expected api_url=http://localhost:8080, got %q", resp.APIURL)
	}
	if resp.WSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected ws_url=ws://localhost:8080/ws/agent, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "http://localhost:8080/api/v1/enroll" {
		t.Fatalf("expected enroll_url, got %q", resp.EnrollURL)
	}
	if resp.HubURL != resp.APIURL {
		t.Fatalf("expected hub_url to mirror api_url, got hub_url=%q api_url=%q", resp.HubURL, resp.APIURL)
	}
	if resp.HubWSURL != resp.WSURL {
		t.Fatalf("expected hub_ws_url to mirror ws_url, got hub_ws_url=%q ws_url=%q", resp.HubWSURL, resp.WSURL)
	}
	if len(resp.HubCandidates) == 0 {
		t.Fatalf("expected discover response to include hub_candidates")
	}
}

func TestDiscoverEndpoint_UsesExternalURLForHubEndpoints(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.externalURL = "https://hub.example.com:9443"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "attacker.local:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://hub.example.com:9443" {
		t.Fatalf("expected api_url to use sanitized external host, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://hub.example.com:9443/ws/agent" {
		t.Fatalf("expected ws_url to use sanitized external host, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://hub.example.com:9443/api/v1/enroll" {
		t.Fatalf("expected enroll_url to use sanitized external host, got %q", resp.EnrollURL)
	}
}

func TestDiscoverEndpoint_IgnoresHTTPExternalURLWhenTLSEnabled(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.externalURL = "http://hub.example.com:8080"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "secure.local:8443"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://secure.local:8443" {
		t.Fatalf("expected api_url to fall back to secure request host, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://secure.local:8443/ws/agent" {
		t.Fatalf("expected ws_url to fall back to secure request host, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://secure.local:8443/api/v1/enroll" {
		t.Fatalf("expected enroll_url to fall back to secure request host, got %q", resp.EnrollURL)
	}
}

func TestDiscoverEndpoint_PrefersTailscaleCandidateWhenAvailable(t *testing.T) {
	t.Setenv("API_PORT", "8080")

	disableTailscaleResolutionForTest(t)
	withMockHubInterfaces(t,
		[]net.Interface{
			{Index: 1, Name: "tailscale0", Flags: net.FlagUp},
			{Index: 2, Name: "en0", Flags: net.FlagUp},
		},
		map[int][]net.Addr{
			1: {ipNet("100.96.0.5/32")},
			2: {ipNet("192.168.1.40/24")},
		},
	)

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.HubURL != "http://100.96.0.5:8080" {
		t.Fatalf("expected tailscale hub_url, got %q", resp.HubURL)
	}
	if len(resp.HubCandidates) < 2 {
		t.Fatalf("expected at least two hub candidates, got %d", len(resp.HubCandidates))
	}
	if resp.HubCandidates[0].Kind != "tailscale" {
		t.Fatalf("expected first hub candidate kind tailscale, got %q", resp.HubCandidates[0].Kind)
	}
}

func TestBuildHTTPHandlers_DiscoverEndpointAllowsUnauthenticatedRequests(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	discoverHandler, ok := handlers["/api/v1/discover"]
	if !ok || discoverHandler == nil {
		t.Fatalf("expected /api/v1/discover handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	discoverHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected unauthenticated discover request to return 200, got %d", rec.Code)
	}
}

func TestBuildHTTPHandlers_DiscoverEndpointRejectsNonGETWithoutAuth(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	discoverHandler, ok := handlers["/api/v1/discover"]
	if !ok || discoverHandler == nil {
		t.Fatalf("expected /api/v1/discover handler")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	rec := httptest.NewRecorder()
	discoverHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected discover POST to return 405 without auth, got %d", rec.Code)
	}
}

func TestDiscoverRejectsNonGET(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func mustCreateEnrollmentToken(t *testing.T, sut *apiServer) (rawToken string, tok enrollment.EnrollmentToken) {
	t.Helper()

	payload := []byte(`{"label":"test-token","ttl_hours":24,"max_uses":10}`)
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

func TestEnrollmentTokenDefaultTTL(t *testing.T) {
	sut := newTestAPIServer(t)

	// TTL=0 should default to 24h
	payload := []byte(`{"label":"default-ttl"}`)
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

	payload := []byte(`{"label":"default-max-uses","ttl_hours":24}`)
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

	payload := []byte(`{"label":"zero-max-uses","ttl_hours":24,"max_uses":0}`)
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

func TestEnrollFullFlow(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)

	// Step 1: Create enrollment token
	rawToken, _ := mustCreateEnrollmentToken(t, sut)

	// Step 2: Enroll an agent
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "test-node-01",
		Platform:        "linux",
	})

	enrollReq := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	enrollReq.Host = "localhost:8080"
	enrollReq.RemoteAddr = "127.0.0.1:12345"
	enrollRec := httptest.NewRecorder()
	sut.handleEnroll(enrollRec, enrollReq)

	if enrollRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", enrollRec.Code, enrollRec.Body.String())
	}

	var enrollResp enrollment.EnrollResponse
	if err := json.Unmarshal(enrollRec.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if enrollResp.AgentToken == "" {
		t.Fatalf("expected agent_token in response")
	}
	if enrollResp.AssetID != "test-node-01" {
		t.Fatalf("expected asset_id=test-node-01, got %q", enrollResp.AssetID)
	}
	if enrollResp.HubWSURL == "" {
		t.Fatalf("expected hub_ws_url in response")
	}

	// Step 3: Verify agent token is valid
	agentHash := auth.HashToken(enrollResp.AgentToken)
	atok, valid, err := sut.enrollmentStore.ValidateAgentToken(agentHash)
	if err != nil {
		t.Fatalf("validate agent token error: %v", err)
	}
	if !valid {
		t.Fatalf("expected agent token to be valid")
	}
	if atok.AssetID != "test-node-01" {
		t.Fatalf("expected agent token asset_id=test-node-01, got %q", atok.AssetID)
	}
	ttlHours := configuredAgentTokenTTLHours()
	expectedMinExpiry := time.Now().UTC().Add(time.Duration(ttlHours-1) * time.Hour)
	expectedMaxExpiry := time.Now().UTC().Add(time.Duration(ttlHours+1) * time.Hour)
	if atok.ExpiresAt.Before(expectedMinExpiry) || atok.ExpiresAt.After(expectedMaxExpiry) {
		t.Fatalf("expected agent token expiry around %dh from now, got %s", ttlHours, atok.ExpiresAt.Format(time.RFC3339))
	}

	// Step 4: Verify asset was created
	allAssets, err := sut.assetStore.ListAssets()
	if err != nil {
		t.Fatalf("list assets error: %v", err)
	}
	found := false
	for _, a := range allAssets {
		if a.ID == "test-node-01" {
			found = true
			if a.Source != "agent" {
				t.Fatalf("expected source=agent, got %q", a.Source)
			}
		}
	}
	if !found {
		t.Fatalf("enrolled asset not found in asset list")
	}

	// Step 5: Verify enrollment token use count incremented
	etokHash := auth.HashToken(rawToken)
	etok, _, err := sut.enrollmentStore.ValidateEnrollmentToken(etokHash)
	if err != nil {
		t.Fatalf("validate enrollment token error: %v", err)
	}
	if etok.UseCount != 1 {
		t.Fatalf("expected enrollment token use_count=1, got %d", etok.UseCount)
	}
}

func TestEnrollRejectsInvalidToken(t *testing.T) {
	sut := newTestAPIServer(t)

	enrollPayload := []byte(`{"enrollment_token":"invalid-token","hostname":"bad-node","platform":"linux"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnrollRejectsRevokedToken(t *testing.T) {
	sut := newTestAPIServer(t)
	rawToken, tok := mustCreateEnrollmentToken(t, sut)

	// Revoke
	if err := sut.enrollmentStore.RevokeEnrollmentToken(tok.ID); err != nil {
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
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnrollRejectsMissingFields(t *testing.T) {
	sut := newTestAPIServer(t)

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
			sut.handleEnroll(rec, req)

			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 400 or 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEnrollRejectsReEnrollmentForExistingHostname(t *testing.T) {
	sut := newTestAPIServer(t)
	rawToken, _ := mustCreateEnrollmentToken(t, sut)

	// First enrollment
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "re-enroll-node",
		Platform:        "linux",
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req1.Host = "localhost:8080"
	req1.RemoteAddr = "127.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	sut.handleEnroll(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first enroll: expected 200, got %d", rec1.Code)
	}
	var resp1 enrollment.EnrollResponse
	json.Unmarshal(rec1.Body.Bytes(), &resp1)
	firstToken := resp1.AgentToken

	// Second enrollment for the same hostname must be rejected to avoid
	// silently transferring the asset identity to a new caller.
	enrollPayload2, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "re-enroll-node",
		Platform:        "linux",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload2))
	req2.Host = "localhost:8080"
	req2.RemoteAddr = "127.0.0.1:12345"
	rec2 := httptest.NewRecorder()
	sut.handleEnroll(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second enroll: expected 409, got %d", rec2.Code)
	}

	// The original agent token must remain valid because the conflicting
	// re-enrollment request was rejected.
	oldHash := auth.HashToken(firstToken)
	_, valid, _ := sut.enrollmentStore.ValidateAgentToken(oldHash)
	if !valid {
		t.Fatalf("expected original agent token to remain valid after rejected re-enrollment")
	}
}

func TestAgentTokenListAndRevoke(t *testing.T) {
	sut := newTestAPIServer(t)
	rawToken, _ := mustCreateEnrollmentToken(t, sut)

	// Enroll to create an agent token
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "agent-tok-node",
		Platform:        "linux",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll: expected 200, got %d", rec.Code)
	}

	// List agent tokens
	listReq := httptest.NewRequest(http.MethodGet, "/settings/agent-tokens", nil)
	listRec := httptest.NewRecorder()
	sut.handleAgentTokens(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var listResp struct {
		Tokens []enrollment.AgentToken `json:"tokens"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(listResp.Tokens) != 1 {
		t.Fatalf("expected 1 agent token, got %d", len(listResp.Tokens))
	}
	agentTokID := listResp.Tokens[0].ID

	// Revoke agent token
	revokeReq := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/"+agentTokID, nil)
	revokeRec := httptest.NewRecorder()
	sut.handleAgentTokenActions(revokeRec, revokeReq)

	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", revokeRec.Code, revokeRec.Body.String())
	}

	// Verify revoked
	listReq2 := httptest.NewRequest(http.MethodGet, "/settings/agent-tokens", nil)
	listRec2 := httptest.NewRecorder()
	sut.handleAgentTokens(listRec2, listReq2)

	var listResp2 struct {
		Tokens []enrollment.AgentToken `json:"tokens"`
	}
	json.Unmarshal(listRec2.Body.Bytes(), &listResp2)
	for _, tok := range listResp2.Tokens {
		if tok.ID == agentTokID && tok.Status != "revoked" {
			t.Fatalf("expected agent token status=revoked, got %q", tok.Status)
		}
	}
}

func TestEnrollAgentTokenHonorsConfiguredTTL(t *testing.T) {
	t.Setenv("LABTETHER_AGENT_TOKEN_TTL_HOURS", "2")
	sut := newTestAPIServer(t)

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "ttl-node-01",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var enrollResp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	agentHash := auth.HashToken(enrollResp.AgentToken)
	atok, valid, err := sut.enrollmentStore.ValidateAgentToken(agentHash)
	if err != nil {
		t.Fatalf("validate agent token error: %v", err)
	}
	if !valid {
		t.Fatalf("expected newly issued agent token to be valid")
	}

	expectedMinExpiry := time.Now().UTC().Add(1 * time.Hour)
	expectedMaxExpiry := time.Now().UTC().Add(3 * time.Hour)
	if atok.ExpiresAt.Before(expectedMinExpiry) || atok.ExpiresAt.After(expectedMaxExpiry) {
		t.Fatalf("expected agent token expiry ~2h from now, got %s", atok.ExpiresAt.Format(time.RFC3339))
	}
}

func TestAgentTokenValidationRejectsExpiredToken(t *testing.T) {
	sut := newTestAPIServer(t)

	_, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	if _, err := sut.enrollmentStore.CreateAgentToken("expired-agent-01", tokenHash, "test", expiredAt); err != nil {
		t.Fatalf("create agent token: %v", err)
	}

	_, valid, err := sut.enrollmentStore.ValidateAgentToken(tokenHash)
	if err != nil {
		t.Fatalf("validate agent token: %v", err)
	}
	if valid {
		t.Fatalf("expected expired agent token to be invalid")
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

func TestAgentTokenActions_EmptyID(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentTokenActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHubSchemes_PlainHTTP(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = false

	httpScheme, wsScheme := sut.hubSchemes()
	if httpScheme != "http" {
		t.Fatalf("expected http, got %q", httpScheme)
	}
	if wsScheme != "ws" {
		t.Fatalf("expected ws, got %q", wsScheme)
	}
}

func TestHubSchemes_TLS(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	httpScheme, wsScheme := sut.hubSchemes()
	if httpScheme != "https" {
		t.Fatalf("expected https, got %q", httpScheme)
	}
	if wsScheme != "wss" {
		t.Fatalf("expected wss, got %q", wsScheme)
	}
}

func TestDiscoverEndpoint_TLS(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://localhost:8080" {
		t.Fatalf("expected https api_url, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://localhost:8080/ws/agent" {
		t.Fatalf("expected wss ws_url, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://localhost:8080/api/v1/enroll" {
		t.Fatalf("expected https enroll_url, got %q", resp.EnrollURL)
	}
}

func TestEnroll_IncludesCACertPEM(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----\n")

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "ca-cert-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.CACertPEM != string(sut.tlsState.CACertPEM) {
		t.Fatalf("expected ca_cert_pem=%q, got %q", string(sut.tlsState.CACertPEM), resp.CACertPEM)
	}
}

func TestEnroll_OmitsCACertPEMWhenNil(t *testing.T) {
	sut := newTestAPIServer(t)
	// caCertPEM is nil by default

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "no-ca-cert-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify ca_cert_pem is omitted from JSON (not present as a key)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, exists := raw["ca_cert_pem"]; exists {
		t.Fatalf("expected ca_cert_pem to be omitted when caCertPEM is nil, but it was present: %v", raw["ca_cert_pem"])
	}

	// Also verify the typed response has empty string
	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.CACertPEM != "" {
		t.Fatalf("expected empty ca_cert_pem, got %q", resp.CACertPEM)
	}
}

func TestEnrollFullFlow_URLSchemes(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)

	// Test with TLS disabled — URLs should use http/ws
	rawToken, _ := mustCreateEnrollmentToken(t, sut)

	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "scheme-test-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.HubWSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected ws:// URL, got %q", resp.HubWSURL)
	}
	if resp.HubAPIURL != "http://localhost:8080" {
		t.Fatalf("expected http:// URL, got %q", resp.HubAPIURL)
	}

	// Test with TLS enabled — URLs should use https/wss
	sut.tlsState.Enabled = true
	rawToken2, _ := mustCreateEnrollmentToken(t, sut)

	enrollPayload2, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken2,
		Hostname:        "scheme-test-node-tls",
		Platform:        "linux",
	})

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload2))
	req2.Host = "localhost:8080"
	req2.RemoteAddr = "127.0.0.1:12345"
	rec2 := httptest.NewRecorder()
	sut.handleEnroll(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp2 enrollment.EnrollResponse
	json.Unmarshal(rec2.Body.Bytes(), &resp2)
	if resp2.HubWSURL != "wss://localhost:8080/ws/agent" {
		t.Fatalf("expected wss:// URL, got %q", resp2.HubWSURL)
	}
	if resp2.HubAPIURL != "https://localhost:8080" {
		t.Fatalf("expected https:// URL, got %q", resp2.HubAPIURL)
	}
}

func TestEnrollFullFlow_UsesExternalURLForHubEndpoints(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.externalURL = "https://hub.example.com:9443"

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "external-url-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "attacker.local:9999"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.HubAPIURL != "https://hub.example.com:9443" {
		t.Fatalf("expected external hub_api_url, got %q", resp.HubAPIURL)
	}
	if resp.HubWSURL != "wss://hub.example.com:9443/ws/agent" {
		t.Fatalf("expected external hub_ws_url, got %q", resp.HubWSURL)
	}
}

func TestEnrollFullFlow_PrefersTailscaleHubEndpointsWhenAvailable(t *testing.T) {
	t.Setenv(envTailscaleManaged, "true")

	sut := newTestAPIServer(t)

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleFallbackPaths = func() []string { return nil }
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": {
					"DNSName": "hub.homelab.ts.net.",
					"TailscaleIPs": ["100.101.102.103"]
				}
			}`), nil
		case "serve status --json":
			return []byte(`{
				"TCP": {
					"443": {
						"HTTPS": true,
						"Web": {
							"/": {
								"Proxy": "http://127.0.0.1:3000"
							}
						}
					}
				}
			}`), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "tailscale-enroll-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8443"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.HubAPIURL != "https://hub.homelab.ts.net" {
		t.Fatalf("expected tailscale hub_api_url, got %q", resp.HubAPIURL)
	}
	if resp.HubWSURL != "wss://hub.homelab.ts.net/ws/agent" {
		t.Fatalf("expected tailscale hub_ws_url, got %q", resp.HubWSURL)
	}
}
