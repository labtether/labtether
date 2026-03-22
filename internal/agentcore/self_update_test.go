package agentcore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBuildAgentReleaseCheckURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  RuntimeConfig
		want string
	}{
		{
			name: "custom override",
			cfg: RuntimeConfig{
				AutoUpdateCheckURL: "https://updates.example.com/release",
			},
			want: "https://updates.example.com/release",
		},
		{
			name: "api base",
			cfg: RuntimeConfig{
				APIBaseURL: "https://hub.example.com",
			},
			want: "https://hub.example.com/api/v1/agent/releases/latest",
		},
		{
			name: "ws base with path",
			cfg: RuntimeConfig{
				WSBaseURL: "wss://hub.example.com/ws/agent",
			},
			want: "https://hub.example.com/api/v1/agent/releases/latest",
		},
		{
			name: "missing base urls",
			cfg:  RuntimeConfig{},
			want: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := buildAgentReleaseCheckURL(tc.cfg); got != tc.want {
				t.Fatalf("buildAgentReleaseCheckURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckAndApplySelfUpdate_ReplacesExecutable(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	tempDir := t.TempDir()
	executablePath := filepath.Join(tempDir, "labtether-agent")
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write old executable: %v", err)
	}

	newBinary := []byte("new-binary-content")
	newSHA := sha256Hex(newBinary)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "v2.0.0",
				"sha256":  newSHA,
				"url":     server.URL + "/binary",
			})
		case "/binary":
			_, _ = w.Write(newBinary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalExecutablePathFn := executablePathFn
	executablePathFn = func() (string, error) { return executablePath, nil }
	t.Cleanup(func() { executablePathFn = originalExecutablePathFn })

	updated, summary, err := checkAndApplySelfUpdate(RuntimeConfig{
		AutoUpdateEnabled:  true,
		AutoUpdateCheckURL: server.URL + "/release",
	})
	if err != nil {
		t.Fatalf("checkAndApplySelfUpdate returned error: %v", err)
	}
	if !updated {
		t.Fatalf("expected update to be applied")
	}
	if !strings.Contains(summary, "v2.0.0") {
		t.Fatalf("expected summary to mention release version, got %q", summary)
	}

	content, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("read replaced executable: %v", err)
	}
	if string(content) != string(newBinary) {
		t.Fatalf("unexpected executable contents after update: got %q", string(content))
	}
}

func TestCheckAndApplySelfUpdate_NoopWhenChecksumMatches(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	tempDir := t.TempDir()
	executablePath := filepath.Join(tempDir, "labtether-agent")
	currentBinary := []byte("same-binary-content")
	if err := os.WriteFile(executablePath, currentBinary, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	currentSHA := sha256Hex(currentBinary)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "v2.0.0",
				"sha256":  currentSHA,
				"url":     server.URL + "/binary",
			})
			return
		}
		_, _ = w.Write(currentBinary)
	}))
	defer server.Close()

	originalExecutablePathFn := executablePathFn
	executablePathFn = func() (string, error) { return executablePath, nil }
	t.Cleanup(func() { executablePathFn = originalExecutablePathFn })

	updated, summary, err := checkAndApplySelfUpdate(RuntimeConfig{
		AutoUpdateEnabled:  true,
		AutoUpdateCheckURL: server.URL + "/release",
	})
	if err != nil {
		t.Fatalf("checkAndApplySelfUpdate returned error: %v", err)
	}
	if updated {
		t.Fatalf("expected no update when checksums match")
	}
	if !strings.Contains(summary, "up to date") {
		t.Fatalf("expected up-to-date summary, got %q", summary)
	}
}

func TestCheckAndApplySelfUpdate_ForceWhenChecksumMatches(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	tempDir := t.TempDir()
	executablePath := filepath.Join(tempDir, "labtether-agent")
	currentBinary := []byte("same-binary-content")
	if err := os.WriteFile(executablePath, currentBinary, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	currentSHA := sha256Hex(currentBinary)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "v2.0.0",
				"sha256":  currentSHA,
				"url":     server.URL + "/binary",
			})
			return
		}
		_, _ = w.Write(currentBinary)
	}))
	defer server.Close()

	originalExecutablePathFn := executablePathFn
	executablePathFn = func() (string, error) { return executablePath, nil }
	t.Cleanup(func() { executablePathFn = originalExecutablePathFn })

	updated, summary, err := checkAndApplySelfUpdateWithOptions(RuntimeConfig{
		AutoUpdateEnabled:  true,
		AutoUpdateCheckURL: server.URL + "/release",
	}, selfUpdateOptions{Force: true})
	if err != nil {
		t.Fatalf("checkAndApplySelfUpdateWithOptions returned error: %v", err)
	}
	if !updated {
		t.Fatalf("expected forced update to be applied")
	}
	if !strings.Contains(summary, "forced update applied") {
		t.Fatalf("expected forced-update summary, got %q", summary)
	}
}

