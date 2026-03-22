package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	encodedPrefixV2 = "v2:"
	keyLenBytes     = 32
)

type Manager struct {
	aead cipher.AEAD
}

func NewManagerFromEncodedKey(encoded string) (*Manager, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, errors.New("encryption key is required")
	}

	key, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encryption key: %w", err)
	}
	if len(key) != keyLenBytes {
		return nil, fmt.Errorf("encryption key must decode to %d bytes", keyLenBytes)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Manager{aead: aead}, nil
}

// EncryptString encrypts plain with AES-256-GCM. The aad parameter binds the
// ciphertext to a specific context (e.g. the credential profile ID) so that
// the ciphertext cannot be transplanted to a different row. Produces v2: format.
func (m *Manager) EncryptString(plain string, aad string) (string, error) {
	if m == nil || m.aead == nil {
		return "", errors.New("secrets manager is not configured")
	}
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := m.aead.Seal(nil, nonce, []byte(plain), []byte(aad))
	payload := append(nonce, ciphertext...)
	return encodedPrefixV2 + base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptString decrypts a ciphertext string. The aad parameter must match the
// value used during encryption.
func (m *Manager) DecryptString(encoded string, aad string) (string, error) {
	if m == nil || m.aead == nil {
		return "", errors.New("secrets manager is not configured")
	}
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return "", nil
	}

	var payloadB64 string
	switch {
	case strings.HasPrefix(trimmed, encodedPrefixV2):
		payloadB64 = strings.TrimPrefix(trimmed, encodedPrefixV2)
	default:
		return "", errors.New("unsupported ciphertext format")
	}

	payload, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", err
	}
	if len(payload) < m.aead.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}
	nonce := payload[:m.aead.NonceSize()]
	ciphertext := payload[m.aead.NonceSize():]
	plain, err := m.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
