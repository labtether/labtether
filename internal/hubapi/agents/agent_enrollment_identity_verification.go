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
	agent.ChallengeNonce = nonce
	agent.ChallengeExpiresAt = expiresAt

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

	connectionID := strings.TrimSpace(proof.ConnectionID)
	nonce := strings.TrimSpace(proof.Nonce)
	if connectionID == "" || nonce == "" {
		return fmt.Errorf("missing connection_id or nonce")
	}
	if connectionID != agent.AssetID {
		return fmt.Errorf("connection_id mismatch")
	}
	if agent.ChallengeNonce == "" || nonce != agent.ChallengeNonce {
		return fmt.Errorf("nonce mismatch")
	}
	if !agent.ChallengeExpiresAt.IsZero() && time.Now().UTC().After(agent.ChallengeExpiresAt) {
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
	d.PendingAgents.mu.Lock()
	agent.DeviceFingerprint = expectedFingerprint
	agent.DeviceKeyAlg = agentidentity.KeyAlgorithmEd25519
	agent.DevicePublicKey = strings.TrimSpace(proof.PublicKey)
	agent.IdentityVerified = true
	agent.IdentityVerifiedAt = &verifiedAt
	agent.ChallengeNonce = ""
	agent.ChallengeExpiresAt = time.Time{}
	d.PendingAgents.mu.Unlock()
	return nil
}