func TestValidateReleaseMetadataRejectsCrossOriginByDefault(t *testing.T) {
	release := agentReleaseMetadata{
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	err := validateReleaseMetadata("https://hub.example.com/api/v1/agent/releases/latest", "https://cdn.example.com/agent.bin", release)
	if err == nil {
		t.Fatalf("expected cross-origin release metadata to be rejected")
	}
}

func TestShouldAttachUpdateTokenOnlyForSameOrigin(t *testing.T) {
	if !shouldAttachUpdateToken("https://hub.example.com/api/v1/agent/releases/latest", "https://hub.example.com/api/v1/agent/binary") {
		t.Fatalf("expected token forwarding for same-origin download")
	}
	if shouldAttachUpdateToken("https://hub.example.com/api/v1/agent/releases/latest", "https://cdn.example.com/agent.bin") {
		t.Fatalf("expected token forwarding to be disabled for cross-origin download")
	}
}

func TestVerifyReleaseMetadataSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	t.Setenv(envSelfUpdateTrustedPublicKey, base64.StdEncoding.EncodeToString(publicKey))

	downloadURL := "https://hub.example.com/api/v1/agent/binary?os=linux&arch=amd64"
	release := agentReleaseMetadata{
		Version:   "v2.0.0",
		OS:        "linux",
		Arch:      "amd64",
		SHA256:    strings.Repeat("a", 64),
		URL:       downloadURL,
		SizeBytes: 123,
	}
	release.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(selfUpdateSignaturePayload(release, downloadURL))))

	if err := verifyReleaseMetadataSignature(release, downloadURL); err != nil {
		t.Fatalf("expected valid signature to verify, got %v", err)
	}

	t.Run("missing signature is rejected", func(t *testing.T) {
		missingSig := release
		missingSig.Signature = ""
		if err := verifyReleaseMetadataSignature(missingSig, downloadURL); err == nil {
			t.Fatalf("expected missing signature to be rejected")
		}
	})

	t.Run("invalid signature is rejected", func(t *testing.T) {
		invalid := release
		invalid.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
		if err := verifyReleaseMetadataSignature(invalid, downloadURL); err == nil {
			t.Fatalf("expected invalid signature to be rejected")
		}
	})
}

