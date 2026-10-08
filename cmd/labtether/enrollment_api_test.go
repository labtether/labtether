package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/groups"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func signedTokenEnrollmentRequest(t *testing.T, rawToken, hostname string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) enrollment.EnrollRequest {
	t.Helper()
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	payload := agentidentity.BuildTokenEnrollmentProofPayload(hostname, rawToken, fingerprint)
	return enrollment.EnrollRequest{
		EnrollmentToken:   rawToken,
		Hostname:          hostname,
		Platform:          "linux",
		DeviceKeyAlg:      agentidentity.KeyAlgorithmEd25519,
		DevicePublicKey:   base64.StdEncoding.EncodeToString(publicKey),
		DeviceFingerprint: fingerprint,
		DeviceSignature:   base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
}

func signedContinuityEnrollmentRequest(t *testing.T, rawToken, hostname string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) enrollment.EnrollRequest {
	t.Helper()
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	assetID := normalizeHostnameForAssetID(hostname)
	payload := agentidentity.BuildTokenEnrollmentProofPayloadV2(assetID, rawToken, fingerprint)
	return enrollment.EnrollRequest{
		EnrollmentToken:    rawToken,
		Hostname:           hostname,
		Platform:           "linux",
		DeviceKeyAlg:       agentidentity.KeyAlgorithmEd25519,
		DevicePublicKey:    base64.StdEncoding.EncodeToString(publicKey),
		DeviceFingerprint:  fingerprint,
		DeviceSignature:    base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
		DeviceProofVersion: enrollment.DeviceProofVersionV2,
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

func TestEnrollRejectsUnapprovedGroupAndForcesApprovedPlacement(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)

	rawMissingGroupToken, _ := mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{
		Label: "unplaced", TTLHours: 24, MaxUses: 2, Scope: enrollment.TokenScopeUnplaced,
	})
	missingGroupPayload, err := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawMissingGroupToken,
		Hostname:        "QAWindowsHost",
		Platform:        "windows",
		GroupID:         "qa",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingGroupRequest := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(missingGroupPayload))
	missingGroupRequest.Host = "localhost:8080"
	missingGroupRequest.RemoteAddr = "127.0.0.1:12345"
	missingGroupRecorder := httptest.NewRecorder()
	sut.handleEnroll(missingGroupRecorder, missingGroupRequest)
	if missingGroupRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unapproved group enrollment: status=%d body=%s", missingGroupRecorder.Code, missingGroupRecorder.Body.String())
	}
	if _, exists, err := sut.assetStore.GetAsset("qawindowshost"); err != nil || exists {
		t.Fatalf("rejected group enrollment created asset: exists=%v err=%v", exists, err)
	}
	if token, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(rawMissingGroupToken)); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("missing group token state: token=%+v valid=%v err=%v", token, valid, err)
	}

	validGroup, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "Enrollment group", Slug: "enrollment-group"})
	if err != nil {
		t.Fatal(err)
	}
	rawValidGroupToken, _ := mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{
		Label: "group", TTLHours: 24, MaxUses: 2, Scope: enrollment.TokenScopeGroup, AllowedGroupID: validGroup.ID,
	})
	otherGroup, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "Other group", Slug: "other-group"})
	if err != nil {
		t.Fatal(err)
	}
	outsideGroupPayload, err := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawValidGroupToken,
		Hostname:        "outside-group-agent",
		Platform:        "linux",
		GroupID:         otherGroup.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	outsideGroupRequest := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(outsideGroupPayload))
	outsideGroupRequest.RemoteAddr = "127.0.0.3:12345"
	outsideGroupRecorder := httptest.NewRecorder()
	sut.handleEnroll(outsideGroupRecorder, outsideGroupRequest)
	if outsideGroupRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("outside group enrollment: status=%d body=%s", outsideGroupRecorder.Code, outsideGroupRecorder.Body.String())
	}
	if token, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(rawValidGroupToken)); err != nil || !valid || token.UseCount != 0 {
		t.Fatalf("outside group consumed token: token=%+v valid=%v err=%v", token, valid, err)
	}
	validGroupPayload, err := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawValidGroupToken,
		Hostname:        "grouped-agent",
		Platform:        "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	validGroupRequest := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(validGroupPayload))
	validGroupRequest.Host = "localhost:8080"
	validGroupRequest.RemoteAddr = "127.0.0.2:12345"
	validGroupRecorder := httptest.NewRecorder()
	sut.handleEnroll(validGroupRecorder, validGroupRequest)
	if validGroupRecorder.Code != http.StatusOK {
		t.Fatalf("valid group enrollment: status=%d body=%s", validGroupRecorder.Code, validGroupRecorder.Body.String())
	}
	var validGroupResponse enrollment.EnrollResponse
	if err := json.Unmarshal(validGroupRecorder.Body.Bytes(), &validGroupResponse); err != nil {
		t.Fatalf("decode valid-group enrollment response: %v", err)
	}
	if validGroupResponse.GroupID != validGroup.ID {
		t.Fatalf("valid-group response=%q, want %q", validGroupResponse.GroupID, validGroup.ID)
	}
	validGroupAsset, exists, err := sut.assetStore.GetAsset("grouped-agent")
	if err != nil || !exists {
		t.Fatalf("valid group asset: exists=%v err=%v", exists, err)
	}
	if validGroupAsset.GroupID != validGroup.ID {
		t.Fatalf("valid group=%q, want %q", validGroupAsset.GroupID, validGroup.ID)
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

func TestAgentTokenListAndRevoke(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
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
	serverConn, _, cleanupConn := createWSPairForPendingEnrollmentTest(t)
	defer cleanupConn()
	liveConn := agentmgr.NewAgentConn(serverConn, "agent-tok-node", "linux")
	liveConn.SetMeta("auth.mode", "agent-token")
	liveConn.SetMeta("auth.agent_token_id", agentTokID)
	sut.agentMgr.Register(liveConn)

	// Revoke agent token
	revokeReq := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/"+agentTokID, nil)
	revokeRec := httptest.NewRecorder()
	sut.handleAgentTokenActions(revokeRec, revokeReq)

	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", revokeRec.Code, revokeRec.Body.String())
	}
	if sut.agentMgr.IsConnected("agent-tok-node") {
		t.Fatal("revoked local agent token left its matching socket registered")
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

func TestAgentTokenActions_EmptyID(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/settings/agent-tokens/", nil)
	rec := httptest.NewRecorder()
	sut.handleAgentTokenActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
