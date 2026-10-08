package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlePendingEnrollmentAcceptsLargeValidProof(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handlePendingEnrollment(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("X-Hostname", "pending-node")
	headers.Set("X-Platform", "linux")

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read challenge message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("expected enrollment.challenge, got %q", msg.Type)
	}

	var challenge agentmgr.EnrollmentChallengeData
	if err := json.Unmarshal(msg.Data, &challenge); err != nil {
		t.Fatalf("decode challenge payload: %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	signature := ed25519.Sign(privateKey, agentidentity.BuildEnrollmentProofPayload(challenge.ConnectionID, challenge.Nonce, fingerprint))

	proofEnvelope := map[string]any{
		"connection_id": challenge.ConnectionID,
		"nonce":         challenge.Nonce,
		"key_algorithm": agentidentity.KeyAlgorithmEd25519,
		"public_key":    base64.StdEncoding.EncodeToString(publicKey),
		"fingerprint":   fingerprint,
		"signature":     base64.StdEncoding.EncodeToString(signature),
		"padding":       strings.Repeat("x", 1500),
	}
	rawProof, err := json.Marshal(proofEnvelope)
	if err != nil {
		t.Fatalf("marshal proof envelope: %v", err)
	}
	rawMsg, err := json.Marshal(agentmgr.Message{
		Type: agentmgr.MsgEnrollmentProof,
		Data: rawProof,
	})
	if err != nil {
		t.Fatalf("marshal websocket message: %v", err)
	}
	if len(rawMsg) <= 1024 {
		t.Fatalf("expected proof message to exceed previous 1024-byte limit, got %d", len(rawMsg))
	}

	if err := clientConn.WriteMessage(websocket.TextMessage, rawMsg); err != nil {
		t.Fatalf("write large proof message: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.pendingAgents.IsIdentityVerified(challenge.ConnectionID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, ok := sut.pendingAgents.Get(challenge.ConnectionID)
	if !ok {
		t.Fatalf("expected pending agent %q to remain connected", challenge.ConnectionID)
	}
	t.Fatalf("expected pending agent %q to verify with large proof payload", challenge.ConnectionID)
}

func TestHandlePendingEnrollmentClosesAfterInvalidProofBudget(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()
	server := httptest.NewServer(http.HandlerFunc(sut.handlePendingEnrollment))
	defer server.Close()

	headers := http.Header{}
	headers.Set("X-Hostname", "proof-budget-node")
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var challenge agentmgr.Message
	if err := client.ReadJSON(&challenge); err != nil || challenge.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("challenge=%+v err=%v", challenge, err)
	}
	for i := 0; i < maxPendingEnrollmentProofMessages; i++ {
		if err := client.WriteJSON(agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: json.RawMessage(`{}`)}); err != nil {
			t.Fatalf("invalid proof %d: %v", i+1, err)
		}
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("pending socket remained open after proof budget exhaustion")
	}
	deadline := time.Now().Add(2 * time.Second)
	for sut.pendingAgents.Count() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sut.pendingAgents.Count() != 0 {
		t.Fatal("proof-budget socket remained in pending capacity registry")
	}
}

func TestHandlePendingEnrollmentRejectsUnexpectedMessageType(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()
	server := httptest.NewServer(http.HandlerFunc(sut.handlePendingEnrollment))
	defer server.Close()
	headers := http.Header{}
	headers.Set("X-Hostname", "unexpected-message-node")
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var challenge agentmgr.Message
	if err := client.ReadJSON(&challenge); err != nil {
		t.Fatal(err)
	}
	if err := client.WriteJSON(agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("unexpected pending message did not close socket")
	}
}

func TestHandlePendingEnrollmentTimesOutAndCleansUp(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.pendingAgents = newPendingAgents()

	type scheduledTimeout struct {
		duration time.Duration
		fn       func()
	}
	timeoutFnCh := make(chan scheduledTimeout, 1)
	prevAfterFunc := agentspkg.PendingEnrollmentAfterFunc
	agentspkg.PendingEnrollmentAfterFunc = func(d time.Duration, fn func()) *time.Timer {
		timeoutFnCh <- scheduledTimeout{duration: d, fn: fn}
		return time.NewTimer(time.Hour)
	}
	defer func() {
		agentspkg.PendingEnrollmentAfterFunc = prevAfterFunc
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sut.handlePendingEnrollment(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("X-Hostname", "timed-out-node")
	headers.Set("X-Platform", "linux")

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer clientConn.Close()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read challenge message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentChallenge {
		t.Fatalf("expected enrollment.challenge, got %q", msg.Type)
	}

	if sut.pendingAgents.Count() != 1 {
		t.Fatalf("expected one pending agent after connect, got %d", sut.pendingAgents.Count())
	}

	var scheduled scheduledTimeout
	select {
	case scheduled = <-timeoutFnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pending enrollment timeout callback")
	}
	if scheduled.duration != maxPendingEnrollmentTimeout {
		t.Fatalf("expected timeout duration %s, got %s", maxPendingEnrollmentTimeout, scheduled.duration)
	}
	scheduled.fn()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sut.pendingAgents.Count() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected pending agent to be removed after timeout")
}

func TestVerifyPendingEnrollmentProof(t *testing.T) {
	t.Run("accepts valid signed proof", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)

		agent := &pendingAgent{
			AssetID:            "pending-alpha-1",
			Hostname:           "alpha",
			ChallengeNonce:     "nonce-123",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err != nil {
			t.Fatalf("expected proof verification success, got %v", err)
		}
		if !agent.IdentityVerified {
			t.Fatalf("expected identity_verified=true")
		}
		if agent.DeviceFingerprint != fingerprint {
			t.Fatalf("expected fingerprint %q, got %q", fingerprint, agent.DeviceFingerprint)
		}
		if agent.IdentityVerifiedAt == nil {
			t.Fatalf("expected identity_verified_at to be set")
		}
	})

	t.Run("rejects invalid signature", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-bravo-1",
			Hostname:           "bravo",
			ChallengeNonce:     "nonce-456",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		invalidSignature := make([]byte, ed25519.SignatureSize)
		if _, err := rand.Read(invalidSignature); err != nil {
			t.Fatalf("rand signature: %v", err)
		}
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(invalidSignature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected invalid signature rejection")
		}
		if agent.IdentityVerified {
			t.Fatalf("expected identity_verified=false")
		}
	})

	t.Run("rejects connection_id mismatch", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-charlie-1",
			ChallengeNonce:     "nonce-789",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload("pending-other-1", agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: "pending-other-1",
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected connection_id mismatch rejection")
		}
	})

	t.Run("rejects expired challenge", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-delta-1",
			ChallengeNonce:     "nonce-expired",
			ChallengeExpiresAt: time.Now().UTC().Add(-time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  fingerprint,
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected expired challenge rejection")
		}
	})

	t.Run("rejects fingerprint mismatch", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.pendingAgents = newPendingAgents()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
		agent := &pendingAgent{
			AssetID:            "pending-echo-1",
			ChallengeNonce:     "nonce-fingerprint",
			ChallengeExpiresAt: time.Now().UTC().Add(time.Minute),
		}
		sut.pendingAgents.Add(agent)

		payload := agentidentity.BuildEnrollmentProofPayload(agent.AssetID, agent.ChallengeNonce, fingerprint)
		signature := ed25519.Sign(privateKey, payload)
		proof := agentmgr.EnrollmentProofData{
			ConnectionID: agent.AssetID,
			Nonce:        agent.ChallengeNonce,
			KeyAlgorithm: agentidentity.KeyAlgorithmEd25519,
			PublicKey:    base64.StdEncoding.EncodeToString(publicKey),
			Fingerprint:  "sha256:wrong",
			Signature:    base64.StdEncoding.EncodeToString(signature),
		}
		raw, _ := json.Marshal(proof)

		if err := sut.verifyPendingEnrollmentProof(agent, agentmgr.Message{Type: agentmgr.MsgEnrollmentProof, Data: raw}); err == nil {
			t.Fatalf("expected fingerprint mismatch rejection")
		}
	})
}
