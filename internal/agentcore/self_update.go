package agentcore

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const selfUpdateExitCode = 10
const (
	envSelfUpdateTrustedPublicKey      = "LABTETHER_AUTO_UPDATE_TRUSTED_PUBLIC_KEY"
	envSelfUpdateAllowExternalDownload = "LABTETHER_AUTO_UPDATE_ALLOW_EXTERNAL_DOWNLOAD"
	maxSelfUpdateBinarySize            = 100 * 1024 * 1024 // 100MB
)

var (
	agentExitFn       = os.Exit
	executablePathFn  = os.Executable
	selfUpdateTimeout = 30 * time.Second
)

type agentReleaseMetadata struct {
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type selfUpdateOptions struct {
	Force bool
}

func maybeAutoUpdateOnStartup(cfg RuntimeConfig) error {
	if !cfg.AutoUpdateEnabled {
		return nil
	}

	updated, summary, err := checkAndApplySelfUpdate(cfg)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}

	// Exit non-zero so service managers configured with restart-on-failure
	// immediately relaunch the process with the new binary.
	_ = summary
	agentExitFn(selfUpdateExitCode)
	return fmt.Errorf("agent restart requested after self-update")
}

func checkAndApplySelfUpdate(cfg RuntimeConfig) (bool, string, error) {
	return checkAndApplySelfUpdateWithOptions(cfg, selfUpdateOptions{})
}

func checkAndApplySelfUpdateWithOptions(cfg RuntimeConfig, opts selfUpdateOptions) (bool, string, error) {
	checkURL := normalizeHTTPSURL(buildAgentReleaseCheckURL(cfg))
	if checkURL == "" {
		return false, "auto-update endpoint unavailable", nil
	}

	release, err := fetchReleaseMetadata(cfg, checkURL)
	if err != nil {
		return false, "", fmt.Errorf("fetch release metadata: %w", err)
	}
	if strings.TrimSpace(release.URL) == "" {
		return false, "", fmt.Errorf("release metadata missing download url")
	}
	downloadURL := resolveReleaseDownloadURL(checkURL, release.URL)
	if downloadURL == "" {
		return false, "", fmt.Errorf("release metadata produced an empty download url")
	}
	if err := validateReleaseMetadata(checkURL, downloadURL, release); err != nil {
		return false, "", err
	}
	if err := verifyReleaseMetadataSignature(release, downloadURL); err != nil {
		return false, "", err
	}

	executablePath, err := executablePathFn()
	if err != nil {
		return false, "", fmt.Errorf("resolve executable path: %w", err)
	}
	localSHA, err := fileSHA256(executablePath)
	if err != nil {
		return false, "", fmt.Errorf("hash local executable: %w", err)
	}
	if !opts.Force && release.SHA256 != "" && strings.EqualFold(localSHA, release.SHA256) {
		return false, "agent is already up to date", nil
	}

	binaryBytes, err := downloadReleaseBinary(cfg, checkURL, downloadURL, release.SizeBytes)
	if err != nil {
		return false, "", fmt.Errorf("download release binary: %w", err)
	}
	downloadSHA := sha256Hex(binaryBytes)
	if release.SHA256 != "" && !strings.EqualFold(downloadSHA, release.SHA256) {
		return false, "", fmt.Errorf("download checksum mismatch")
	}

	if err := replaceExecutable(executablePath, binaryBytes); err != nil {
		return false, "", fmt.Errorf("replace executable: %w", err)
	}
	version := strings.TrimSpace(release.Version)
	if version == "" {
		version = downloadSHA[:12]
	}
	if opts.Force {
		return true, fmt.Sprintf("forced update applied to %s", version), nil
	}
	return true, fmt.Sprintf("updated agent binary to %s", version), nil
}

func buildAgentReleaseCheckURL(cfg RuntimeConfig) string {
	if custom := strings.TrimSpace(cfg.AutoUpdateCheckURL); custom != "" {
		return normalizeHTTPSURL(custom)
	}
	if api := strings.TrimSpace(cfg.APIBaseURL); api != "" {
		return strings.TrimRight(normalizeAPIBaseURL(api), "/") + "/api/v1/agent/releases/latest"
	}
	ws := normalizeWSBaseURL(cfg.WSBaseURL)
	if ws == "" {
		return ""
	}
	parsed, err := url.Parse(ws)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "wss":
		parsed.Scheme = "https"
	case "ws":
		parsed.Scheme = "http"
	case "https", "http":
		// keep as-is
	default:
		return ""
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	base := strings.TrimRight(parsed.String(), "/")
	if base == "" {
		return ""
	}
	return base + "/api/v1/agent/releases/latest"
}

func fetchReleaseMetadata(cfg RuntimeConfig, endpoint string) (agentReleaseMetadata, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return agentReleaseMetadata{}, err
	}
	query := requestURL.Query()
	query.Set("os", runtime.GOOS)
	query.Set("arch", runtime.GOARCH)
	requestURL.RawQuery = query.Encode()

	client := newSelfUpdateHTTPClient(cfg)
	req, err := securityruntime.NewOutboundRequestWithContext(context.Background(), http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return agentReleaseMetadata{}, err
	}
	if token := strings.TrimSpace(cfg.APIToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return agentReleaseMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return agentReleaseMetadata{}, fmt.Errorf("release endpoint returned status %d", resp.StatusCode)
	}

	var payload agentReleaseMetadata
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return agentReleaseMetadata{}, err
	}
	return payload, nil
}

