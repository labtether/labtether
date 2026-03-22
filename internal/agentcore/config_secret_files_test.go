package agentcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsSecretsFromFiles(t *testing.T) {
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "agent-token")
	enrollmentFile := filepath.Join(tempDir, "enrollment-token")
	turnPassFile := filepath.Join(tempDir, "turn-pass")

	if err := os.WriteFile(tokenFile, []byte("token-from-file\n"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if err := os.WriteFile(enrollmentFile, []byte("enroll-from-file\n"), 0600); err != nil {
		t.Fatalf("write enrollment file: %v", err)
	}
	if err := os.WriteFile(turnPassFile, []byte("turn-pass-from-file\n"), 0600); err != nil {
		t.Fatalf("write turn pass file: %v", err)
	}

	t.Setenv("LABTETHER_TOKEN_FILE", tokenFile)
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN_FILE", enrollmentFile)
	t.Setenv("LABTETHER_WEBRTC_TURN_PASS_FILE", turnPassFile)
	t.Setenv("LABTETHER_API_TOKEN", "")
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN", "")
	t.Setenv("LABTETHER_WEBRTC_TURN_PASS", "")

	cfg := LoadConfig("test-agent", "8090", "test")

	if cfg.APIToken != "token-from-file" {
		t.Fatalf("expected api token from file, got %q", cfg.APIToken)
	}
	if cfg.EnrollmentToken != "enroll-from-file" {
		t.Fatalf("expected enrollment token from file, got %q", cfg.EnrollmentToken)
	}
	if cfg.WebRTCTURNPass != "turn-pass-from-file" {
		t.Fatalf("expected turn pass from file, got %q", cfg.WebRTCTURNPass)
	}
	if cfg.EnrollmentTokenFilePath != enrollmentFile {
		t.Fatalf("expected enrollment token file path %q, got %q", enrollmentFile, cfg.EnrollmentTokenFilePath)
	}
	if cfg.WebRTCTURNPassFilePath != turnPassFile {
		t.Fatalf("expected turn pass file path %q, got %q", turnPassFile, cfg.WebRTCTURNPassFilePath)
	}
}

func TestResolveAgentLocalAuthTokenReadsFile(t *testing.T) {
	tempDir := t.TempDir()
	tokenFile := filepath.Join(tempDir, "local-auth-token")
	if err := os.WriteFile(tokenFile, []byte("local-auth-token\n"), 0600); err != nil {
		t.Fatalf("write local auth token file: %v", err)
	}

	t.Setenv(envAgentLocalAuthToken, "")
	t.Setenv(envAgentLocalAuthTokenFile, tokenFile)

	token, err := resolveAgentLocalAuthToken(RuntimeConfig{Name: "test-agent"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("resolve local auth token: %v", err)
	}
	if token != "local-auth-token" {
		t.Fatalf("expected local auth token from file, got %q", token)
	}
}

func TestResolveAgentLocalAuthTokenAllowsLoopbackWithoutToken(t *testing.T) {
	t.Setenv(envAgentLocalAuthToken, "")
	t.Setenv(envAgentLocalAuthTokenFile, "")
	t.Setenv(envAgentLocalAllowUnauth, "false")

	for _, bindAddress := range []string{"127.0.0.1", "::1", "localhost"} {
		bindAddress := bindAddress
		t.Run(bindAddress, func(t *testing.T) {
			token, err := resolveAgentLocalAuthToken(RuntimeConfig{Name: "test-agent"}, bindAddress)
			if err != nil {
				t.Fatalf("resolve local auth token: %v", err)
			}
			if token != "" {
				t.Fatalf("expected empty token for loopback bind %q, got %q", bindAddress, token)
			}
		})
	}
}

func TestResolveAgentLocalAuthTokenRejectsNonLoopbackWithoutAuth(t *testing.T) {
	t.Setenv(envAgentLocalAuthToken, "")
	t.Setenv(envAgentLocalAuthTokenFile, "")
	t.Setenv(envAgentLocalAllowUnauth, "false")

	if _, err := resolveAgentLocalAuthToken(RuntimeConfig{Name: "test-agent"}, "0.0.0.0"); err == nil {
		t.Fatalf("expected non-loopback bind without auth to be rejected")
	}
}

func TestResolveAgentLocalAuthTokenAllowsExplicitUnauthenticatedNonLoopbackBind(t *testing.T) {
	t.Setenv(envAgentLocalAuthToken, "")
	t.Setenv(envAgentLocalAuthTokenFile, "")
	t.Setenv(envAgentLocalAllowUnauth, "true")

	token, err := resolveAgentLocalAuthToken(RuntimeConfig{Name: "test-agent"}, "0.0.0.0")
	if err != nil {
		t.Fatalf("resolve local auth token: %v", err)
	}
	if token != "" {
		t.Fatalf("expected empty token when unauthenticated non-loopback bind is explicitly allowed, got %q", token)
	}
}

func TestResolveAgentLocalAuthTokenFallsBackToRuntimeAPIToken(t *testing.T) {
	t.Setenv(envAgentLocalAuthToken, "")
	t.Setenv(envAgentLocalAuthTokenFile, "")
	t.Setenv(envAgentLocalAllowUnauth, "false")

	token, err := resolveAgentLocalAuthToken(RuntimeConfig{
		Name:     "test-agent",
		APIToken: "runtime-api-token",
	}, "0.0.0.0")
	if err != nil {
		t.Fatalf("resolve local auth token: %v", err)
	}
	if token != "runtime-api-token" {
		t.Fatalf("expected runtime API token fallback, got %q", token)
	}
}
