package agentcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestApplyAgentSettingsRejectsLocalOnlyKey(t *testing.T) {
	runtime := NewRuntime(RuntimeConfig{
		AssetID:                 "node-local-only",
		CollectInterval:         10 * time.Second,
		HeartbeatInterval:       20 * time.Second,
		AllowRemoteOverrides:    true,
		AgentSettingsPath:       filepath.Join(t.TempDir(), "agent-settings.json"),
		DockerEnabled:           "auto",
		DockerDiscoveryInterval: 30 * time.Second,
		FileRootMode:            "home",
		LogLevel:                "info",
	}, nil, nil)

	if _, _, err := runtime.applyAgentSettings(map[string]string{
		SettingKeyAllowRemoteOverrides: "false",
	}); err == nil {
		t.Fatalf("expected local-only setting to be rejected")
	}
}

func TestHandleAgentSettingsApplyRejectsDisabledRemoteOverridesAndReportsState(t *testing.T) {
	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		AllowRemoteOverrides: false,
	})
	runtime.deviceIdentity = &deviceIdentity{Fingerprint: "fp-disabled"}

	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	request := mustMarshalSettingsMessage(t, agentmgr.AgentSettingsApplyData{
		RequestID: "req-disabled",
		Revision:  "rev-disabled",
		Values: map[string]string{
			SettingKeyCollectIntervalSec: "30",
		},
	})
	handleAgentSettingsApply(transport, agentmgr.Message{Type: agentmgr.MsgAgentSettingsApply, Data: request}, runtime)

	applied := decodeAgentSettingsApplied(t, readSettingsLifecycleMessage(t, messages))
	if applied.Applied {
		t.Fatalf("expected apply rejection, got %+v", applied)
	}
	if !strings.Contains(applied.Error, "disabled") {
		t.Fatalf("error=%q, want remote overrides disabled message", applied.Error)
	}
	if applied.Fingerprint != "fp-disabled" {
		t.Fatalf("fingerprint=%q, want fp-disabled", applied.Fingerprint)
	}

	state := decodeAgentSettingsState(t, readSettingsLifecycleMessage(t, messages))
	if state.Revision != "rev-disabled" {
		t.Fatalf("revision=%q, want rev-disabled", state.Revision)
	}
	if state.AllowRemoteOverrides {
		t.Fatalf("expected allow_remote_overrides=false")
	}
	if state.Values[SettingKeyCollectIntervalSec] != "10" {
		t.Fatalf("collect_interval_sec=%q, want 10", state.Values[SettingKeyCollectIntervalSec])
	}
}

func TestHandleAgentSettingsApplyRejectsFingerprintMismatchAndReportsCurrentState(t *testing.T) {
	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		AllowRemoteOverrides: true,
		LogLevel:             "info",
	})
	runtime.deviceIdentity = &deviceIdentity{Fingerprint: "fp-current"}

	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	request := mustMarshalSettingsMessage(t, agentmgr.AgentSettingsApplyData{
		RequestID:           "req-fp",
		Revision:            "rev-fp",
		ExpectedFingerprint: "fp-other",
		Values: map[string]string{
			SettingKeyLogLevel: "debug",
		},
	})
	handleAgentSettingsApply(transport, agentmgr.Message{Type: agentmgr.MsgAgentSettingsApply, Data: request}, runtime)

	applied := decodeAgentSettingsApplied(t, readSettingsLifecycleMessage(t, messages))
	if applied.Applied {
		t.Fatalf("expected fingerprint mismatch rejection")
	}
	if !strings.Contains(applied.Error, "fingerprint mismatch") {
		t.Fatalf("error=%q, want fingerprint mismatch", applied.Error)
	}

	state := decodeAgentSettingsState(t, readSettingsLifecycleMessage(t, messages))
	if state.Values[SettingKeyLogLevel] != "info" {
		t.Fatalf("log_level=%q, want info", state.Values[SettingKeyLogLevel])
	}
	if runtime.ReportedAgentSettings()[SettingKeyLogLevel] != "info" {
		t.Fatalf("runtime log_level=%q, want info", runtime.ReportedAgentSettings()[SettingKeyLogLevel])
	}
}

