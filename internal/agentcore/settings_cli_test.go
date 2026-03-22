package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureCLIOutput(t *testing.T, stdin string, fn func() int) (int, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStdin := os.Stdin

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	stdinPath := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(stdinPath, []byte(stdin), 0o600); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}
	stdinFile, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open stdin fixture: %v", err)
	}

	os.Stdout = stdoutWriter
	os.Stdin = stdinFile

	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stdin = oldStdin
	})

	exitCode := fn()

	_ = stdoutWriter.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	output, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	_ = stdoutReader.Close()
	_ = stdinFile.Close()
	return exitCode, string(output)
}

func runCLI(t *testing.T, cfg RuntimeConfig, args []string, stdin string) (bool, int, string) {
	t.Helper()

	handled := false
	exitCode, output := captureCLIOutput(t, stdin, func() int {
		var code int
		handled, code = HandleCLICommand(cfg, args)
		return code
	})
	return handled, exitCode, output
}

func TestHandleCLICommandRejectsUnknownCommand(t *testing.T) {
	handled, exitCode, output := runCLI(t, RuntimeConfig{}, []string{"nope"}, "")
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 1 {
		t.Fatalf("exitCode=%d, want 1", exitCode)
	}
	if !strings.Contains(output, "unknown command: nope") {
		t.Fatalf("output=%q, want unknown command message", output)
	}
}

func TestHandleCLICommandSettingsShowOutputsJSON(t *testing.T) {
	cfg := RuntimeConfig{
		CollectInterval:                           15 * time.Second,
		HeartbeatInterval:                         45 * time.Second,
		DockerEnabled:                             "auto",
		DockerSocket:                              "/var/run/docker.sock",
		DockerDiscoveryInterval:                   30 * time.Second,
		FileRootMode:                              "home",
		WebRTCEnabled:                             true,
		WebRTCSTUNURL:                             "stun:stun.example.com:3478",
		CaptureFPS:                                30,
		AllowRemoteOverrides:                      true,
		LogLevel:                                  "info",
		ServicesDiscoveryDockerEnabled:            true,
		ServicesDiscoveryProxyEnabled:             true,
		ServicesDiscoveryProxyTraefikEnabled:      true,
		ServicesDiscoveryProxyCaddyEnabled:        true,
		ServicesDiscoveryProxyNPMEnabled:          true,
		ServicesDiscoveryPortScanEnabled:          true,
		ServicesDiscoveryPortScanIncludeListening: true,
	}

	handled, exitCode, output := runCLI(t, cfg, []string{"settings", "show"}, "")
	if !handled {
		t.Fatalf("expected settings show to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(output), &values); err != nil {
		t.Fatalf("decode settings show output: %v\noutput=%s", err, output)
	}
	if values[SettingKeyCollectIntervalSec] != "15" {
		t.Fatalf("collect_interval_sec=%q, want 15", values[SettingKeyCollectIntervalSec])
	}
	if values[SettingKeyHeartbeatIntervalSec] != "45" {
		t.Fatalf("heartbeat_interval_sec=%q, want 45", values[SettingKeyHeartbeatIntervalSec])
	}
}

func TestRunSettingsCommandSetCanonicalizesMixedCaseKey(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "agent-settings.json")
	cfg := RuntimeConfig{AgentSettingsPath: settingsPath}

	exitCode, output := captureCLIOutput(t, "", func() int {
		return runSettingsCommand(cfg, []string{"set", "Docker_Enabled", "TRUE"})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0, output=%s", exitCode, output)
	}
	if !strings.Contains(output, "saved docker_enabled=true") {
		t.Fatalf("output=%q, want canonical saved key", output)
	}

	var payload struct {
		Version   int               `json:"version"`
		Values    map[string]string `json:"values"`
		UpdatedAt string            `json:"updated_at"`
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode settings file: %v", err)
	}
	if payload.Values[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("docker_enabled=%q, want true", payload.Values[SettingKeyDockerEnabled])
	}
	if _, ok := payload.Values["Docker_Enabled"]; ok {
		t.Fatalf("unexpected mixed-case key persisted: %+v", payload.Values)
	}
}

func TestRunSettingsWizardPersistsSelections(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "agent-settings.json")
	cfg := RuntimeConfig{
		AgentSettingsPath:       settingsPath,
		DockerEnabled:           "auto",
		DockerSocket:            "/var/run/docker.sock",
		DockerDiscoveryInterval: 30 * time.Second,
		FileRootMode:            "home",
		LogLevel:                "info",
		AllowRemoteOverrides:    true,
	}

	exitCode, output := captureCLIOutput(t, "true\nUNIX:///tmp/docker.sock\nfull\nfalse\n", func() int {
		return runSettingsWizard(cfg)
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0, output=%s", exitCode, output)
	}

	values, err := LoadAgentSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("load settings file: %v", err)
	}
	if values[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("docker_enabled=%q, want true", values[SettingKeyDockerEnabled])
	}
	if values[SettingKeyDockerEndpoint] != "unix:///tmp/docker.sock" {
		t.Fatalf("docker_endpoint=%q, want unix:///tmp/docker.sock", values[SettingKeyDockerEndpoint])
	}
	if values[SettingKeyFilesRootMode] != "full" {
		t.Fatalf("files_root_mode=%q, want full", values[SettingKeyFilesRootMode])
	}
	if values[SettingKeyAllowRemoteOverrides] != "false" {
		t.Fatalf("allow_remote_overrides=%q, want false", values[SettingKeyAllowRemoteOverrides])
	}
}

