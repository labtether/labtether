package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	keySecretBytes = 32
	prefixLength   = 4
)

var base32NoPad = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func GenerateKey() (GeneratedKey, error) {
	prefixBuf := make([]byte, 3)
	if _, err := rand.Read(prefixBuf); err != nil {
		return GeneratedKey{}, fmt.Errorf("generate prefix: %w", err)
	}
	prefix := base32NoPad.EncodeToString(prefixBuf)[:prefixLength]

	secretBuf := make([]byte, keySecretBytes)
	if _, err := rand.Read(secretBuf); err != nil {
		return GeneratedKey{}, fmt.Errorf("generate secret: %w", err)
	}
	secret := base32NoPad.EncodeToString(secretBuf)

	raw := fmt.Sprintf("lt_%s_%s", prefix, secret)
	return GeneratedKey{
		Raw:    raw,
		Prefix: prefix,
		Hash:   HashKey(raw),
	}, nil
}

func HashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func ExtractPrefix(raw string) (string, bool) {
	if !strings.HasPrefix(raw, "lt_") {
		return "", false
	}
	rest := raw[3:]
	idx := strings.Index(rest, "_")
	if idx < 1 {
		return "", false
	}
	return rest[:idx], true
}

func IsAPIKeyFormat(s string) bool {
	_, ok := ExtractPrefix(s)
	return ok
}
