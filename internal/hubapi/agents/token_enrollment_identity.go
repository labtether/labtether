package agents

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/enrollment"
)

const (
	maxTokenEnrollmentKeyAlgorithmLen = 64
	maxTokenEnrollmentPublicKeyLen    = 512
	maxTokenEnrollmentFingerprintLen  = 160
	maxTokenEnrollmentSignatureLen    = 512
	maxTokenEnrollmentProofVersionLen = 16
)

// verifyTokenEnrollmentIdentity verifies an optional, self-contained device
// identity proof on token-based enrollment. The enrollment token authorizes a
// first enrollment; this proof additionally demonstrates continuity with a
// previously recorded device key before an existing asset can be reclaimed.
func verifyTokenEnrollmentIdentity(req enrollment.EnrollRequest, canonicalAssetID string) (string, string, bool, error) {
	keyAlgorithm := strings.ToLower(strings.TrimSpace(req.DeviceKeyAlg))
	publicKeyEncoded := strings.TrimSpace(req.DevicePublicKey)
	fingerprint := strings.TrimSpace(req.DeviceFingerprint)
	signatureEncoded := strings.TrimSpace(req.DeviceSignature)
	proofVersion := strings.ToLower(strings.TrimSpace(req.DeviceProofVersion))

	present := 0
	for _, value := range []string{keyAlgorithm, publicKeyEncoded, fingerprint, signatureEncoded} {
		if value != "" {
			present++
		}
	}
	if present == 0 {
		if proofVersion != "" {
			return "", "", true, fmt.Errorf("proof version provided without device identity")
		}
		return "", "", false, nil
	}
	if present != 4 {
		return "", "", true, fmt.Errorf("incomplete device identity proof")
	}
	if proofVersion == "" {
		proofVersion = enrollment.DeviceProofVersionV1
	}
	if len(keyAlgorithm) > maxTokenEnrollmentKeyAlgorithmLen ||
		len(publicKeyEncoded) > maxTokenEnrollmentPublicKeyLen ||
		len(fingerprint) > maxTokenEnrollmentFingerprintLen ||
		len(signatureEncoded) > maxTokenEnrollmentSignatureLen ||
		len(proofVersion) > maxTokenEnrollmentProofVersionLen {
		return "", "", true, fmt.Errorf("device identity proof exceeds size limits")
	}
	if keyAlgorithm != agentidentity.KeyAlgorithmEd25519 {
		return "", "", true, fmt.Errorf("unsupported device key algorithm")
	}
	if proofVersion != enrollment.DeviceProofVersionV1 && proofVersion != enrollment.DeviceProofVersionV2 {
		return "", "", true, fmt.Errorf("unsupported device proof version")
	}

	publicKeyRaw, err := base64.StdEncoding.DecodeString(publicKeyEncoded)
	if err != nil || len(publicKeyRaw) != ed25519.PublicKeySize {
		return "", "", true, fmt.Errorf("invalid device public key")
	}
	signatureRaw, err := base64.StdEncoding.DecodeString(signatureEncoded)
	if err != nil || len(signatureRaw) != ed25519.SignatureSize {
		return "", "", true, fmt.Errorf("invalid device signature")
	}

	expectedFingerprint := agentidentity.FingerprintFromPublicKey(publicKeyRaw)
	if !strings.EqualFold(fingerprint, expectedFingerprint) {
		return "", "", true, fmt.Errorf("device fingerprint mismatch")
	}
	var payload []byte
	if proofVersion == enrollment.DeviceProofVersionV2 {
		if strings.TrimSpace(canonicalAssetID) == "" {
			return "", "", true, fmt.Errorf("canonical asset id is required for v2 proof")
		}
		payload = agentidentity.BuildTokenEnrollmentProofPayloadV2(canonicalAssetID, req.EnrollmentToken, expectedFingerprint)
	} else {
		payload = agentidentity.BuildTokenEnrollmentProofPayload(req.Hostname, req.EnrollmentToken, expectedFingerprint)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKeyRaw), payload, signatureRaw) {
		return "", "", true, fmt.Errorf("device signature verification failed")
	}

	return expectedFingerprint, proofVersion, true, nil
}