func TestDownloadReleaseBinary(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	t.Run("attaches auth token for same-origin download", func(t *testing.T) {
		t.Setenv(envAllowInsecureTransport, "true")
		tokenSeen := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenSeen = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("same-origin-binary"))
		}))
		defer server.Close()

		payload, err := downloadReleaseBinary(RuntimeConfig{APIToken: "token-123"}, server.URL+"/release", server.URL+"/binary", int64(len("same-origin-binary")))
		if err != nil {
			t.Fatalf("downloadReleaseBinary returned error: %v", err)
		}
		if string(payload) != "same-origin-binary" {
			t.Fatalf("unexpected payload %q", string(payload))
		}
		if tokenSeen != "Bearer token-123" {
			t.Fatalf("expected auth token to be forwarded, got %q", tokenSeen)
		}
	})

	t.Run("does not attach auth token for cross-origin download", func(t *testing.T) {
		t.Setenv(envAllowInsecureTransport, "true")
		tokenSeen := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenSeen = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("cross-origin-binary"))
		}))
		defer server.Close()

		payload, err := downloadReleaseBinary(RuntimeConfig{APIToken: "token-123"}, "https://hub.example.com/release", server.URL+"/binary", int64(len("cross-origin-binary")))
		if err != nil {
			t.Fatalf("downloadReleaseBinary returned error: %v", err)
		}
		if string(payload) != "cross-origin-binary" {
			t.Fatalf("unexpected payload %q", string(payload))
		}
		if tokenSeen != "" {
			t.Fatalf("expected no auth token for cross-origin download, got %q", tokenSeen)
		}
	})

	t.Run("rejects HTTP error status", func(t *testing.T) {
		t.Setenv(envAllowInsecureTransport, "true")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		if _, err := downloadReleaseBinary(RuntimeConfig{}, server.URL+"/release", server.URL+"/binary", 0); err == nil {
			t.Fatalf("expected status error")
		}
	})

	t.Run("rejects size mismatch", func(t *testing.T) {
		t.Setenv(envAllowInsecureTransport, "true")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("size-mismatch"))
		}))
		defer server.Close()

		if _, err := downloadReleaseBinary(RuntimeConfig{}, server.URL+"/release", server.URL+"/binary", 99); err == nil {
			t.Fatalf("expected size mismatch error")
		}
	})

	t.Run("rejects oversized downloads", func(t *testing.T) {
		t.Setenv(envAllowInsecureTransport, "true")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			chunk := strings.Repeat("a", 1024*1024)
			for written := 0; written <= maxSelfUpdateBinarySize; written += len(chunk) {
				if _, err := w.Write([]byte(chunk)); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		if _, err := downloadReleaseBinary(RuntimeConfig{}, server.URL+"/release", server.URL+"/binary", 0); err == nil {
			t.Fatalf("expected oversized download to be rejected")
		}
	})
}

func TestCheckAndApplySelfUpdateRejectsSignedMetadataMismatch(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	t.Setenv(envSelfUpdateTrustedPublicKey, base64.StdEncoding.EncodeToString(publicKey))

	tempDir := t.TempDir()
	executablePath := filepath.Join(tempDir, "labtether-agent")
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	newBinary := []byte("signed-binary")
	newSHA := sha256Hex(newBinary)
	badDownloadURL := "https://hub.example.com/agent.bin"
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(selfUpdateSignaturePayload(agentReleaseMetadata{
		Version:   "v2.0.0",
		OS:        "linux",
		Arch:      "amd64",
		SHA256:    newSHA,
		URL:       badDownloadURL,
		SizeBytes: int64(len(newBinary)),
	}, badDownloadURL))))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version":    "v2.0.0",
				"os":         "linux",
				"arch":       "amd64",
				"sha256":     newSHA,
				"size_bytes": len(newBinary),
				"url":        server.URL + "/binary",
				"signature":  signature,
			})
		case "/binary":
			_, _ = w.Write(newBinary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalExecutablePathFn := executablePathFn
	executablePathFn = func() (string, error) { return executablePath, nil }
	t.Cleanup(func() { executablePathFn = originalExecutablePathFn })

	if _, _, err := checkAndApplySelfUpdate(RuntimeConfig{
		AutoUpdateEnabled:  true,
		AutoUpdateCheckURL: server.URL + "/release",
	}); err == nil {
		t.Fatalf("expected signature verification failure")
	}
}

func TestMaybeAutoUpdateOnStartupRequestsRestart(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	tempDir := t.TempDir()
	executablePath := filepath.Join(tempDir, "labtether-agent")
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write old executable: %v", err)
	}

	newBinary := []byte("new-binary-content")
	newSHA := sha256Hex(newBinary)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "v2.0.0",
				"sha256":  newSHA,
				"url":     server.URL + "/binary",
			})
		case "/binary":
			_, _ = w.Write(newBinary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalExecutablePathFn := executablePathFn
	executablePathFn = func() (string, error) { return executablePath, nil }
	t.Cleanup(func() { executablePathFn = originalExecutablePathFn })

	var exitCode string
	originalAgentExitFn := agentExitFn
	agentExitFn = func(code int) {
		exitCode = strconv.Itoa(code)
	}
	t.Cleanup(func() { agentExitFn = originalAgentExitFn })

	err := maybeAutoUpdateOnStartup(RuntimeConfig{
		AutoUpdateEnabled:  true,
		AutoUpdateCheckURL: server.URL + "/release",
	})
	if err == nil {
		t.Fatalf("expected restart error after successful self-update")
	}
	if exitCode != strconv.Itoa(selfUpdateExitCode) {
		t.Fatalf("expected exit code %d, got %q", selfUpdateExitCode, exitCode)
	}
}
