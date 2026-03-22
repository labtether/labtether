package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAgentSettingsFileCanonicalizesMixedCaseKeys(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "agent-settings.json")
	payload := struct {
		Version   int               `json:"version"`
		Values    map[string]string `json:"values"`
		UpdatedAt string            `json:"updated_at"`
	}{
		Version: 1,
		Values: map[string]string{
			"Docker_Enabled":       "TRUE",
			"Collect_Interval_Sec": "45",
			"Log_Level":            "DEBUG",
		},
		UpdatedAt: "2026-03-09T00:00:00Z",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal settings payload: %v", err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		t.Fatalf("write settings payload: %v", err)
	}

	values, err := LoadAgentSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("load settings file: %v", err)
	}
	if values[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("docker_enabled=%q, want true", values[SettingKeyDockerEnabled])
	}
	if values[SettingKeyCollectIntervalSec] != "45" {
		t.Fatalf("collect_interval_sec=%q, want 45", values[SettingKeyCollectIntervalSec])
	}
	if values[SettingKeyLogLevel] != "debug" {
		t.Fatalf("log_level=%q, want debug", values[SettingKeyLogLevel])
	}
	if _, ok := values["Docker_Enabled"]; ok {
		t.Fatalf("unexpected mixed-case key in loaded values: %+v", values)
	}
}

func TestLoadConfigSettingsFileOverridesEnvAcrossRuntimeFields(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "agent-settings.json")
	payload := struct {
		Version   int               `json:"version"`
		Values    map[string]string `json:"values"`
		UpdatedAt string            `json:"updated_at"`
	}{
		Version: 1,
		Values: map[string]string{
			"Collect_Interval_Sec":              "45",
			"Heartbeat_Interval_Sec":            "90",
			"Docker_Enabled":                    "FALSE",
			"Docker_Endpoint":                   " UNIX:///tmp/docker.sock ",
			"Services_Discovery_Docker_Enabled": "false",
			"Allow_Remote_Overrides":            "true",
			"Log_Level":                         "DEBUG",
		},
		UpdatedAt: "2026-03-09T00:00:00Z",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal settings payload: %v", err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		t.Fatalf("write settings payload: %v", err)
	}

	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", settingsPath)
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("AGENT_COLLECT_INTERVAL", "12")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL", "30")
	t.Setenv("LABTETHER_DOCKER_ENABLED", "true")
	t.Setenv("LABTETHER_DOCKER_SOCKET", "/var/run/docker.sock")
	t.Setenv("LABTETHER_SERVICES_DISCOVERY_DOCKER_ENABLED", "true")
	t.Setenv("LABTETHER_ALLOW_REMOTE_OVERRIDES", "false")
	t.Setenv("LABTETHER_LOG_LEVEL", "error")

	cfg := LoadConfig("test-agent", "8090", "test")

	if cfg.CollectInterval != 45*time.Second {
		t.Fatalf("CollectInterval=%v, want 45s", cfg.CollectInterval)
	}
	if cfg.HeartbeatInterval != 90*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want 90s", cfg.HeartbeatInterval)
	}
	if cfg.DockerEnabled != "false" {
		t.Fatalf("DockerEnabled=%q, want false", cfg.DockerEnabled)
	}
	if cfg.DockerSocket != "unix:///tmp/docker.sock" {
		t.Fatalf("DockerSocket=%q, want unix:///tmp/docker.sock", cfg.DockerSocket)
	}
	if cfg.ServicesDiscoveryDockerEnabled {
		t.Fatalf("expected ServicesDiscoveryDockerEnabled=false")
	}
	if !cfg.AllowRemoteOverrides {
		t.Fatalf("expected AllowRemoteOverrides=true from settings file")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel=%q, want debug", cfg.LogLevel)
	}
}

func TestLoadConfigExplicitSecretsBeatFileSecrets(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "agent-token")
	enrollmentFile := filepath.Join(dir, "enrollment-token")
	turnPassFile := filepath.Join(dir, "turn-pass")

	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if err := os.WriteFile(enrollmentFile, []byte("file-enroll\n"), 0o600); err != nil {
		t.Fatalf("write enrollment file: %v", err)
	}
	if err := os.WriteFile(turnPassFile, []byte("file-turn\n"), 0o600); err != nil {
		t.Fatalf("write turn pass file: %v", err)
	}

	t.Setenv("LABTETHER_TOKEN_FILE", tokenFile)
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN_FILE", enrollmentFile)
	t.Setenv("LABTETHER_WEBRTC_TURN_PASS_FILE", turnPassFile)
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-settings.json"))
	t.Setenv("LABTETHER_API_TOKEN", "env-token")
	t.Setenv("LABTETHER_ENROLLMENT_TOKEN", "env-enroll")
	t.Setenv("LABTETHER_WEBRTC_TURN_PASS", "env-turn")

	cfg := LoadConfig("test-agent", "8090", "test")

	if cfg.APIToken != "env-token" {
		t.Fatalf("APIToken=%q, want env-token", cfg.APIToken)
	}
	if cfg.EnrollmentToken != "env-enroll" {
		t.Fatalf("EnrollmentToken=%q, want env-enroll", cfg.EnrollmentToken)
	}
	if cfg.WebRTCTURNPass != "env-turn" {
		t.Fatalf("WebRTCTURNPass=%q, want env-turn", cfg.WebRTCTURNPass)
	}
}