func TestRunSettingsCommandTestDockerUsesNormalizedEndpoint(t *testing.T) {
	originalPing := settingsCLIDockerPing
	t.Cleanup(func() {
		settingsCLIDockerPing = originalPing
	})

	var gotEndpoint string
	settingsCLIDockerPing = func(_ context.Context, endpoint string) error {
		gotEndpoint = endpoint
		return nil
	}

	cfg := RuntimeConfig{DockerSocket: "/var/run/docker.sock"}
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runSettingsCommand(cfg, []string{"test", "docker", " UNIX:///tmp/docker.sock "})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0, output=%s", exitCode, output)
	}
	if gotEndpoint != "unix:///tmp/docker.sock" {
		t.Fatalf("endpoint=%q, want unix:///tmp/docker.sock", gotEndpoint)
	}
	if !strings.Contains(output, "docker endpoint reachable: unix:///tmp/docker.sock") {
		t.Fatalf("output=%q, want normalized endpoint", output)
	}
}

func TestRunIdentityCommandShowUsesIdentitySeam(t *testing.T) {
	originalEnsure := settingsCLIEnsureDeviceIdentity
	t.Cleanup(func() {
		settingsCLIEnsureDeviceIdentity = originalEnsure
	})

	settingsCLIEnsureDeviceIdentity = func(cfg RuntimeConfig) (*deviceIdentity, error) {
		return &deviceIdentity{
			KeyAlgorithm: "ed25519",
			Fingerprint:  "fp-cli-test",
		}, nil
	}

	cfg := RuntimeConfig{
		DeviceKeyPath:         "/tmp/device-key",
		DevicePublicKeyPath:   "/tmp/device-key.pub",
		DeviceFingerprintPath: "/tmp/device-fingerprint",
	}
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runIdentityCommand(cfg, []string{"show"})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0, output=%s", exitCode, output)
	}
	if !strings.Contains(output, "Fingerprint: fp-cli-test") {
		t.Fatalf("output=%q, want fingerprint", output)
	}
	if !strings.Contains(output, "Key Algorithm: ed25519") {
		t.Fatalf("output=%q, want key algorithm", output)
	}
	if !strings.Contains(output, "Private Key: /tmp/device-key") {
		t.Fatalf("output=%q, want device key path", output)
	}
}