func TestHandleAgentSettingsApplyReportsRestartRequiredAndPersistsValues(t *testing.T) {
	runtime := newSettingsLifecycleRuntime(t, RuntimeConfig{
		AllowRemoteOverrides: true,
		LogLevel:             "info",
		DockerEnabled:        "auto",
	})
	runtime.deviceIdentity = &deviceIdentity{Fingerprint: "fp-success"}

	transport, messages, cleanup := newSettingsLifecycleTransport(t)
	defer cleanup()

	request := mustMarshalSettingsMessage(t, agentmgr.AgentSettingsApplyData{
		RequestID: "req-success",
		Revision:  "rev-success",
		Values: map[string]string{
			SettingKeyDockerEnabled:      "TRUE",
			SettingKeyCollectIntervalSec: "30",
			SettingKeyLogLevel:           "DEBUG",
		},
	})
	handleAgentSettingsApply(transport, agentmgr.Message{Type: agentmgr.MsgAgentSettingsApply, Data: request}, runtime)

	applied := decodeAgentSettingsApplied(t, readSettingsLifecycleMessage(t, messages))
	if !applied.Applied {
		t.Fatalf("expected apply success, got %+v", applied)
	}
	if !applied.RestartRequired {
		t.Fatalf("expected restart_required=true")
	}
	if applied.Fingerprint != "fp-success" {
		t.Fatalf("fingerprint=%q, want fp-success", applied.Fingerprint)
	}
	if applied.AppliedAt == "" {
		t.Fatalf("expected applied_at to be populated")
	}
	if applied.AppliedValues[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("docker_enabled=%q, want true", applied.AppliedValues[SettingKeyDockerEnabled])
	}
	if applied.AppliedValues[SettingKeyCollectIntervalSec] != "30" {
		t.Fatalf("collect_interval_sec=%q, want 30", applied.AppliedValues[SettingKeyCollectIntervalSec])
	}
	if applied.AppliedValues[SettingKeyLogLevel] != "debug" {
		t.Fatalf("log_level=%q, want debug", applied.AppliedValues[SettingKeyLogLevel])
	}

	state := decodeAgentSettingsState(t, readSettingsLifecycleMessage(t, messages))
	if !state.AllowRemoteOverrides {
		t.Fatalf("expected allow_remote_overrides=true")
	}
	if state.Values[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("state docker_enabled=%q, want true", state.Values[SettingKeyDockerEnabled])
	}
	if state.Values[SettingKeyCollectIntervalSec] != "30" {
		t.Fatalf("state collect_interval_sec=%q, want 30", state.Values[SettingKeyCollectIntervalSec])
	}
	if state.Values[SettingKeyLogLevel] != "debug" {
		t.Fatalf("state log_level=%q, want debug", state.Values[SettingKeyLogLevel])
	}

	if got := runtime.ReportedAgentSettings()[SettingKeyCollectIntervalSec]; got != "30" {
		t.Fatalf("runtime collect interval=%q, want 30", got)
	}
	if got := runtime.ReportedAgentSettings()[SettingKeyDockerEnabled]; got != "true" {
		t.Fatalf("runtime docker_enabled=%q, want true", got)
	}
	if got := runtime.ReportedAgentSettings()[SettingKeyLogLevel]; got != "debug" {
		t.Fatalf("runtime log_level=%q, want debug", got)
	}

	persisted, err := LoadAgentSettingsFile(runtime.cfg.AgentSettingsPath)
	if err != nil {
		t.Fatalf("load persisted settings: %v", err)
	}
	if persisted[SettingKeyDockerEnabled] != "true" {
		t.Fatalf("persisted docker_enabled=%q, want true", persisted[SettingKeyDockerEnabled])
	}
	if persisted[SettingKeyCollectIntervalSec] != "30" {
		t.Fatalf("persisted collect_interval_sec=%q, want 30", persisted[SettingKeyCollectIntervalSec])
	}
	if persisted[SettingKeyLogLevel] != "debug" {
		t.Fatalf("persisted log_level=%q, want debug", persisted[SettingKeyLogLevel])
	}
}

