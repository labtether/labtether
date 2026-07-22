// verify-agent-release validates a published LabTether Go agent binary against
// every independently supplied release binding before the hub embeds it.
//
// The Ed25519 payload format intentionally matches
// labtether-agent/scripts/release/sign-release.go:
//
//	version\nos\narch\nsha256\nsize
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxAgentBinaryBytes = 100 * 1024 * 1024
	maxMetadataBytes    = 64 * 1024
	maxSmallAssetBytes  = 16 * 1024
)

type signedMetadata struct {
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Signature string `json:"signature"`
}

type verifyConfig struct {
	PublicKeyFile string
	BinaryFile    string
	ChecksumFile  string
	SignatureFile string
	MetadataFile  string
	Version       string
	OS            string
	Arch          string
	APIDigest     string
	APISize       int64
}

func main() {
	var cfg verifyConfig
	flag.StringVar(&cfg.PublicKeyFile, "public-key-file", "", "file containing the trusted Ed25519 public key")
	flag.StringVar(&cfg.BinaryFile, "binary", "", "downloaded agent binary")
	flag.StringVar(&cfg.ChecksumFile, "checksum", "", "published SHA-256 checksum file")
	flag.StringVar(&cfg.SignatureFile, "signature", "", "published detached Ed25519 signature")
	flag.StringVar(&cfg.MetadataFile, "metadata", "", "published signed metadata JSON")
	flag.StringVar(&cfg.Version, "version", "", "expected exact release version")
	flag.StringVar(&cfg.OS, "os", "", "expected target operating system")
	flag.StringVar(&cfg.Arch, "arch", "", "expected target architecture")
	flag.StringVar(&cfg.APIDigest, "api-digest", "", "GitHub release asset digest")
	flag.Int64Var(&cfg.APISize, "api-size", -1, "GitHub release asset size")
	flag.Parse()

	metadata, err := verifyRelease(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-agent-release: %v\n", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(metadata); err != nil {
		fmt.Fprintf(os.Stderr, "verify-agent-release: encode verified metadata: %v\n", err)
		os.Exit(1)
	}
}

func verifyRelease(cfg verifyConfig) (signedMetadata, error) {
	for name, value := range map[string]string{
		"public key file": cfg.PublicKeyFile,
		"binary":          cfg.BinaryFile,
		"checksum":        cfg.ChecksumFile,
		"signature":       cfg.SignatureFile,
		"metadata":        cfg.MetadataFile,
		"version":         cfg.Version,
		"os":              cfg.OS,
		"arch":            cfg.Arch,
		"API digest":      cfg.APIDigest,
	} {
		if strings.TrimSpace(value) == "" {
			return signedMetadata{}, fmt.Errorf("%s is required", name)
		}
	}
	if cfg.APISize <= 0 || cfg.APISize > maxAgentBinaryBytes {
		return signedMetadata{}, fmt.Errorf("API size must be between 1 and %d bytes", maxAgentBinaryBytes)
	}

	metadata, err := readMetadata(cfg.MetadataFile)
	if err != nil {
		return signedMetadata{}, err
	}
	if metadata.Version != strings.TrimSpace(cfg.Version) {
		return signedMetadata{}, fmt.Errorf("signed metadata version does not match the exact requested release")
	}
	if metadata.OS != strings.TrimSpace(cfg.OS) || metadata.Arch != strings.TrimSpace(cfg.Arch) {
		return signedMetadata{}, fmt.Errorf("signed metadata platform does not match the requested platform")
	}
	metadataDigest, err := normalizeSHA256(metadata.SHA256)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("signed metadata sha256: %w", err)
	}
	if metadata.SHA256 != metadataDigest {
		return signedMetadata{}, fmt.Errorf("signed metadata sha256 must be lowercase canonical hex")
	}
	if metadata.SizeBytes <= 0 || metadata.SizeBytes > maxAgentBinaryBytes {
		return signedMetadata{}, fmt.Errorf("signed metadata size must be between 1 and %d bytes", maxAgentBinaryBytes)
	}

	apiDigest, err := normalizeSHA256(cfg.APIDigest)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("GitHub API digest: %w", err)
	}
	if metadataDigest != apiDigest || metadata.SizeBytes != cfg.APISize {
		return signedMetadata{}, fmt.Errorf("signed metadata does not match GitHub release asset digest and size")
	}

	checksumDigest, err := readChecksum(cfg.ChecksumFile, filepath.Base(cfg.BinaryFile))
	if err != nil {
		return signedMetadata{}, err
	}
	if checksumDigest != metadataDigest {
		return signedMetadata{}, fmt.Errorf("published checksum does not match signed metadata")
	}

	binaryDigest, binarySize, err := hashRegularFile(cfg.BinaryFile, maxAgentBinaryBytes)
	if err != nil {
		return signedMetadata{}, err
	}
	if binaryDigest != metadataDigest || binarySize != metadata.SizeBytes {
		return signedMetadata{}, fmt.Errorf("downloaded binary does not match signed metadata")
	}

	publicKeyText, err := readLimitedFile(cfg.PublicKeyFile, maxSmallAssetBytes)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("read trusted public key: %w", err)
	}
	publicKey, err := decodeFixedBytes(strings.TrimSpace(string(publicKeyText)), ed25519.PublicKeySize)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("decode trusted public key: %w", err)
	}

	detachedText, err := readLimitedFile(cfg.SignatureFile, maxSmallAssetBytes)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("read detached signature: %w", err)
	}
	detachedSignature, err := decodeFixedBytes(strings.TrimSpace(string(detachedText)), ed25519.SignatureSize)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("decode detached signature: %w", err)
	}
	metadataSignature, err := decodeFixedBytes(strings.TrimSpace(metadata.Signature), ed25519.SignatureSize)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("decode metadata signature: %w", err)
	}
	if !bytes.Equal(detachedSignature, metadataSignature) {
		return signedMetadata{}, fmt.Errorf("detached signature does not match signed metadata")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(canonicalPayload(metadata)), metadataSignature) {
		return signedMetadata{}, fmt.Errorf("Ed25519 release signature verification failed")
	}

	return metadata, nil
}

