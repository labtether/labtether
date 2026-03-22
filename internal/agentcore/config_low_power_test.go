package agentcore

import (
	"path/filepath"
	"testing"
	"time"
)

func setBaseConfigTestEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LABTETHER_TOKEN_FILE", filepath.Join(dir, "agent-token"))
	t.Setenv("LABTETHER_AGENT_SETTINGS_FILE", filepath.Join(dir, "agent-config.json"))

	// Clear overrides so tests only exercise the defaults/inputs in each case.
	keys := []string{
		"LABTETHER_LOW_POWER_MODE",
		"AGENT_COLLECT_INTERVAL",
		"AGENT_HEARTBEAT_INTERVAL",
		"LABTETHER_DOCKER_DISCOVERY_INTERVAL",
		"LABTETHER_SERVICES_DISCOVERY_INTERVAL",
		"LABTETHER_LOG_STREAM_ENABLED",
		"LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_ENABLED",
		"LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_INCLUDE_LISTENING",
		"LABTETHER_WEBSVC_PORTSCAN_DISABLED",
		"LABTETHER_WEBSVC_PORTSCAN_INCLUDE_LISTENING",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestLoadConfigDefaultsWhenLowPowerDisabled(t *testing.T) {
	setBaseConfigTestEnv(t)
	t.Setenv("LABTETHER_LOW_POWER_MODE", "false")

	cfg := LoadConfig("test-agent", "8090", "test")

	if cfg.LowPowerMode {
		t.Fatalf("LowPowerMode=%v, want false", cfg.LowPowerMode)
	}
	if got := cfg.CollectInterval; got != 10*time.Second {
		t.Fatalf("CollectInterval=%v, want %v", got, 10*time.Second)
	}
	if got := cfg.HeartbeatInterval; got != 20*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want %v", got, 20*time.Second)
	}
	if got := cfg.DockerDiscoveryInterval; got != 30*time.Second {
		t.Fatalf("DockerDiscoveryInterval=%v, want %v", got, 30*time.Second)
	}
	if got := cfg.WebServiceDiscoveryInterval; got != 60*time.Second {
		t.Fatalf("WebServiceDiscoveryInterval=%v, want %v", got, 60*time.Second)
	}
	if !cfg.ServicesDiscoveryPortScanEnabled {
		t.Fatal("ServicesDiscoveryPortScanEnabled=false, want true")
	}
	if !cfg.ServicesDiscoveryPortScanIncludeListening {
		t.Fatal("ServicesDiscoveryPortScanIncludeListening=false, want true")
	}
	if !cfg.LogStreamEnabled {
		t.Fatal("LogStreamEnabled=false, want true")
	}
}

func TestLoadConfigLowPowerDefaults(t *testing.T) {
	setBaseConfigTestEnv(t)
	t.Setenv("LABTETHER_LOW_POWER_MODE", "true")

	cfg := LoadConfig("test-agent", "8090", "test")

	if !cfg.LowPowerMode {
		t.Fatalf("LowPowerMode=%v, want true", cfg.LowPowerMode)
	}
	if got := cfg.CollectInterval; got != 30*time.Second {
		t.Fatalf("CollectInterval=%v, want %v", got, 30*time.Second)
	}
	if got := cfg.HeartbeatInterval; got != 120*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want %v", got, 120*time.Second)
	}
	if got := cfg.DockerDiscoveryInterval; got != 5*time.Minute {
		t.Fatalf("DockerDiscoveryInterval=%v, want %v", got, 5*time.Minute)
	}
	if got := cfg.WebServiceDiscoveryInterval; got != 10*time.Minute {
		t.Fatalf("WebServiceDiscoveryInterval=%v, want %v", got, 10*time.Minute)
	}
	if cfg.ServicesDiscoveryPortScanEnabled {
		t.Fatal("ServicesDiscoveryPortScanEnabled=true, want false")
	}
	if cfg.ServicesDiscoveryPortScanIncludeListening {
		t.Fatal("ServicesDiscoveryPortScanIncludeListening=true, want false")
	}
	if cfg.LogStreamEnabled {
		t.Fatal("LogStreamEnabled=true, want false")
	}
}

func TestLoadConfigLowPowerAllowsExplicitOverrides(t *testing.T) {
	setBaseConfigTestEnv(t)
	t.Setenv("LABTETHER_LOW_POWER_MODE", "true")
	t.Setenv("AGENT_COLLECT_INTERVAL", "12")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL", "25")
	t.Setenv("LABTETHER_DOCKER_DISCOVERY_INTERVAL", "45")
	t.Setenv("LABTETHER_SERVICES_DISCOVERY_INTERVAL", "90")
	t.Setenv("LABTETHER_LOG_STREAM_ENABLED", "true")
	t.Setenv("LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_ENABLED", "true")
	t.Setenv("LABTETHER_SERVICES_DISCOVERY_PORT_SCAN_INCLUDE_LISTENING", "true")

	cfg := LoadConfig("test-agent", "8090", "test")

	if got := cfg.CollectInterval; got != 12*time.Second {
		t.Fatalf("CollectInterval=%v, want %v", got, 12*time.Second)
	}
	if got := cfg.HeartbeatInterval; got != 25*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want %v", got, 25*time.Second)
	}
	if got := cfg.DockerDiscoveryInterval; got != 45*time.Second {
		t.Fatalf("DockerDiscoveryInterval=%v, want %v", got, 45*time.Second)
	}
	if got := cfg.WebServiceDiscoveryInterval; got != 90*time.Second {
		t.Fatalf("WebServiceDiscoveryInterval=%v, want %v", got, 90*time.Second)
	}
	if !cfg.ServicesDiscoveryPortScanEnabled {
		t.Fatal("ServicesDiscoveryPortScanEnabled=false, want true")
	}
	if !cfg.ServicesDiscoveryPortScanIncludeListening {
		t.Fatal("ServicesDiscoveryPortScanIncludeListening=false, want true")
	}
	if !cfg.LogStreamEnabled {
		t.Fatal("LogStreamEnabled=false, want true")
	}
}
