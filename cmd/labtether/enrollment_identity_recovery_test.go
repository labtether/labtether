package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnrollRejectsRetiredAgentIdentityWithoutConsumingToken(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)
	transactions, ok := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if !ok {
		t.Fatal("test enrollment store lacks transaction interface")
	}

	firstToken, _ := mustCreateEnrollmentToken(t, sut)
	firstPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: firstToken,
		Hostname:        "retired-http-node",
		Platform:        "linux",
	})
	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(firstPayload))
	firstReq.RemoteAddr = "127.0.0.1:12345"
	firstRec := httptest.NewRecorder()
	sut.handleEnroll(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("initial enrollment status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	if err := transactions.DecommissionAgentAsset(context.Background(), "retired-http-node"); err != nil {
		t.Fatal(err)
	}

	retryToken, retryRecord := mustCreateEnrollmentToken(t, sut)
	retryPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: retryToken,
		Hostname:        "retired-http-node",
		Platform:        "linux",
	})
	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(retryPayload))
	retryReq.RemoteAddr = "127.0.0.1:12345"
	retryRec := httptest.NewRecorder()
	sut.handleEnroll(retryRec, retryReq)
	if retryRec.Code != http.StatusConflict || !strings.Contains(retryRec.Body.String(), "new hostname/asset ID") {
		t.Fatalf("retired enrollment status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	if token, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(retryToken)); err != nil || !valid || token.ID != retryRecord.ID || token.UseCount != 0 {
		t.Fatalf("retirement response mutated token: token=%+v valid=%v err=%v", token, valid, err)
	}

	invalidPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: "invalid-token",
		Hostname:        "retired-http-node",
		Platform:        "linux",
	})
	invalidReq := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(invalidPayload))
	invalidReq.RemoteAddr = "127.0.0.1:12345"
	invalidRec := httptest.NewRecorder()
	sut.handleEnroll(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token leaked retirement state: status=%d body=%s", invalidRec.Code, invalidRec.Body.String())
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

func TestEnrollAllowsContinuityProvenReEnrollmentAndRotatesCredential(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate device identity: %v", err)
	}

	firstRawToken, _ := mustCreateEnrollmentToken(t, sut)
	firstRequest := signedTokenEnrollmentRequest(t, firstRawToken, "continuity-node", publicKey, privateKey)
	firstPayload, _ := json.Marshal(firstRequest)
	firstHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(firstPayload))
	firstHTTP.Host = "localhost:8080"
	firstHTTP.RemoteAddr = "127.0.0.1:12345"
	firstRecorder := httptest.NewRecorder()
	sut.handleEnroll(firstRecorder, firstHTTP)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first enrollment: expected 200, got %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	var firstResponse enrollment.EnrollResponse
	if err := json.Unmarshal(firstRecorder.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("decode first enrollment: %v", err)
	}
	firstAsset, exists, err := sut.assetStore.GetAsset("continuity-node")
	if err != nil || !exists {
		t.Fatalf("get first asset: exists=%v err=%v", exists, err)
	}
	wantFingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	if got := firstAsset.Metadata["agent_device_fingerprint"]; got != wantFingerprint {
		t.Fatalf("stored fingerprint=%q, want %q", got, wantFingerprint)
	}
	firstStoredToken, valid, err := sut.enrollmentStore.ValidateAgentToken(auth.HashToken(firstResponse.AgentToken))
	if err != nil || !valid {
		t.Fatalf("load first agent token: valid=%v err=%v", valid, err)
	}
	serverConn, _, cleanupConn := createWSPairForPendingEnrollmentTest(t)
	defer cleanupConn()
	oldLiveConn := agentmgr.NewAgentConn(serverConn, "continuity-node", "linux")
	oldLiveConn.SetMeta("auth.mode", "agent-token")
	oldLiveConn.SetMeta("auth.agent_token_id", firstStoredToken.ID)
	enrollmentTransactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	oldLiveConn.SetCredentialValidatorWithLease(func() error {
		return enrollmentTransactions.ValidateActiveAgentTokenID(context.Background(), firstStoredToken.ID, "continuity-node")
	}, 5*time.Second)
	if err := oldLiveConn.ValidateCredential(); err != nil {
		t.Fatalf("prime old connection credential lease: %v", err)
	}
	sut.agentMgr.Register(oldLiveConn)

	secondRawToken, secondEnrollmentToken := mustCreateEnrollmentTokenWithMaxUses(t, sut, 1)
	secondRequest := signedContinuityEnrollmentRequest(t, secondRawToken, "continuity-node", publicKey, privateKey)
	secondPayload, _ := json.Marshal(secondRequest)
	secondHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(secondPayload))
	secondHTTP.Host = "localhost:8080"
	secondHTTP.RemoteAddr = "127.0.0.2:12345"
	secondRecorder := httptest.NewRecorder()
	sut.handleEnroll(secondRecorder, secondHTTP)
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("continuity re-enrollment: expected 200, got %d: %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var secondResponse enrollment.EnrollResponse
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("decode second enrollment: %v", err)
	}
	if secondResponse.AssetID != firstResponse.AssetID {
		t.Fatalf("asset identity changed from %q to %q", firstResponse.AssetID, secondResponse.AssetID)
	}
	if sut.agentMgr.IsConnected("continuity-node") {
		t.Fatal("recovery left the revoked connection registered")
	}
	if err := oldLiveConn.ValidateCredential(); !errors.Is(err, agentmgr.ErrAgentCredentialRejected) {
		t.Fatalf("revoked connection validation error=%v, want immediate rejection", err)
	}

	if _, valid, err := sut.enrollmentStore.ValidateAgentToken(auth.HashToken(firstResponse.AgentToken)); err != nil || valid {
		t.Fatalf("old agent token remained valid after rotation: valid=%v err=%v", valid, err)
	}
	rotatedToken, valid, err := sut.enrollmentStore.ValidateAgentToken(auth.HashToken(secondResponse.AgentToken))
	if err != nil || !valid {
		t.Fatalf("replacement agent token invalid: valid=%v err=%v", valid, err)
	}
	if rotatedToken.AssetID != "continuity-node" || rotatedToken.EnrolledVia != secondEnrollmentToken.ID {
		t.Fatalf("replacement token binding=%+v", rotatedToken)
	}

	allTokens, err := sut.enrollmentStore.ListAgentTokens(10)
	if err != nil {
		t.Fatalf("list agent tokens: %v", err)
	}
	activeForAsset := 0
	for _, token := range allTokens {
		if token.AssetID == "continuity-node" && token.Status == "active" {
			activeForAsset++
		}
	}
	if activeForAsset != 1 {
		t.Fatalf("active token count for continuity-node=%d, want 1", activeForAsset)
	}

	secondAsset, exists, err := sut.assetStore.GetAsset("continuity-node")
	if err != nil || !exists {
		t.Fatalf("get re-enrolled asset: exists=%v err=%v", exists, err)
	}
	if !secondAsset.CreatedAt.Equal(firstAsset.CreatedAt) {
		t.Fatalf("asset history was replaced: created_at before=%s after=%s", firstAsset.CreatedAt, secondAsset.CreatedAt)
	}
}

