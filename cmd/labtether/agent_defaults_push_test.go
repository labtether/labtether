package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/agentsettings"
)

// Risk: a Hub-wide agent default changed in Settings reached agents only when
// they next connected, and after a reset agents kept the old value for good,
// because they keep pushed values over their own and the connect-time push
// sends only values the Hub has set.
func TestRuntimeAgentDefaultsReachConnectedAgents(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-defaults", "linux"))
	defer sut.agentMgr.Unregister("node-defaults")

	key := agentsettings.SettingKeyServicesDiscoveryPortScanPorts
	patch := httptest.NewRequest(http.MethodPatch, "/settings/runtime",
		bytes.NewReader([]byte(`{"values":{"services.discovery_default_port_scan_ports":"22,443,7443"}}`)))
	rec := httptest.NewRecorder()
	sut.handleRuntimeSettings(rec, patch)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH runtime settings: %d %s", rec.Code, rec.Body.String())
	}
	if got := readAppliedValues(t, clientConn)[key]; got != "22,443,7443" {
		t.Fatalf("pushed %s=%q after the change, want 22,443,7443", key, got)
	}

	reset := httptest.NewRequest(http.MethodPost, "/settings/runtime/reset",
		bytes.NewReader([]byte(`{"keys":["services.discovery_default_port_scan_ports"]}`)))
	rec = httptest.NewRecorder()
	sut.handleRuntimeSettingsReset(rec, reset)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset runtime setting: %d %s", rec.Code, rec.Body.String())
	}
	definition, _ := agentsettings.AgentSettingDefinitionByKey(key)
	values := readAppliedValues(t, clientConn)
	if got, ok := values[key]; !ok || got != definition.DefaultValue {
		t.Fatalf("pushed %s=%q (present %v) after the reset, want the default %q", key, got, ok, definition.DefaultValue)
	}
}

func readAppliedValues(t *testing.T, conn *websocket.Conn) map[string]string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var msg agentmgr.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("no settings sent to the connected agent: %v", err)
	}
	if msg.Type != agentmgr.MsgAgentSettingsApply {
		t.Fatalf("message type %q, want %q", msg.Type, agentmgr.MsgAgentSettingsApply)
	}
	var data agentmgr.AgentSettingsApplyData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		t.Fatalf("decode settings apply: %v", err)
	}
	return data.Values
}