func downloadReleaseBinary(cfg RuntimeConfig, checkURL, absoluteURL string, expectedSize int64) ([]byte, error) {
	client := newSelfUpdateHTTPClient(cfg)
	req, err := securityruntime.NewOutboundRequestWithContext(context.Background(), http.MethodGet, absoluteURL, nil)
	if err != nil {
		return nil, err
	}
	if token := strings.TrimSpace(cfg.APIToken); token != "" && shouldAttachUpdateToken(checkURL, absoluteURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := securityruntime.DoOutboundRequest(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download endpoint returned status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxSelfUpdateBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxSelfUpdateBinarySize {
		return nil, fmt.Errorf("download exceeded maximum size of %d bytes", maxSelfUpdateBinarySize)
	}
	if expectedSize > 0 && int64(len(payload)) != expectedSize {
		return nil, fmt.Errorf("download size mismatch: expected %d bytes, got %d bytes", expectedSize, len(payload))
	}
	return payload, nil
}

func resolveReleaseDownloadURL(checkURL, releaseURL string) string {
	releaseURL = strings.TrimSpace(releaseURL)
	if releaseURL == "" {
		return ""
	}
	if parsed, err := url.Parse(releaseURL); err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(checkURL)
	if err != nil {
		return releaseURL
	}
	rel, err := url.Parse(releaseURL)
	if err != nil {
		return releaseURL
	}
	return base.ResolveReference(rel).String()
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- Path is the updater-managed artifact path selected by runtime config.
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func newSelfUpdateHTTPClient(cfg RuntimeConfig) *http.Client {
	transport := &http.Transport{}
	if tlsCfg := buildTLSConfig(&cfg); tlsCfg != nil {
		transport.TLSClientConfig = tlsCfg
	}
	return &http.Client{
		Timeout:   selfUpdateTimeout,
		Transport: transport,
	}
}

func validateReleaseMetadata(checkURL, downloadURL string, release agentReleaseMetadata) error {
	sha := strings.TrimSpace(strings.ToLower(release.SHA256))
	if sha == "" {
		return fmt.Errorf("release metadata missing sha256 digest")
	}
	if _, err := hex.DecodeString(sha); err != nil || len(sha) != 64 {
		return fmt.Errorf("release metadata includes an invalid sha256 digest")
	}
	if release.SizeBytes < 0 {
		return fmt.Errorf("release metadata includes a negative size")
	}
	if release.SizeBytes > maxSelfUpdateBinarySize {
		return fmt.Errorf("release size %d exceeds maximum supported size %d", release.SizeBytes, maxSelfUpdateBinarySize)
	}
	if !parseBoolEnv(envSelfUpdateAllowExternalDownload, false) && !sameOrigin(checkURL, downloadURL) {
		return fmt.Errorf("release download url must match release metadata origin")
	}
	return nil
}

func verifyReleaseMetadataSignature(release agentReleaseMetadata, downloadURL string) error {
	trustedKeyRaw := strings.TrimSpace(os.Getenv(envSelfUpdateTrustedPublicKey))
	if trustedKeyRaw == "" {
		return nil
	}
	keyBytes, err := decodeSelfUpdatePublicKey(trustedKeyRaw)
	if err != nil {
		return fmt.Errorf("decode %s: %w", envSelfUpdateTrustedPublicKey, err)
	}
	signatureRaw := strings.TrimSpace(release.Signature)
	if signatureRaw == "" {
		return fmt.Errorf("release metadata signature is required when %s is configured", envSelfUpdateTrustedPublicKey)
	}
	signature, err := decodeSelfUpdateSignature(signatureRaw)
	if err != nil {
		return fmt.Errorf("decode release signature: %w", err)
	}
	payload := []byte(selfUpdateSignaturePayload(release, downloadURL))
	if !ed25519.Verify(ed25519.PublicKey(keyBytes), payload, signature) {
		return fmt.Errorf("release metadata signature verification failed")
	}
	return nil
}

func decodeSelfUpdatePublicKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("public key is required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PublicKeySize {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PublicKeySize {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PublicKeySize {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == ed25519.PublicKeySize {
		return decoded, nil
	}
	return nil, fmt.Errorf("public key must be %d-byte base64 or hex", ed25519.PublicKeySize)
}

func decodeSelfUpdateSignature(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("signature is required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == ed25519.SignatureSize {
		return decoded, nil
	}
	return nil, fmt.Errorf("signature must be %d-byte base64 or hex", ed25519.SignatureSize)
}

func selfUpdateSignaturePayload(release agentReleaseMetadata, downloadURL string) string {
	return strings.Join([]string{
		strings.TrimSpace(release.Version),
		strings.TrimSpace(release.OS),
		strings.TrimSpace(release.Arch),
		strings.ToLower(strings.TrimSpace(release.SHA256)),
		strings.TrimSpace(downloadURL),
		fmt.Sprintf("%d", release.SizeBytes),
	}, "\n")
}

func shouldAttachUpdateToken(checkURL, downloadURL string) bool {
	return sameOrigin(checkURL, downloadURL)
}

func sameOrigin(leftRaw, rightRaw string) bool {
	left, err := url.Parse(strings.TrimSpace(leftRaw))
	if err != nil {
		return false
	}
	right, err := url.Parse(strings.TrimSpace(rightRaw))
	if err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(left.Scheme), strings.TrimSpace(right.Scheme)) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(left.Host), strings.TrimSpace(right.Host))
}