func TestEnrollRejectsExistingHostnameWhenDeviceKeyDoesNotMatch(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)

	trustedPublicKey, trustedPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate trusted identity: %v", err)
	}
	firstRawToken, _ := mustCreateEnrollmentToken(t, sut)
	firstRequest := signedTokenEnrollmentRequest(t, firstRawToken, "protected-node", trustedPublicKey, trustedPrivateKey)
	firstPayload, _ := json.Marshal(firstRequest)
	firstHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(firstPayload))
	firstHTTP.Host = "localhost:8080"
	firstHTTP.RemoteAddr = "127.0.0.1:12345"
	firstRecorder := httptest.NewRecorder()
	sut.handleEnroll(firstRecorder, firstHTTP)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first enrollment: expected 200, got %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	var firstResponse enrollment.EnrollResponse
	if err := json.Unmarshal(firstRecorder.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("decode first enrollment: %v", err)
	}

	attackerPublicKey, attackerPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate mismatched identity: %v", err)
	}
	secondRawToken, _ := mustCreateEnrollmentTokenWithMaxUses(t, sut, 1)
	secondRequest := signedContinuityEnrollmentRequest(t, secondRawToken, "protected-node", attackerPublicKey, attackerPrivateKey)
	secondPayload, _ := json.Marshal(secondRequest)
	secondHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(secondPayload))
	secondHTTP.Host = "localhost:8080"
	secondHTTP.RemoteAddr = "127.0.0.2:12345"
	secondRecorder := httptest.NewRecorder()
	sut.handleEnroll(secondRecorder, secondHTTP)
	if secondRecorder.Code != http.StatusConflict {
		t.Fatalf("mismatched identity: expected 409, got %d: %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if _, valid, err := sut.enrollmentStore.ValidateAgentToken(auth.HashToken(firstResponse.AgentToken)); err != nil || !valid {
		t.Fatalf("original token changed after rejected takeover: valid=%v err=%v", valid, err)
	}
	secondEnrollment, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(secondRawToken))
	if err != nil || !valid || secondEnrollment.UseCount != 0 {
		t.Fatalf("rejected takeover consumed enrollment token: valid=%v use_count=%d err=%v", valid, secondEnrollment.UseCount, err)
	}
}