func readMetadata(path string) (signedMetadata, error) {
	data, err := readLimitedFile(path, maxMetadataBytes)
	if err != nil {
		return signedMetadata{}, fmt.Errorf("read signed metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var metadata signedMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return signedMetadata{}, fmt.Errorf("decode signed metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return signedMetadata{}, fmt.Errorf("decode signed metadata: trailing JSON value")
		}
		return signedMetadata{}, fmt.Errorf("decode signed metadata trailing content: %w", err)
	}
	return metadata, nil
}

func readChecksum(path, expectedName string) (string, error) {
	data, err := readLimitedFile(path, maxSmallAssetBytes)
	if err != nil {
		return "", fmt.Errorf("read published checksum: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		return "", fmt.Errorf("published checksum must contain exactly one entry")
	}
	fields := strings.Fields(lines[0])
	if len(fields) != 2 {
		return "", fmt.Errorf("published checksum entry must contain a digest and filename")
	}
	assetName := strings.TrimPrefix(fields[1], "*")
	if assetName != expectedName {
		return "", fmt.Errorf("published checksum filename does not match the expected binary")
	}
	digest, err := normalizeSHA256(fields[0])
	if err != nil {
		return "", fmt.Errorf("published checksum digest: %w", err)
	}
	return digest, nil
}

func hashRegularFile(path string, limit int64) (string, int64, error) {
	file, info, err := openRegularFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("open downloaded binary: %w", err)
	}
	defer file.Close()
	if info.Size() <= 0 || info.Size() > limit {
		return "", 0, fmt.Errorf("downloaded binary size must be between 1 and %d bytes", limit)
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil {
		return "", 0, fmt.Errorf("hash downloaded binary: %w", err)
	}
	if written != info.Size() {
		return "", 0, fmt.Errorf("downloaded binary changed while it was being verified")
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, info, err := openRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	return data, nil
}

func openRegularFile(path string) (*os.File, os.FileInfo, error) {
	cleanPath := filepath.Clean(path)
	directory := filepath.Dir(cleanPath)
	base := filepath.Base(cleanPath)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return nil, nil, fmt.Errorf("path does not name a file")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	info, err := root.Lstat(base)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("file must not be a symbolic link")
	}
	file, err := root.Open(base)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("file is not regular")
	}
	return file, openedInfo, nil
}

func normalizeSHA256(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, "sha256:")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("digest must be 64 hexadecimal characters")
	}
	return value, nil
}

func decodeFixedBytes(raw string, size int) ([]byte, error) {
	decoders := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	for _, encoding := range decoders {
		decoded, err := encoding.DecodeString(raw)
		if err == nil && len(decoded) == size {
			return decoded, nil
		}
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == size {
		return decoded, nil
	}
	return nil, fmt.Errorf("value must decode to exactly %s bytes", strconv.Itoa(size))
}

func canonicalPayload(metadata signedMetadata) string {
	return strings.Join([]string{
		strings.TrimSpace(metadata.Version),
		strings.TrimSpace(metadata.OS),
		strings.TrimSpace(metadata.Arch),
		strings.ToLower(strings.TrimSpace(metadata.SHA256)),
		fmt.Sprintf("%d", metadata.SizeBytes),
	}, "\n")
}
