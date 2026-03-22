package agentcore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labtether/labtether/internal/agentidentity"
)

type deviceIdentity struct {
	KeyAlgorithm string
	PublicKey    ed25519.PublicKey
	PrivateKey   ed25519.PrivateKey

	PublicKeyBase64 string
	Fingerprint     string
}

func ensureDeviceIdentity(cfg RuntimeConfig) (*deviceIdentity, error) {
	identity, err := loadDeviceIdentity(cfg)
	if err == nil {
		_ = persistDeviceIdentityArtifacts(cfg, identity)
		return identity, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	publicKey, privateKey, genErr := ed25519.GenerateKey(rand.Reader)
	if genErr != nil {
		return nil, fmt.Errorf("generate device key: %w", genErr)
	}

	identity = &deviceIdentity{
		KeyAlgorithm:    agentidentity.KeyAlgorithmEd25519,
		PublicKey:       publicKey,
		PrivateKey:      privateKey,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicKey),
		Fingerprint:     agentidentity.FingerprintFromPublicKey(publicKey),
	}

	if persistErr := persistDeviceIdentity(cfg, identity); persistErr != nil {
		return nil, persistErr
	}
	return identity, nil
}

func loadDeviceIdentity(cfg RuntimeConfig) (*deviceIdentity, error) {
	privateKeyPath := resolveDeviceKeyPath(cfg.DeviceKeyPath, defaultDeviceKeyFile)
	raw, err := os.ReadFile(privateKeyPath) // #nosec G304 -- Device identity key path is runtime configuration/default state, not user input.
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode device key %s: %w", privateKeyPath, err)
	}

	privateKey, err := normalizePrivateKey(decoded)
	if err != nil {
		return nil, fmt.Errorf("parse device key %s: %w", privateKeyPath, err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)

	return &deviceIdentity{
		KeyAlgorithm:    agentidentity.KeyAlgorithmEd25519,
		PublicKey:       publicKey,
		PrivateKey:      privateKey,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicKey),
		Fingerprint:     agentidentity.FingerprintFromPublicKey(publicKey),
	}, nil
}

func normalizePrivateKey(raw []byte) (ed25519.PrivateKey, error) {
	switch len(raw) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	default:
		return nil, fmt.Errorf("unexpected private key size %d", len(raw))
	}
}

func persistDeviceIdentity(cfg RuntimeConfig, identity *deviceIdentity) error {
	privateKeyPath := resolveDeviceKeyPath(cfg.DeviceKeyPath, defaultDeviceKeyFile)
	publicKeyPath := resolveDeviceKeyPath(cfg.DevicePublicKeyPath, defaultDevicePublicKeyFile)
	fingerprintPath := resolveDeviceKeyPath(cfg.DeviceFingerprintPath, defaultDeviceFingerprintFile)

	if err := writePrivateKeyFile(privateKeyPath, identity.PrivateKey); err != nil {
		return err
	}
	if err := writeTextFile(publicKeyPath, identity.PublicKeyBase64+"\n", 0644); err != nil {
		return err
	}
	if err := writeTextFile(fingerprintPath, identity.Fingerprint+"\n", 0644); err != nil {
		return err
	}
	return nil
}

func persistDeviceIdentityArtifacts(cfg RuntimeConfig, identity *deviceIdentity) error {
	publicKeyPath := resolveDeviceKeyPath(cfg.DevicePublicKeyPath, defaultDevicePublicKeyFile)
	fingerprintPath := resolveDeviceKeyPath(cfg.DeviceFingerprintPath, defaultDeviceFingerprintFile)

	if err := writeTextFile(publicKeyPath, identity.PublicKeyBase64+"\n", 0644); err != nil {
		return err
	}
	if err := writeTextFile(fingerprintPath, identity.Fingerprint+"\n", 0644); err != nil {
		return err
	}
	return nil
}

func writePrivateKeyFile(path string, key ed25519.PrivateKey) error {
	encoded := base64.StdEncoding.EncodeToString(key)
	return writeTextFile(path, encoded+"\n", 0600)
}

func writeTextFile(path, value string, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func resolveDeviceKeyPath(raw, fallback string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