func TestRunUpdateCommandSelfUsesForceOption(t *testing.T) {
	originalSelfUpdate := settingsCLISelfUpdate
	t.Cleanup(func() {
		settingsCLISelfUpdate = originalSelfUpdate
	})

	called := false
	settingsCLISelfUpdate = func(_ RuntimeConfig, opts selfUpdateOptions) (bool, string, error) {
		called = true
		if !opts.Force {
			t.Fatalf("expected force=true")
		}
		return true, "forced update applied to 1.2.3", nil
	}

	exitCode, output := captureCLIOutput(t, "", func() int {
		return runUpdateCommand(RuntimeConfig{}, []string{"self", "--force"})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0, output=%s", exitCode, output)
	}
	if !called {
		t.Fatalf("expected self-update seam to be called")
	}
	if !strings.Contains(output, "forced update applied to 1.2.3") {
		t.Fatalf("output=%q, want update summary", output)
	}
	if !strings.Contains(output, "restart labtether-agent service") {
		t.Fatalf("output=%q, want restart hint", output)
	}
}

func TestRunSettingsCommandTestDockerReportsPingFailure(t *testing.T) {
	originalPing := settingsCLIDockerPing
	t.Cleanup(func() {
		settingsCLIDockerPing = originalPing
	})

	settingsCLIDockerPing = func(_ context.Context, endpoint string) error {
		if endpoint != "/var/run/docker.sock" {
			t.Fatalf("endpoint=%q, want /var/run/docker.sock", endpoint)
		}
		return errors.New("dial failed")
	}

	exitCode, output := captureCLIOutput(t, "", func() int {
		return runSettingsCommand(RuntimeConfig{DockerSocket: "/var/run/docker.sock"}, []string{"test", "docker"})
	})
	if exitCode != 1 {
		t.Fatalf("exitCode=%d, want 1, output=%s", exitCode, output)
	}
	if !strings.Contains(output, "docker endpoint check failed: dial failed") {
		t.Fatalf("output=%q, want ping failure", output)
	}
}

func TestTopLevelHelpShowsVersionAndCommands(t *testing.T) {
	cfg := RuntimeConfig{Version: "v2.1.0"}

	handled, exitCode, output := runCLI(t, cfg, []string{"help"}, "")
	if !handled {
		t.Fatalf("expected help to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	if !strings.Contains(output, "labtether-agent v2.1.0") {
		t.Fatalf("output=%q, want version header", output)
	}
	for _, keyword := range []string{"settings", "identity", "update", "help"} {
		if !strings.Contains(output, keyword) {
			t.Fatalf("output=%q, missing command %q", output, keyword)
		}
	}
}

func TestTopLevelHelpWithDashH(t *testing.T) {
	handled, exitCode, _ := runCLI(t, RuntimeConfig{Version: "dev"}, []string{"-h"}, "")
	if !handled {
		t.Fatalf("expected -h to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
}

func TestTopLevelHelpWithDoubleDashHelp(t *testing.T) {
	handled, exitCode, _ := runCLI(t, RuntimeConfig{Version: "dev"}, []string{"--help"}, "")
	if !handled {
		t.Fatalf("expected --help to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
}

func TestHelpSubcommandSettings(t *testing.T) {
	cfg := RuntimeConfig{AgentSettingsPath: "/etc/labtether/agent-config.json"}
	handled, exitCode, output := runCLI(t, cfg, []string{"help", "settings"}, "")
	if !handled {
		t.Fatalf("expected help settings to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	for _, keyword := range []string{"settings show", "settings set", "settings wizard", "settings test docker", "Environment variables", "LABTETHER_API_BASE_URL", "Available setting keys"} {
		if !strings.Contains(output, keyword) {
			t.Fatalf("output missing %q", keyword)
		}
	}
}

func TestHelpSubcommandIdentity(t *testing.T) {
	cfg := RuntimeConfig{
		DeviceKeyPath:         "/etc/labtether/device-key",
		DevicePublicKeyPath:   "/etc/labtether/device-key.pub",
		DeviceFingerprintPath: "/etc/labtether/device-fingerprint",
	}
	handled, exitCode, output := runCLI(t, cfg, []string{"help", "identity"}, "")
	if !handled {
		t.Fatalf("expected help identity to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	for _, keyword := range []string{"identity show", "Key files", "LABTETHER_DEVICE_KEY_FILE"} {
		if !strings.Contains(output, keyword) {
			t.Fatalf("output missing %q", keyword)
		}
	}
}

func TestHelpSubcommandUpdate(t *testing.T) {
	handled, exitCode, output := runCLI(t, RuntimeConfig{}, []string{"help", "update"}, "")
	if !handled {
		t.Fatalf("expected help update to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	for _, keyword := range []string{"update self", "--force", "LABTETHER_AUTO_UPDATE"} {
		if !strings.Contains(output, keyword) {
			t.Fatalf("output missing %q", keyword)
		}
	}
}

func TestHelpSubcommandUnknownReturnsError(t *testing.T) {
	handled, exitCode, output := runCLI(t, RuntimeConfig{}, []string{"help", "bogus"}, "")
	if !handled {
		t.Fatalf("expected help bogus to be handled")
	}
	if exitCode != 1 {
		t.Fatalf("exitCode=%d, want 1", exitCode)
	}
	if !strings.Contains(output, "unknown command: bogus") {
		t.Fatalf("output=%q, want unknown command message", output)
	}
}

func TestSettingsNoArgsShowsHelp(t *testing.T) {
	cfg := RuntimeConfig{AgentSettingsPath: "/etc/labtether/agent-config.json"}
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runSettingsCommand(cfg, []string{})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	if !strings.Contains(output, "settings show") {
		t.Fatalf("output=%q, want settings help", output)
	}
}

func TestSettingsDashHShowsHelp(t *testing.T) {
	cfg := RuntimeConfig{}
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runSettingsCommand(cfg, []string{"--help"})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	if !strings.Contains(output, "settings show") {
		t.Fatalf("output=%q, want settings help", output)
	}
}

func TestUpdateNoArgsShowsHelp(t *testing.T) {
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runUpdateCommand(RuntimeConfig{}, []string{})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	if !strings.Contains(output, "update self") {
		t.Fatalf("output=%q, want update help", output)
	}
}

func TestIdentityDashHShowsHelp(t *testing.T) {
	cfg := RuntimeConfig{}
	exitCode, output := captureCLIOutput(t, "", func() int {
		return runIdentityCommand(cfg, []string{"--help"})
	})
	if exitCode != 0 {
		t.Fatalf("exitCode=%d, want 0", exitCode)
	}
	if !strings.Contains(output, "identity") {
		t.Fatalf("output=%q, want identity help", output)
	}
}

func TestUnknownCommandShowsTopLevelHelp(t *testing.T) {
	cfg := RuntimeConfig{Version: "v1.0.0"}
	handled, exitCode, output := runCLI(t, cfg, []string{"nope"}, "")
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 1 {
		t.Fatalf("exitCode=%d, want 1", exitCode)
	}
	if !strings.Contains(output, "unknown command: nope") {
		t.Fatalf("output=%q, want unknown command message", output)
	}
	if !strings.Contains(output, "Commands:") {
		t.Fatalf("output=%q, want top-level help after unknown command", output)
	}
}
