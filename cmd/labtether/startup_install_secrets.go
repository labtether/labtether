package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/installstate"
	"golang.org/x/crypto/hkdf"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runtimeInstallSecrets struct {
	OwnerToken    string
	APIToken      string // #nosec G117 -- Runtime install secret, not a hardcoded credential.
	EncryptionKey string
}

func resolveRuntimeInstallSecrets(store *installstate.Store) (runtimeInstallSecrets, error) {
	var resolved runtimeInstallSecrets
	if store == nil {
		return resolved, errors.New("install state store is required")
	}

	meta, persisted, exists, err := store.Load()
	if err != nil {
		return resolved, err
	}

	envOwnerToken := strings.TrimSpace(os.Getenv("LABTETHER_OWNER_TOKEN"))
	envAPIToken := strings.TrimSpace(os.Getenv("LABTETHER_API_TOKEN"))
	envEncryptionKey := strings.TrimSpace(os.Getenv("LABTETHER_ENCRYPTION_KEY"))
	envPostgresPassword := strings.TrimSpace(os.Getenv("POSTGRES_PASSWORD"))

	resolved.OwnerToken = strings.TrimSpace(persisted.OwnerToken)
	if isWellKnownPlaceholder(resolved.OwnerToken) {
		log.Printf("labtether: WARNING: persisted owner token matches a well-known dev placeholder — regenerating")
		resolved.OwnerToken = ""
	}
	if envOwnerToken != "" {
		resolved.OwnerToken = envOwnerToken
	}
	if resolved.OwnerToken == "" {
		token, err := generateHexToken(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate owner token: %w", err)
		}
		resolved.OwnerToken = token
	}

	resolved.APIToken = strings.TrimSpace(persisted.APIToken)
	if isWellKnownPlaceholder(resolved.APIToken) {
		log.Printf("labtether: WARNING: persisted API token matches a well-known dev placeholder — regenerating")
		resolved.APIToken = ""
	}
	if envAPIToken != "" {
		resolved.APIToken = envAPIToken
	}
	if resolved.APIToken == "" {
		token, err := generateHexToken(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate api token: %w", err)
		}
		resolved.APIToken = token
	}

	resolved.EncryptionKey = strings.TrimSpace(persisted.EncryptionKey)
	if isWellKnownPlaceholderKey(resolved.EncryptionKey) {
		log.Printf("labtether: WARNING: persisted encryption key matches a well-known dev placeholder — regenerating")
		resolved.EncryptionKey = ""
	}
	if envEncryptionKey != "" {
		resolved.EncryptionKey = envEncryptionKey
	}
	if resolved.EncryptionKey == "" {
		key, err := generateBase64Key(32)
		if err != nil {
			return runtimeInstallSecrets{}, fmt.Errorf("generate encryption key: %w", err)
		}
		resolved.EncryptionKey = key
	}

	if _, err := loadSecretsManager(resolved.EncryptionKey); err != nil {
		return runtimeInstallSecrets{}, fmt.Errorf("validate encryption key: %w", err)
	}

	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	nextSecrets := installstate.Secrets{
		OwnerToken:       resolved.OwnerToken,
		APIToken:         resolved.APIToken,
		EncryptionKey:    resolved.EncryptionKey,
		PostgresPassword: strings.TrimSpace(persisted.PostgresPassword),
	}
	if envPostgresPassword != "" {
		nextSecrets.PostgresPassword = envPostgresPassword
	}
	if !exists || persisted != nextSecrets || meta.SchemaVersion != 1 {
		if err := store.Save(meta, nextSecrets); err != nil {
			return runtimeInstallSecrets{}, err
		}
		if exists {
			log.Printf("labtether: install state secrets refreshed in %s", store.Root())
		} else {
			log.Printf("labtether: install state initialized in %s", store.Root())
		}
	}
	if err := writeRuntimeAPITokenFile(resolved.APIToken); err != nil {
		return runtimeInstallSecrets{}, err
	}

	return resolved, nil
}

