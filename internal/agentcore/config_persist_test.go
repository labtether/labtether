package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPersistedConfigRestoresEffectiveIntervalsAtStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".labtether")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	data, err := json.Marshal(appliedConfig{
		CollectIntervalSec:   30,
		HeartbeatIntervalSec: 60,
	})
	if err != nil {
		t.Fatalf("marshal applied config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, appliedConfigFile), data, 0o600); err != nil {
		t.Fatalf("write applied config: %v", err)
	}

	runtime := NewRuntime(RuntimeConfig{
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 20 * time.Second,
	}, nil, nil)

	loadPersistedConfig(runtime)

	if got := runtime.collectIntervalOverride.Load(); got != 30 {
		t.Fatalf("collect override=%d, want 30", got)
	}
	if got := runtime.heartbeatIntervalOverride.Load(); got != 60 {
		t.Fatalf("heartbeat override=%d, want 60", got)
	}
	if runtime.cfg.CollectInterval != 30*time.Second {
		t.Fatalf("CollectInterval=%v, want 30s", runtime.cfg.CollectInterval)
	}
	if runtime.cfg.HeartbeatInterval != 60*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want 60s", runtime.cfg.HeartbeatInterval)
	}
	if got := runtime.effectiveCollectIntervalSec(); got != 30 {
		t.Fatalf("effective collect=%d, want 30", got)
	}
	if got := runtime.effectiveHeartbeatIntervalSec(); got != 60 {
		t.Fatalf("effective heartbeat=%d, want 60", got)
	}
}

func TestPersistAppliedConfigRemovesStaleFileWhenOverridesCleared(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	runtime := NewRuntime(RuntimeConfig{
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 20 * time.Second,
	}, nil, nil)
	runtime.collectIntervalOverride.Store(30)
	runtime.heartbeatIntervalOverride.Store(60)

	persistAppliedConfig(runtime)

	path := filepath.Join(home, ".labtether", appliedConfigFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected applied config file: %v", err)
	}

	runtime.collectIntervalOverride.Store(0)
	runtime.heartbeatIntervalOverride.Store(0)
	persistAppliedConfig(runtime)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected applied config file to be removed, got err=%v", err)
	}
}
