package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigTLSSkipVerifyParsesBooleanEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "true lowercase", value: "true", want: true},
		{name: "true uppercase", value: "TRUE", want: true},
		{name: "true numeric", value: "1", want: true},
		{name: "true short", value: "t", want: true},
		{name: "false lowercase", value: "false", want: false},
		{name: "false numeric", value: "0", want: false},
		{name: "invalid falls back false", value: "definitely-not-bool", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LABTETHER_TLS_SKIP_VERIFY", tc.value)
			t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(t.TempDir(), "agent-token"))
			t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(t.TempDir(), "agent-config.json"))
			t.Setenv("LABTETHER_TLS_CA_FILE", "")

			cfg := LoadConfig("test-agent", "8090", "test")
			if cfg.TLSSkipVerify != tc.want {
				t.Fatalf("TLSSkipVerify=%v, want %v for env %q", cfg.TLSSkipVerify, tc.want, tc.value)
			}
		})
	}
}

func TestLoadConfigTLSSettingsOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "agent-config.json")
	payload := map[string]any{
		"version": 1,
		"values": map[string]string{
			SettingKeyTLSSkipVerify: "true",
			SettingKeyTLSCAFile:     "/tmp/ca-from-settings.pem",
		},
		"updated_at": "2026-03-01T00:00:00Z",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal settings payload: %v", err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", settingsPath)
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_TLS_SKIP_VERIFY", "false")
	t.Setenv("LABTETHER_TLS_CA_FILE", "/tmp/ca-from-env.pem")

	cfg := LoadConfig("test-agent", "8090", "test")
	if !cfg.TLSSkipVerify {
		t.Fatalf("expected TLSSkipVerify=true from settings override")
	}
	if cfg.TLSCAFile != "/tmp/ca-from-settings.pem" {
		t.Fatalf("expected TLSCAFile from settings override, got %q", cfg.TLSCAFile)
	}
}

func TestLoadConfigNormalizesSecureTransportDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-config.json"))
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	t.Setenv("LABTETHER_API_BASE_URL", "http://hub.example.com:8080")
	t.Setenv("LABTETHER_WS_URL", "ws://hub.example.com/ws/agent")
	t.Setenv("LABTETHER_AUTO_UPDATE_CHECK_URL", "http://hub.example.com/api/v1/agent/releases/latest")

	cfg := LoadConfig("test-agent", "8090", "test")
	if cfg.APIBaseURL != "https://hub.example.com:8080" {
		t.Fatalf("APIBaseURL = %q, want %q", cfg.APIBaseURL, "https://hub.example.com:8080")
	}
	if cfg.WSBaseURL != "wss://hub.example.com/ws/agent" {
		t.Fatalf("WSBaseURL = %q, want %q", cfg.WSBaseURL, "wss://hub.example.com/ws/agent")
	}
	if cfg.AutoUpdateCheckURL != "https://hub.example.com/api/v1/agent/releases/latest" {
		t.Fatalf("AutoUpdateCheckURL = %q, want https scheme", cfg.AutoUpdateCheckURL)
	}
}

func TestLoadConfigAllowsExplicitInsecureTransportOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-config.json"))
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_API_BASE_URL", "http://hub.example.com:8080")
	t.Setenv("LABTETHER_WS_URL", "ws://hub.example.com/ws/agent")

	cfg := LoadConfig("test-agent", "8090", "test")
	if cfg.APIBaseURL != "http://hub.example.com:8080" {
		t.Fatalf("APIBaseURL = %q, want insecure scheme to remain", cfg.APIBaseURL)
	}
	if cfg.WSBaseURL != "ws://hub.example.com/ws/agent" {
		t.Fatalf("WSBaseURL = %q, want insecure scheme to remain", cfg.WSBaseURL)
	}
}

func TestLoadConfigNormalizesDockerSocketFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-config.json"))
	t.Setenv("LABTETHER_DOCKER_SOCKET", " UNIX:///var/run/docker.sock ")

	cfg := LoadConfig("test-agent", "8090", "test")
	if cfg.DockerSocket != "unix:///var/run/docker.sock" {
		t.Fatalf("DockerSocket = %q, want unix:///var/run/docker.sock", cfg.DockerSocket)
	}
}

func TestLoadConfigFallsBackDefaultDockerSocketOnInvalidValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-config.json"))
	t.Setenv("LABTETHER_DOCKER_SOCKET", "not-a-valid-endpoint")

	cfg := LoadConfig("test-agent", "8090", "test")
	if cfg.DockerSocket != "/var/run/docker.sock" {
		t.Fatalf("DockerSocket = %q, want /var/run/docker.sock", cfg.DockerSocket)
	}
}