func writeRuntimeAPITokenFile(token string) error {
	path := strings.TrimSpace(os.Getenv("LABTETHER_API_TOKEN_FILE"))
	if path == "" {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("runtime api token is empty")
	}
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- Directory is derived from the fixed runtime token file path.
		return fmt.Errorf("create runtime api token directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect runtime api token directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime api token directory must be a real directory")
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open runtime api token directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return errors.New("runtime api token file path is invalid")
	}
	if info, statErr := root.Lstat(name); statErr == nil {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return errors.New("runtime api token destination must be a regular file or symlink")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect runtime api token destination: %w", statErr)
	}

	randomSuffix := make([]byte, 16)
	if _, err := rand.Read(randomSuffix); err != nil {
		return fmt.Errorf("generate runtime api token temp name: %w", err)
	}
	tmpName := "." + name + "." + hex.EncodeToString(randomSuffix) + ".tmp"
	tmp, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create runtime api token temp file: %w", err)
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = root.Remove(tmpName)
		}
	}()
	if _, err := io.WriteString(tmp, token); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write runtime api token temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync runtime api token temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close runtime api token temp file: %w", err)
	}
	if err := root.Rename(tmpName, name); err != nil {
		return fmt.Errorf("install runtime api token file: %w", err)
	}
	removeTemp = false
	return nil
}

func generateHexToken(numBytes int) (string, error) {
	raw := make([]byte, numBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func generateBase64Key(numBytes int) (string, error) {
	raw := make([]byte, numBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// deriveTOTPKey returns a stable 32-byte AES-256 key for encrypting TOTP secrets.
//
// Key derivation priority:
//  1. LABTETHER_TOTP_KEY env var — base64-decoded directly (must be 32 bytes).
//  2. LABTETHER_ENCRYPTION_KEY env var — derive via HKDF-SHA256 with info "labtether-totp-v1".
//  3. Random ephemeral key — logs a warning because 2FA secrets will not survive restart.
func deriveTOTPKey(runtimeEncryptionKey string) ([]byte, error) {
	// Option 1: explicit TOTP key override.
	if raw := strings.TrimSpace(os.Getenv("LABTETHER_TOTP_KEY")); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("LABTETHER_TOTP_KEY is not valid base64: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("LABTETHER_TOTP_KEY must decode to exactly 32 bytes (got %d)", len(key))
		}
		return key, nil
	}

	// Option 2: derive from the main encryption key via HKDF.
	if raw := strings.TrimSpace(runtimeEncryptionKey); raw != "" {
		master, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("runtime encryption key is not valid base64: %w", err)
		}
		r := hkdf.New(sha256.New, master, nil, []byte("labtether-totp-v1"))
		key := make([]byte, 32)
		if _, err := io.ReadFull(r, key); err != nil {
			return nil, fmt.Errorf("hkdf derive TOTP key: %w", err)
		}
		return key, nil
	}

	// Option 3: ephemeral random key — 2FA will break on restart.
	log.Printf("labtether WARNING: TOTP encryption key is ephemeral; 2FA secrets will not survive restart. Set LABTETHER_ENCRYPTION_KEY or LABTETHER_TOTP_KEY for persistence.")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("rand.Read TOTP key: %w", err)
	}
	return key, nil
}

// wellKnownPlaceholderTokens are dev/example tokens that must never be used in
// real deployments. If persisted state contains one of these, the bootstrap
// regenerates a cryptographically random replacement.
var wellKnownPlaceholderTokens = map[string]bool{
	"labtether-owner-local-token": true,
}

// wellKnownPlaceholderKeys are dev/example encryption keys (base64) that must
// never be used in real deployments.
var wellKnownPlaceholderKeys = map[string]bool{
	"MDEyMzQ1Njc4OUFCQ0RFRjAxMjM0NTY3ODlBQkNERUY=": true,
}

func isWellKnownPlaceholder(token string) bool {
	return token != "" && wellKnownPlaceholderTokens[token]
}

func isWellKnownPlaceholderKey(key string) bool {
	return key != "" && wellKnownPlaceholderKeys[key]
}
