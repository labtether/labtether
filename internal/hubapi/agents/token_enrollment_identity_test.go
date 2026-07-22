package agents

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/enrollment"
)

func TestVerifyTokenEnrollmentIdentityBindsProofToTokenAndHostname(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	request := enrollment.EnrollRequest{
		EnrollmentToken:   "one-time-token-a",
		Hostname:          "continuity-node",
		DeviceKeyAlg:      agentidentity.KeyAlgorithmEd25519,
		DevicePublicKey:   base64.StdEncoding.EncodeToString(publicKey),
		DeviceFingerprint: fingerprint,
	}
	payload := agentidentity.BuildTokenEnrollmentProofPayload(request.Hostname, request.EnrollmentToken, fingerprint)
	request.DeviceSignature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))

	verifiedFingerprint, version, provided, err := verifyTokenEnrollmentIdentity(request, "continuity-node")
	if err != nil || !provided || verifiedFingerprint != fingerprint || version != enrollment.DeviceProofVersionV1 {
		t.Fatalf("valid proof rejected: fingerprint=%q version=%q provided=%v err=%v", verifiedFingerprint, version, provided, err)
	}

	tamperedToken := request
	tamperedToken.EnrollmentToken = "one-time-token-b"
	if _, _, provided, err := verifyTokenEnrollmentIdentity(tamperedToken, "continuity-node"); err == nil || !provided {
		t.Fatalf("proof replayed with another token: provided=%v err=%v", provided, err)
	}

	tamperedHostname := request
	tamperedHostname.Hostname = "other-node"
	if _, _, provided, err := verifyTokenEnrollmentIdentity(tamperedHostname, "other-node"); err == nil || !provided {
		t.Fatalf("proof replayed for another hostname: provided=%v err=%v", provided, err)
	}
}

func TestVerifyTokenEnrollmentIdentityDistinguishesAbsentAndPartialProof(t *testing.T) {
	if fingerprint, version, provided, err := verifyTokenEnrollmentIdentity(enrollment.EnrollRequest{}, "node"); err != nil || provided || fingerprint != "" || version != "" {
		t.Fatalf("absent proof result: fingerprint=%q provided=%v err=%v", fingerprint, provided, err)
	}

	partial := enrollment.EnrollRequest{DeviceKeyAlg: agentidentity.KeyAlgorithmEd25519}
	if _, _, provided, err := verifyTokenEnrollmentIdentity(partial, "node"); err == nil || !provided {
		t.Fatalf("partial proof result: provided=%v err=%v", provided, err)
	}
}

func TestVerifyTokenEnrollmentIdentityV2BindsCanonicalAssetID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	fingerprint := agentidentity.FingerprintFromPublicKey(publicKey)
	request := enrollment.EnrollRequest{
		EnrollmentToken:    "single-use-token",
		Hostname:           "Mixed CASE Node",
		DeviceKeyAlg:       agentidentity.KeyAlgorithmEd25519,
		DevicePublicKey:    base64.StdEncoding.EncodeToString(publicKey),
		DeviceFingerprint:  fingerprint,
		DeviceProofVersion: enrollment.DeviceProofVersionV2,
	}
	payload := agentidentity.BuildTokenEnrollmentProofPayloadV2("mixed-case-node", request.EnrollmentToken, fingerprint)
	request.DeviceSignature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if got, version, provided, err := verifyTokenEnrollmentIdentity(request, "mixed-case-node"); err != nil || !provided || got != fingerprint || version != enrollment.DeviceProofVersionV2 {
		t.Fatalf("v2 proof rejected: fingerprint=%q version=%q provided=%v err=%v", got, version, provided, err)
	}
	if _, _, provided, err := verifyTokenEnrollmentIdentity(request, "another-node"); err == nil || !provided {
		t.Fatalf("v2 proof replayed for another asset: provided=%v err=%v", provided, err)
	}
}
