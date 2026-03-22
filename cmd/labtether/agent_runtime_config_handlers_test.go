package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestSendConfigUpdateSendsStoredRuntimeOverrides(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		"agent_collect_interval_sec":   "30",
		"agent_heartbeat_interval_sec": "45",
	}); err != nil {
		t.Fatalf("save runtime overrides: %v", err)
	}

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	conn := agentmgr.NewAgentConn(serverConn, "node-runtime", "linux")
	sut.sendConfigUpdate(conn)

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var outbound agentmgr.Message
	if err := clientConn.ReadJSON(&outbound); err != nil {
		t.Fatalf("read outbound message: %v", err)
	}
	if outbound.Type != agentmgr.MsgConfigUpdate {
		t.Fatalf("message type=%q, want %q", outbound.Type, agentmgr.MsgConfigUpdate)
	}

	var payload agentmgr.ConfigUpdateData
	if err := json.Unmarshal(outbound.Data, &payload); err != nil {
		t.Fatalf("decode config update payload: %v", err)
	}
	if payload.CollectIntervalSec == nil || *payload.CollectIntervalSec != 30 {
		t.Fatalf("collect interval payload=%v, want 30", payload.CollectIntervalSec)
	}
	if payload.HeartbeatIntervalSec == nil || *payload.HeartbeatIntervalSec != 45 {
		t.Fatalf("heartbeat interval payload=%v, want 45", payload.HeartbeatIntervalSec)
	}
}

func TestSendConfigUpdateSendsExplicitZeroesWhenOverridesAreCleared(t *testing.T) {
	sut := newTestAPIServer(t)

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	conn := agentmgr.NewAgentConn(serverConn, "node-runtime", "linux")
	sut.sendConfigUpdate(conn)

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var outbound agentmgr.Message
	if err := clientConn.ReadJSON(&outbound); err != nil {
		t.Fatalf("read outbound message: %v", err)
	}

	var payload agentmgr.ConfigUpdateData
	if err := json.Unmarshal(outbound.Data, &payload); err != nil {
		t.Fatalf("decode config update payload: %v", err)
	}
	if payload.CollectIntervalSec == nil || *payload.CollectIntervalSec != 0 {
		t.Fatalf("collect interval payload=%v, want explicit zero", payload.CollectIntervalSec)
	}
	if payload.HeartbeatIntervalSec == nil || *payload.HeartbeatIntervalSec != 0 {
		t.Fatalf("heartbeat interval payload=%v, want explicit zero", payload.HeartbeatIntervalSec)
	}
}