func TestEnrollRejectsRecoveryFromHeartbeatOnlyFingerprint(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	initialEnrollmentToken, _ := mustCreateEnrollmentTokenWithMaxUses(t, sut, 1)
	initialPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: initialEnrollmentToken,
		Hostname:        "heartbeat-anchor-node",
		Platform:        "linux",
	})
	initialRequest := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(initialPayload))
	initialRequest.Host = "localhost:8080"
	initialRequest.RemoteAddr = "127.0.0.1:12345"
	initialRecorder := httptest.NewRecorder()
	sut.handleEnroll(initialRecorder, initialRequest)
	if initialRecorder.Code != http.StatusOK {
		t.Fatalf("initial unsigned enrollment: expected 200, got %d: %s", initialRecorder.Code, initialRecorder.Body.String())
	}
	var initialResponse enrollment.EnrollResponse
	if err := json.Unmarshal(initialRecorder.Body.Bytes(), &initialResponse); err != nil {
		t.Fatalf("decode initial enrollment: %v", err)
	}

	attackerPublicKey, attackerPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate attacker identity: %v", err)
	}
	attackerFingerprint := agentidentity.FingerprintFromPublicKey(attackerPublicKey)
	heartbeatPayload, _ := json.Marshal(assets.HeartbeatRequest{
		AssetID: "heartbeat-anchor-node", Type: "host", Name: "Heartbeat anchor node", Source: "agent", Status: "online", Platform: "linux",
		Metadata: map[string]string{
			assets.MetadataKeyAgentDeviceFingerprint:  attackerFingerprint,
			assets.MetadataKeyAgentDeviceKeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
		},
	})
	heartbeatRequest := httptest.NewRequest(http.MethodPost, "https://labtether.test/assets/heartbeat", bytes.NewReader(heartbeatPayload))
	heartbeatRequest.Header.Set("Authorization", "Bearer "+initialResponse.AgentToken)
	heartbeatRecorder := httptest.NewRecorder()
	sut.withAgentHeartbeatAuth(sut.handleAssetActions)(heartbeatRecorder, heartbeatRequest)
	if heartbeatRecorder.Code != http.StatusAccepted {
		t.Fatalf("authenticated heartbeat: expected 202, got %d: %s", heartbeatRecorder.Code, heartbeatRecorder.Body.String())
	}
	anchored, exists, err := sut.assetStore.GetAsset("heartbeat-anchor-node")
	if err != nil || !exists {
		t.Fatalf("load heartbeat anchor: exists=%v err=%v", exists, err)
	}
	if anchored.Metadata[assets.MetadataKeyAgentDeviceFingerprint] != attackerFingerprint || anchored.Metadata[assets.MetadataKeyAgentIdentityVerifiedAt] != "" {
		t.Fatalf("heartbeat identity state=%+v, want matching provisional fingerprint", anchored.Metadata)
	}

	recoveryRawToken, recoveryToken := mustCreateEnrollmentTokenWithMaxUses(t, sut, 1)
	recoveryPayload, _ := json.Marshal(signedContinuityEnrollmentRequest(t, recoveryRawToken, "heartbeat-anchor-node", attackerPublicKey, attackerPrivateKey))
	recoveryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(recoveryPayload))
	recoveryRequest.Host = "localhost:8080"
	recoveryRequest.RemoteAddr = "127.0.0.2:12345"
	recoveryRecorder := httptest.NewRecorder()
	sut.handleEnroll(recoveryRecorder, recoveryRequest)
	if recoveryRecorder.Code != http.StatusConflict {
		t.Fatalf("unverified recovery: expected 409, got %d: %s", recoveryRecorder.Code, recoveryRecorder.Body.String())
	}
	if _, valid, err := sut.enrollmentStore.ValidateAgentToken(auth.HashToken(initialResponse.AgentToken)); err != nil || !valid {
		t.Fatalf("rejected recovery changed original bearer: valid=%v err=%v", valid, err)
	}
	storedRecoveryToken, valid, err := sut.enrollmentStore.ValidateEnrollmentToken(auth.HashToken(recoveryRawToken))
	if err != nil || !valid || storedRecoveryToken.ID != recoveryToken.ID || storedRecoveryToken.UseCount != 0 {
		t.Fatalf("rejected recovery changed enrollment token: token=%+v valid=%v err=%v", storedRecoveryToken, valid, err)
	}
}
