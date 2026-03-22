package agentidentity

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

const KeyAlgorithmEd25519 = "ed25519"

// FingerprintFromPublicKey returns a stable, human-friendly fingerprint for a
// device public key.
func FingerprintFromPublicKey(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return "LT-" + groupFingerprint(encoded, 4)
}

// BuildEnrollmentProofPayload builds the canonical bytes an agent signs to
// prove possession of its device private key during pending enrollment.
func BuildEnrollmentProofPayload(connectionID, nonce, fingerprint string) []byte {
	return []byte(
		"labtether-enrollment-proof|" +
			strings.TrimSpace(connectionID) + "|" +
			strings.TrimSpace(nonce) + "|" +
			strings.TrimSpace(fingerprint),
	)
}

func groupFingerprint(raw string, groupSize int) string {
	if groupSize <= 0 {
		return raw
	}
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	for i, r := range raw {
		if i > 0 && i%groupSize == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}
