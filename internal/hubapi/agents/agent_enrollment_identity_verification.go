package agents

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
)

func (d *Deps) SendPendingEnrollmentChallenge(agent *PendingAgent) error {
	if agent == nil || agent.Conn == nil {
		return fmt.Errorf("pending agent connection not available")
	}

	nonce, _, err := auth.GenerateSessionToken()
	if err != nil {
		return fmt.Errorf("generate challenge nonce: %w", err)
	}
	expiresAt := time.Now().UTC().Add(pendingChallengeTTL)
	if !d.PendingAgents.SetChallenge(agent.AssetID, agent.Conn, nonce, expiresAt) {
		return fmt.Errorf("pending agent is no longer available for challenge")
	}

	payload, err := json.Marshal(agentmgr.EnrollmentChallengeData{
		ConnectionID: agent.AssetID,
		Nonce:        nonce,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal challenge: %w", err)
	}

	agent.ConnMu.Lock()
	defer agent.ConnMu.Unlock()

	_ = agent.Conn.SetWriteDeadline(time.Now().Add(agentmgr.AgentWriteDeadline))
	if err := agent.Conn.WriteJSON(agentmgr.Message{
		Type: agentmgr.MsgEnrollmentChallenge,
		Data: payload,
	}); err != nil {
		return fmt.Errorf("write challenge: %w", err)
	}
	return nil
}

func (d *Deps) VerifyPendingEnrollmentProof(agent *PendingAgent, msg agentmgr.Message) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}

	var proof agentmgr.EnrollmentProofData
	if err := json.Unmarshal(msg.Data, &proof); err != nil {
		return fmt.Errorf("decode proof: %w", err)
	}

	snapshot, ok := d.PendingAgents.ChallengeSnapshot(agent.AssetID, agent.Conn)
	if !ok {
		return fmt.Errorf("pending challenge is no longer active")
	}
	connectionID := strings.TrimSpace(proof.ConnectionID)
	nonce := strings.TrimSpace(proof.Nonce)
	if connectionID == "" || nonce == "" {
		return fmt.Errorf("missing connection_id or nonce")
	}
	if connectionID != snapshot.AssetID {
		return fmt.Errorf("connection_id mismatch")
	}
	if snapshot.Nonce == "" || nonce != snapshot.Nonce {
		return fmt.Errorf("nonce mismatch")
	}
	if !snapshot.Expires.IsZero() && time.Now().UTC().After(snapshot.Expires) {
		return fmt.Errorf("challenge expired")
	}

	keyAlg := strings.ToLower(strings.TrimSpace(proof.KeyAlgorithm))
	if keyAlg != agentidentity.KeyAlgorithmEd25519 {
		return fmt.Errorf("unsupported key algorithm %q", proof.KeyAlgorithm)
	}

	publicKeyRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(proof.PublicKey))
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	if len(publicKeyRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size %d", len(publicKeyRaw))
	}

	signatureRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(proof.Signature))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(signatureRaw) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size %d", len(signatureRaw))
	}

	expectedFingerprint := agentidentity.FingerprintFromPublicKey(publicKeyRaw)
	proofFingerprint := strings.TrimSpace(proof.Fingerprint)
	if proofFingerprint != "" && !strings.EqualFold(proofFingerprint, expectedFingerprint) {
		return fmt.Errorf("fingerprint mismatch")
	}

	signingPayload := agentidentity.BuildEnrollmentProofPayload(connectionID, nonce, expectedFingerprint)
	if !ed25519.Verify(ed25519.PublicKey(publicKeyRaw), signingPayload, signatureRaw) {
		return fmt.Errorf("signature verification failed")
	}

	verifiedAt := time.Now().UTC()
	if _, ok := d.PendingAgents.CommitIdentityProof(
		agent.AssetID,
		agent.Conn,
		snapshot.Nonce,
		expectedFingerprint,
		agentidentity.KeyAlgorithmEd25519,
		strings.TrimSpace(proof.PublicKey),
		verifiedAt,
	); !ok {
		return fmt.Errorf("pending challenge changed before proof commit")
	}
	return nil
}
