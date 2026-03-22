package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/runtimesettings"
)

func TestAgentSettingGlobalDefaultKeyServiceDiscoveryMappings(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{agentcore.SettingKeyServicesDiscoveryDockerEnabled, runtimesettings.KeyServicesDiscoveryDefaultDockerEnabled},
		{agentcore.SettingKeyServicesDiscoveryProxyEnabled, runtimesettings.KeyServicesDiscoveryDefaultProxyEnabled},
		{agentcore.SettingKeyServicesDiscoveryProxyTraefikEnabled, runtimesettings.KeyServicesDiscoveryDefaultProxyTraefikEnabled},
		{agentcore.SettingKeyServicesDiscoveryProxyCaddyEnabled, runtimesettings.KeyServicesDiscoveryDefaultProxyCaddyEnabled},
		{agentcore.SettingKeyServicesDiscoveryProxyNPMEnabled, runtimesettings.KeyServicesDiscoveryDefaultProxyNPMEnabled},
		{agentcore.SettingKeyServicesDiscoveryPortScanEnabled, runtimesettings.KeyServicesDiscoveryDefaultPortScanEnabled},
		{agentcore.SettingKeyServicesDiscoveryPortScanIncludeListening, runtimesettings.KeyServicesDiscoveryDefaultPortScanIncludeListening},
		{agentcore.SettingKeyServicesDiscoveryPortScanPorts, runtimesettings.KeyServicesDiscoveryDefaultPortScanPorts},
		{agentcore.SettingKeyServicesDiscoveryLANScanEnabled, runtimesettings.KeyServicesDiscoveryDefaultLANScanEnabled},
		{agentcore.SettingKeyServicesDiscoveryLANScanCIDRs, runtimesettings.KeyServicesDiscoveryDefaultLANScanCIDRs},
		{agentcore.SettingKeyServicesDiscoveryLANScanPorts, runtimesettings.KeyServicesDiscoveryDefaultLANScanPorts},
		{agentcore.SettingKeyServicesDiscoveryLANScanMaxHosts, runtimesettings.KeyServicesDiscoveryDefaultLANScanMaxHosts},
	}

	for _, tt := range tests {
		got, ok := agentSettingGlobalDefaultKey(tt.key)
		if !ok {
			t.Fatalf("expected mapping for %s", tt.key)
		}
		if got != tt.want {
			t.Fatalf("agentSettingGlobalDefaultKey(%s) = %s; want %s", tt.key, got, tt.want)
		}
	}
}

func TestDockerConnectivityTestCommandUnixSocketPath(t *testing.T) {
	command := dockerConnectivityTestCommand("/var/run/docker.sock")
	if !strings.Contains(command, "--unix-socket") {
		t.Fatalf("expected unix socket curl check, got %q", command)
	}
	if !strings.Contains(command, "docker --host") {
		t.Fatalf("expected docker CLI fallback for unix socket, got %q", command)
	}
}

func TestDockerConnectivityTestCommandUnixScheme(t *testing.T) {
	command := dockerConnectivityTestCommand("unix:///custom/docker.sock")
	if !strings.Contains(command, "--unix-socket") {
		t.Fatalf("expected unix socket curl check, got %q", command)
	}
	if !strings.Contains(command, "unix:///custom/docker.sock") {
		t.Fatalf("expected unix endpoint in docker CLI fallback, got %q", command)
	}
}

func TestDockerConnectivityTestCommandUnixSchemeCaseInsensitive(t *testing.T) {
	command := dockerConnectivityTestCommand("UNIX:///custom/docker.sock")
	if !strings.Contains(command, "--unix-socket") {
		t.Fatalf("expected unix socket curl check, got %q", command)
	}
	if !strings.Contains(command, "/custom/docker.sock") {
		t.Fatalf("expected unix socket path in curl command, got %q", command)
	}
}

func TestDockerConnectivityTestCommandHTTP(t *testing.T) {
	command := dockerConnectivityTestCommand("http://127.0.0.1:2375")
	if !strings.Contains(command, "/_ping") {
		t.Fatalf("expected HTTP ping path, got %q", command)
	}
	if !strings.Contains(command, "docker --host") {
		t.Fatalf("expected docker CLI fallback, got %q", command)
	}
}

