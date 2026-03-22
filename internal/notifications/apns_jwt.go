package notifications

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

// signAPNsJWT generates an ES256-signed JWT for Apple Push Notification Service
// token-based authentication. This uses only Go's standard library.
//
// The JWT format follows Apple's requirements:
//   - Header:  {"alg":"ES256","kid":"<key_id>"}
//   - Payload: {"iss":"<team_id>","iat":<unix_timestamp>}
//   - Signature: ECDSA P-256 with SHA-256
func signAPNsJWT(key *ecdsa.PrivateKey, keyID, teamID string, now time.Time) (string, error) {
	if key == nil {
		return "", fmt.Errorf("apns jwt: private key is nil")
	}

	header, err := json.Marshal(map[string]string{
		"alg": "ES256",
		"kid": keyID,
	})
	if err != nil {
		return "", fmt.Errorf("apns jwt: marshal header: %w", err)
	}

	claims, err := json.Marshal(map[string]any{
		"iss": teamID,
		"iat": now.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("apns jwt: marshal claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(header)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claims)
	signingInput := headerB64 + "." + claimsB64

	// Sign with ECDSA P-256 / SHA-256.
	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", fmt.Errorf("apns jwt: sign: %w", err)
	}

	// Encode r and s as fixed-size 32-byte big-endian values (per RFC 7518 / JWS).
	curveBits := key.Curve.Params().BitSize
	keyBytes := curveBits / 8
	if curveBits%8 > 0 {
		keyBytes++
	}

	rBytes := padLeft(r.Bytes(), keyBytes)
	sBytes := padLeft(s.Bytes(), keyBytes)

	sig := append(rBytes, sBytes...)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64, nil
}

// padLeft pads b with leading zeros to ensure it is exactly size bytes.
func padLeft(b []byte, size int) []byte {
	if len(b) >= size {
		return b[:size]
	}
	padded := make([]byte, size)
	copy(padded[size-len(b):], b)
	return padded
}

// verifyAPNsJWT verifies an ES256 JWT signature. Used only in tests.
func verifyAPNsJWT(token string, pub *ecdsa.PublicKey) bool {
	parts := splitJWT(token)
	if parts == nil {
		return false
	}

	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sigBytes) < 64 {
		return false
	}

	hash := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	return ecdsa.Verify(pub, hash[:], r, s)
}

// splitJWT splits a compact JWT into its three parts.
func splitJWT(token string) []string {
	var parts []string
	start := 0
	count := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
			count++
		}
	}
	if count == 2 {
		parts = append(parts, token[start:])
		return parts
	}
	return nil
}
