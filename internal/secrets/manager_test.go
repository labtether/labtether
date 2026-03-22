package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	manager, err := NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return manager
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	manager := newTestManager(t)

	encoded, err := manager.EncryptString("super-secret", "profile-123")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decoded, err := manager.DecryptString(encoded, "profile-123")
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decoded != "super-secret" {
		t.Fatalf("expected secret round-trip, got %q", decoded)
	}
}

func TestDecryptRejectsWrongAAD(t *testing.T) {
	manager := newTestManager(t)

	encoded, err := manager.EncryptString("super-secret", "profile-123")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Decrypt with wrong AAD must fail (prevents ciphertext transplant).
	_, err = manager.DecryptString(encoded, "profile-456")
	if err == nil {
		t.Fatal("expected decrypt to fail with wrong AAD, but it succeeded")
	}
}

func TestNewManager_KeyLengthInvariant(t *testing.T) {
	// Invariant: key must decode to exactly 32 bytes (AES-256).
	for _, size := range []int{0, 1, 16, 31, 33, 64} {
		key := make([]byte, size)
		_, err := NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString(key))
		if err == nil {
			t.Errorf("expected error for %d-byte key, got nil", size)
		}
	}
	// Exactly 32 bytes must succeed.
	key := make([]byte, 32)
	_, err := NewManagerFromEncodedKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Errorf("unexpected error for 32-byte key: %v", err)
	}
}

func TestNewManager_RejectsEmptyKey(t *testing.T) {
	_, err := NewManagerFromEncodedKey("")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestNewManager_RejectsInvalidBase64(t *testing.T) {
	_, err := NewManagerFromEncodedKey("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestEncrypt_V2FormatInvariant(t *testing.T) {
	mgr := newTestManager(t)
	encrypted, err := mgr.EncryptString("test", "aad")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encrypted, "v2:") {
		t.Errorf("encrypted should start with v2:, got %q", encrypted[:10])
	}
}

func TestDecrypt_RejectsUnknownFormat(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.DecryptString("v3:invaliddata", "aad")
	if err == nil {
		t.Error("expected error for unknown format prefix")
	}
}

func TestEncrypt_UniqueNonces(t *testing.T) {
	mgr := newTestManager(t)
	// Same plaintext encrypted twice must produce different ciphertexts.
	a, _ := mgr.EncryptString("same", "aad")
	b, _ := mgr.EncryptString("same", "aad")
	if a == b {
		t.Error("two encryptions produced identical ciphertexts (nonce reuse)")
	}
}

func TestDecrypt_EmptyStringReturnsEmpty(t *testing.T) {
	mgr := newTestManager(t)
	result, err := mgr.DecryptString("", "aad")
	if err != nil {
		t.Errorf("unexpected error for empty string: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