func newSettingsLifecycleRuntime(t *testing.T, overrides RuntimeConfig) *Runtime {
	t.Helper()

	cfg := RuntimeConfig{
		AssetID:                 "node-settings",
		CollectInterval:         10 * time.Second,
		HeartbeatInterval:       20 * time.Second,
		AllowRemoteOverrides:    true,
		AgentSettingsPath:       filepath.Join(t.TempDir(), "agent-settings.json"),
		DockerEnabled:           "auto",
		DockerDiscoveryInterval: 30 * time.Second,
		FileRootMode:            "home",
		LogLevel:                "info",
	}
	if overrides.AssetID != "" {
		cfg.AssetID = overrides.AssetID
	}
	if overrides.CollectInterval != 0 {
		cfg.CollectInterval = overrides.CollectInterval
	}
	if overrides.HeartbeatInterval != 0 {
		cfg.HeartbeatInterval = overrides.HeartbeatInterval
	}
	cfg.AllowRemoteOverrides = overrides.AllowRemoteOverrides
	if overrides.AgentSettingsPath != "" {
		cfg.AgentSettingsPath = overrides.AgentSettingsPath
	}
	if overrides.DockerEnabled != "" {
		cfg.DockerEnabled = overrides.DockerEnabled
	}
	if overrides.DockerDiscoveryInterval != 0 {
		cfg.DockerDiscoveryInterval = overrides.DockerDiscoveryInterval
	}
	if overrides.FileRootMode != "" {
		cfg.FileRootMode = overrides.FileRootMode
	}
	if overrides.LogLevel != "" {
		cfg.LogLevel = overrides.LogLevel
	}
	return NewRuntime(cfg, nil, nil)
}

func newSettingsLifecycleTransport(t *testing.T) (*wsTransport, <-chan agentmgr.Message, func()) {
	t.Helper()
	t.Setenv(envAllowInsecureTransport, "true")

	messages := make(chan agentmgr.Message, 8)
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		for {
			var msg agentmgr.Message
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			messages <- msg
		}
	}))

	transport := newWSTransport(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		"token-123",
		"node-settings",
		"linux",
		"dev",
		nil,
		"",
		nil,
	)
	resp, err := transport.connectWithResponse(context.Background())
	if err != nil {
		server.Close()
		t.Fatalf("connect transport: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	cleanup := func() {
		transport.Close()
		server.Close()
	}
	return transport, messages, cleanup
}

func readSettingsLifecycleMessage(t *testing.T, messages <-chan agentmgr.Message) agentmgr.Message {
	t.Helper()
	select {
	case msg := <-messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent settings message")
		return agentmgr.Message{}
	}
}

func decodeAgentSettingsApplied(t *testing.T, msg agentmgr.Message) agentmgr.AgentSettingsAppliedData {
	t.Helper()
	if msg.Type != agentmgr.MsgAgentSettingsApplied {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgAgentSettingsApplied)
	}
	var payload agentmgr.AgentSettingsAppliedData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode applied payload: %v", err)
	}
	return payload
}

func decodeAgentSettingsState(t *testing.T, msg agentmgr.Message) agentmgr.AgentSettingsStateData {
	t.Helper()
	if msg.Type != agentmgr.MsgAgentSettingsState {
		t.Fatalf("message type=%q, want %q", msg.Type, agentmgr.MsgAgentSettingsState)
	}
	var payload agentmgr.AgentSettingsStateData
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("decode state payload: %v", err)
	}
	return payload
}

func mustMarshalSettingsMessage(t *testing.T, payload any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}
