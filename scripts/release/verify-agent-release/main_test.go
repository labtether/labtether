package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyReleaseAcceptsAgentSignerContract(t *testing.T) {
	cfg, expected := writeValidFixture(t)

	verified, err := verifyRelease(cfg)
	if err != nil {
		t.Fatalf("verifyRelease() error = %v", err)
	}
	if verified != expected {
		t.Fatalf("verifyRelease() = %#v, want %#v", verified, expected)
	}
}

func TestVerifyReleaseFailsClosedOnMismatchedBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, cfg *verifyConfig)
	}{
		{
			name: "binary",
			mutate: func(t *testing.T, cfg *verifyConfig) {
				t.Helper()
				mustWrite(t, cfg.BinaryFile, []byte("tampered binary\n"), 0o600)
			},
		},
		{
			name: "binary symlink",
			mutate: func(t *testing.T, cfg *verifyConfig) {
				t.Helper()
				target := cfg.BinaryFile + ".target"
				if err := os.Rename(cfg.BinaryFile, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, cfg.BinaryFile); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checksum",
			mutate: func(t *testing.T, cfg *verifyConfig) {
				t.Helper()
				mustWrite(t, cfg.ChecksumFile, []byte(strings.Repeat("0", 64)+"  "+filepath.Base(cfg.BinaryFile)+"\n"), 0o600)
			},
		},
		{
			name: "detached signature",
			mutate: func(t *testing.T, cfg *verifyConfig) {
				t.Helper()
				mustWrite(t, cfg.SignatureFile, []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))+"\n"), 0o600)
			},
		},
		{
			name: "exact version",
			mutate: func(_ *testing.T, cfg *verifyConfig) {
				cfg.Version = "v9.9.9"
			},
		},
		{
			name: "API digest",
			mutate: func(_ *testing.T, cfg *verifyConfig) {
				cfg.APIDigest = "sha256:" + strings.Repeat("f", 64)
			},
		},
		{
			name: "API size",
			mutate: func(_ *testing.T, cfg *verifyConfig) {
				cfg.APISize++
			},
		},
		{
			name: "checksum filename",
			mutate: func(t *testing.T, cfg *verifyConfig) {
				t.Helper()
				digest, _, err := hashRegularFile(cfg.BinaryFile, maxAgentBinaryBytes)
				if err != nil {
					t.Fatal(err)
				}
				mustWrite(t, cfg.ChecksumFile, []byte(digest+"  other-binary\n"), 0o600)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, _ := writeValidFixture(t)
			test.mutate(t, &cfg)
			if _, err := verifyRelease(cfg); err == nil {
				t.Fatal("verifyRelease() succeeded for a mismatched release binding")
			}
		})
	}
}

func TestVerifyReleaseRejectsUnknownMetadataFields(t *testing.T) {
	cfg, metadata := writeValidFixture(t)
	payload, err := json.Marshal(map[string]any{
		"version":      metadata.Version,
		"os":           metadata.OS,
		"arch":         metadata.Arch,
		"sha256":       metadata.SHA256,
		"size_bytes":   metadata.SizeBytes,
		"signature":    metadata.Signature,
		"download_url": "https://attacker.invalid/agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, cfg.MetadataFile, payload, 0o600)
	if _, err := verifyRelease(cfg); err == nil {
		t.Fatal("verifyRelease() accepted an unknown signed metadata field")
	}
}

func TestCanonicalPayloadMatchesAgentSignerContract(t *testing.T) {
	metadata := signedMetadata{
		Version:   "v1.4.0",
		OS:        "linux",
		Arch:      "arm64",
		SHA256:    strings.Repeat("a", 64),
		SizeBytes: 12345,
	}
	want := "v1.4.0\nlinux\narm64\n" + strings.Repeat("a", 64) + "\n12345"
	if got := canonicalPayload(metadata); got != want {
		t.Fatalf("canonicalPayload() = %q, want %q", got, want)
	}
}

func writeValidFixture(t *testing.T) (verifyConfig, signedMetadata) {
	t.Helper()
	dir := t.TempDir()
	binaryName := "labtether-agent-linux-amd64"
	binaryPath := filepath.Join(dir, binaryName)
	binary := []byte("fixture agent binary\n")
	mustWrite(t, binaryPath, binary, 0o700)
	digestBytes := sha256.Sum256(binary)
	digest := hex.EncodeToString(digestBytes[:])

	seed := bytes.Repeat([]byte{0x42}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicKeyPath := filepath.Join(dir, "public-key.b64")
	mustWrite(t, publicKeyPath, []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0o600)

	metadata := signedMetadata{
		Version:   "v1.4.0",
		OS:        "linux",
		Arch:      "amd64",
		SHA256:    digest,
		SizeBytes: int64(len(binary)),
	}
	signature := ed25519.Sign(privateKey, []byte(canonicalPayload(metadata)))
	metadata.Signature = base64.StdEncoding.EncodeToString(signature)

	metadataPath := filepath.Join(dir, binaryName+"-v1.4.0.metadata.json")
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, metadataPath, metadataJSON, 0o600)
	signaturePath := filepath.Join(dir, binaryName+"-v1.4.0.sig")
	mustWrite(t, signaturePath, []byte(metadata.Signature+"\n"), 0o600)
	checksumPath := filepath.Join(dir, binaryName+".sha256")
	mustWrite(t, checksumPath, []byte(digest+"  "+binaryName+"\n"), 0o600)

	return verifyConfig{
		PublicKeyFile: publicKeyPath,
		BinaryFile:    binaryPath,
		ChecksumFile:  checksumPath,
		SignatureFile: signaturePath,
		MetadataFile:  metadataPath,
		Version:       metadata.Version,
		OS:            metadata.OS,
		Arch:          metadata.Arch,
		APIDigest:     "sha256:" + digest,
		APISize:       int64(len(binary)),
	}, metadata
}

func mustWrite(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