func TestHandleAgentSettingsUpdateAgentInvalidPayload(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/node-1/settings/update-agent", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()
	sut.handleAgentSettingsRoutes(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", rec.Code)
	}
}

func TestHandleAgentSettingsUpdateAgentRequiresConnectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/node-1/settings/update-agent", bytes.NewBufferString(`{"force":true}`))
	rec := httptest.NewRecorder()
	sut.handleAgentSettingsRoutes(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when agent is disconnected, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "connected") {
		t.Fatalf("expected connected-agent error, got %s", rec.Body.String())
	}
}

func TestHandleAgentSettingsUpdateAgentRequiresAgentManager(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = nil

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/node-1/settings/update-agent", bytes.NewBufferString(`{"force":false}`))
	rec := httptest.NewRecorder()
	sut.handleAgentSettingsRoutes(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when agent manager is unavailable, got %d", rec.Code)
	}
}

func TestHandleAgentSettingsRoutesRejectsExtraPathSegments(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/node-1/settings/update-agent/extra", bytes.NewBufferString(`{"force":true}`))
	rec := httptest.NewRecorder()
	sut.handleAgentSettingsRoutes(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for extra path segments, got %d", rec.Code)
	}
}

func TestDetermineAgentVersionStatus(t *testing.T) {
	if got := determineAgentVersionStatus("", "v1.2.3"); got != "unknown" {
		t.Fatalf("expected unknown when current is empty, got %q", got)
	}
	if got := determineAgentVersionStatus("v1.2.3", ""); got != "unknown" {
		t.Fatalf("expected unknown when latest is empty, got %q", got)
	}
	if got := determineAgentVersionStatus("v1.2.3", "v1.2.3"); got != "up_to_date" {
		t.Fatalf("expected up_to_date, got %q", got)
	}
	if got := determineAgentVersionStatus("v1.2.2", "v1.2.3"); got != "update_available" {
		t.Fatalf("expected update_available, got %q", got)
	}
}

func TestNormalizeAgentReleaseOS(t *testing.T) {
	cases := map[string]string{
		"linux":   "linux",
		"darwin":  "darwin",
		"macOS":   "darwin",
		"windows": "windows",
		"freebsd": "",
		"":        "",
	}
	for input, want := range cases {
		if got := normalizeAgentReleaseOS(input); got != want {
			t.Fatalf("normalizeAgentReleaseOS(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAgentReleaseArch(t *testing.T) {
	cases := map[string]string{
		"amd64":   "amd64",
		"x86_64":  "amd64",
		"x64":     "amd64",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"386":     "",
		"":        "",
	}
	for input, want := range cases {
		if got := normalizeAgentReleaseArch(input); got != want {
			t.Fatalf("normalizeAgentReleaseArch(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestAgentVersionFromBuildInfo(t *testing.T) {
	t.Run("prefers semantic version", func(t *testing.T) {
		got := agentVersionFromBuildInfo("v1.2.3", nil)
		if got != "v1.2.3" {
			t.Fatalf("agentVersionFromBuildInfo() = %q, want %q", got, "v1.2.3")
		}
	})

	t.Run("falls back to vcs revision", func(t *testing.T) {
		got := agentVersionFromBuildInfo("(devel)", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.modified", Value: "false"},
		})
		if got != "git:0123456789ab" {
			t.Fatalf("agentVersionFromBuildInfo() = %q, want %q", got, "git:0123456789ab")
		}
	})

	t.Run("includes dirty suffix", func(t *testing.T) {
		got := agentVersionFromBuildInfo("", []debug.BuildSetting{
			{Key: "vcs.revision", Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{Key: "vcs.modified", Value: "true"},
		})
		if got != "git:aaaaaaaaaaaa-dirty" {
			t.Fatalf("agentVersionFromBuildInfo() = %q, want %q", got, "git:aaaaaaaaaaaa-dirty")
		}
	})

	t.Run("returns empty when unavailable", func(t *testing.T) {
		got := agentVersionFromBuildInfo("(devel)", []debug.BuildSetting{})
		if got != "" {
			t.Fatalf("agentVersionFromBuildInfo() = %q, want empty", got)
		}
	})
}
