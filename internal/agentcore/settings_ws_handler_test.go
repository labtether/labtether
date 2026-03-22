package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func decodeConfigApplied(t *testing.T, msg agentmgr.Message) agentmgr.ConfigAppliedData {
	t.Helper()
	if msg.Type != agentmgr.MsgConfigApplied {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgConfigApplied)
	}
	var payload agentmgr.ConfigAppliedData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode config.applied payload: %v", err)
	}
	return payload
}

func TestHandleConfigUpdateAcknowledgesEffectiveIntervalsOnInvalidUpdate(t *testing.T) {
	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		CollectInterval:   15 * time.Second,
		HeartbeatInterval: 45 * time.Second,
	})
	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	collect := 1
	heartbeat := 4
	data, err := json.Marshal(agentmgr.ConfigUpdateData{
		CollectIntervalSec:   &collect,
		HeartbeatIntervalSec: &heartbeat,
	})
	if err != nil {
		t.Fatalf("marshal config update: %v", err)
	}

	handleConfigUpdate(transport, agentmgr.Message{Type: agentmgr.MsgConfigUpdate, Data: data}, runtime)

	ack := decodeConfigApplied(t, readSettingsLifecycleMessage(t, messages))
	if ack.CollectIntervalSec != 15 {
		t.Fatalf("collect ack=%d, want 15", ack.CollectIntervalSec)
	}
	if ack.HeartbeatIntervalSec != 45 {
		t.Fatalf("heartbeat ack=%d, want 45", ack.HeartbeatIntervalSec)
	}
}

func TestHandleConfigUpdateAppliesValidIntervalsAndPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 20 * time.Second,
	})
	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	collect := 30
	heartbeat := 60
	data, err := json.Marshal(agentmgr.ConfigUpdateData{
		CollectIntervalSec:   &collect,
		HeartbeatIntervalSec: &heartbeat,
	})
	if err != nil {
		t.Fatalf("marshal config update: %v", err)
	}

	handleConfigUpdate(transport, agentmgr.Message{Type: agentmgr.MsgConfigUpdate, Data: data}, runtime)

	ack := decodeConfigApplied(t, readSettingsLifecycleMessage(t, messages))
	if ack.CollectIntervalSec != 30 {
		t.Fatalf("collect ack=%d, want 30", ack.CollectIntervalSec)
	}
	if ack.HeartbeatIntervalSec != 60 {
		t.Fatalf("heartbeat ack=%d, want 60", ack.HeartbeatIntervalSec)
	}
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

	path := filepath.Join(home, ".labtether", appliedConfigFile)
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read applied config: %v", err)
	}
	var persisted appliedConfig
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode applied config: %v", err)
	}
	if persisted.CollectIntervalSec != 30 || persisted.HeartbeatIntervalSec != 60 {
		t.Fatalf("persisted=%+v, want collect=30 heartbeat=60", persisted)
	}
}

func TestHandleConfigUpdateClearsLegacyOverridesBackToBaseline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		CollectInterval:   10 * time.Second,
		HeartbeatInterval: 20 * time.Second,
	})
	runtime.collectIntervalOverride.Store(30)
	runtime.heartbeatIntervalOverride.Store(60)
	runtime.cfg.CollectInterval = 30 * time.Second
	runtime.cfg.HeartbeatInterval = 60 * time.Second
	persistAppliedConfig(runtime)

	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	clearCollect := 0
	clearHeartbeat := 0
	data, err := json.Marshal(agentmgr.ConfigUpdateData{
		CollectIntervalSec:   &clearCollect,
		HeartbeatIntervalSec: &clearHeartbeat,
	})
	if err != nil {
		t.Fatalf("marshal config clear: %v", err)
	}

	handleConfigUpdate(transport, agentmgr.Message{Type: agentmgr.MsgConfigUpdate, Data: data}, runtime)

	ack := decodeConfigApplied(t, readSettingsLifecycleMessage(t, messages))
	if ack.CollectIntervalSec != 10 {
		t.Fatalf("collect ack=%d, want 10", ack.CollectIntervalSec)
	}
	if ack.HeartbeatIntervalSec != 20 {
		t.Fatalf("heartbeat ack=%d, want 20", ack.HeartbeatIntervalSec)
	}
	if got := runtime.collectIntervalOverride.Load(); got != 0 {
		t.Fatalf("collect override=%d, want 0", got)
	}
	if got := runtime.heartbeatIntervalOverride.Load(); got != 0 {
		t.Fatalf("heartbeat override=%d, want 0", got)
	}
	if runtime.cfg.CollectInterval != 10*time.Second {
		t.Fatalf("CollectInterval=%v, want 10s", runtime.cfg.CollectInterval)
	}
	if runtime.cfg.HeartbeatInterval != 20*time.Second {
		t.Fatalf("HeartbeatInterval=%v, want 20s", runtime.cfg.HeartbeatInterval)
	}

	path := filepath.Join(home, ".labtether", appliedConfigFile)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected applied config file to be removed, got err=%v", err)
	}
}
