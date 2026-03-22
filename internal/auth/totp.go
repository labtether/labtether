package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// GenerateTOTPSecret creates a new TOTP key for the given user.
func GenerateTOTPSecret(username, issuer string) (secret string, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTPCode checks a 6-digit TOTP code against the secret.
func ValidateTOTPCode(secret, code string) bool {
	valid, _ := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}

// EncryptTOTPSecret encrypts a TOTP secret using AES-256-GCM.
func EncryptTOTPSecret(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptTOTPSecret decrypts a base64-encoded AES-256-GCM ciphertext.
func DecryptTOTPSecret(encoded string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// GenerateRecoveryCodes produces n random recovery codes in the format "xxxxxxxx-xxxxxxxx"
// (8 hex chars per segment, 64 bits of entropy per code).
func GenerateRecoveryCodes(n int) []string {
	codes := make([]string, n)
	for i := range codes {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		codes[i] = fmt.Sprintf("%s-%s", hex.EncodeToString(b[:4]), hex.EncodeToString(b[4:]))
	}
	return codes
}

// HashRecoveryCode hashes a recovery code with bcrypt (cost 10).
func HashRecoveryCode(code string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(code)), 10)
	return string(hash), err
}

// CheckRecoveryCode compares a plaintext recovery code against a bcrypt hash.
func CheckRecoveryCode(code, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(strings.TrimSpace(code))) == nil
}
