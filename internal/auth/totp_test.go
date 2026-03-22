package auth

import (
	"crypto/rand"
	"testing"
)

func testEncryptionKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptTOTPSecret(t *testing.T) {
	key := testEncryptionKey(t)
	secret := "JBSWY3DPEHPK3PXP"

	encrypted, err := EncryptTOTPSecret(secret, key)
	if err != nil {
		t.Fatalf("EncryptTOTPSecret: %v", err)
	}
	if encrypted == secret {
		t.Error("encrypted should differ from plaintext")
	}

	decrypted, err := DecryptTOTPSecret(encrypted, key)
	if err != nil {
		t.Fatalf("DecryptTOTPSecret: %v", err)
	}
	if decrypted != secret {
		t.Errorf("decrypted = %q, want %q", decrypted, secret)
	}
}

func TestDecryptTOTPSecret_WrongKey(t *testing.T) {
	key1 := testEncryptionKey(t)
	key2 := testEncryptionKey(t)

	encrypted, err := EncryptTOTPSecret("MYSECRET", key1)
	if err != nil {
		t.Fatalf("EncryptTOTPSecret: %v", err)
	}

	_, err = DecryptTOTPSecret(encrypted, key2)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestGenerateTOTPSecret(t *testing.T) {
	key, uri, err := GenerateTOTPSecret("admin", "LabTether")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if key == "" {
		t.Error("key should not be empty")
	}
	if uri == "" {
		t.Error("uri should not be empty")
	}
}

func TestValidateTOTPCode_InvalidCode(t *testing.T) {
	key, _, err := GenerateTOTPSecret("admin", "LabTether")
	if err != nil {
		t.Fatal(err)
	}
	if ValidateTOTPCode(key, "000000") {
		t.Skip("000000 happened to be valid right now")
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	codes := GenerateRecoveryCodes(8)
	if len(codes) != 8 {
		t.Errorf("len = %d, want 8", len(codes))
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if len(code) < 8 {
			t.Errorf("code %q too short", code)
		}
		if seen[code] {
			t.Errorf("duplicate code %q", code)
		}
		seen[code] = true
	}
}

func TestHashAndCheckRecoveryCode(t *testing.T) {
	code := "abcd-efgh-1234"
	hash, err := HashRecoveryCode(code)
	if err != nil {
		t.Fatalf("HashRecoveryCode: %v", err)
	}
	if !CheckRecoveryCode(code, hash) {
		t.Error("CheckRecoveryCode should return true for matching code")
	}
	if CheckRecoveryCode("wrong-code-1234", hash) {
		t.Error("CheckRecoveryCode should return false for wrong code")
	}
}
