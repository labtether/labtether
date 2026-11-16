package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/agentmgr"
	"testing"
)

func TestProcessAgentHeartbeatIgnoresPayloadAssetID(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.HeartbeatData{
		AssetID:  "spoofed-node",
		Type:     "node",
		Name:     "trusted-node",
		Source:   "agent",
		Status:   "online",
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}

	sut.ensureAgentsDeps().ProcessAgentHeartbeat(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: payload})

	if _, ok, err := sut.assetStore.GetAsset("trusted-node"); err != nil {
		t.Fatalf("failed to get trusted asset: %v", err)
	} else if !ok {
		t.Fatalf("expected trusted asset to be updated")
	}
	if _, ok, err := sut.assetStore.GetAsset("spoofed-node"); err != nil {
		t.Fatalf("failed to get spoofed asset: %v", err)
	} else if ok {
		t.Fatalf("did not expect spoofed asset to be updated from payload asset_id")
	}
}

func TestProcessAgentHeartbeatStoresWebRTCUnavailabilityReason(t *testing.T) {
	sut := newTestAPIServer(t)
	conn := &agentmgr.AgentConn{AssetID: "trusted-node"}

	payload, err := json.Marshal(agentmgr.HeartbeatData{
		Type:     "node",
		Name:     "trusted-node",
		Source:   "agent",
		Status:   "online",
		Platform: "darwin",
		Metadata: map[string]string{
			"webrtc_available":          "false",
			"webrtc_unavailable_reason": "unsupported_platform:darwin",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal heartbeat payload: %v", err)
	}

	sut.ensureAgentsDeps().ProcessAgentHeartbeat(conn, agentmgr.Message{Type: agentmgr.MsgHeartbeat, Data: payload})

	if got := conn.Meta("webrtc_unavailable_reason"); got != "unsupported_platform:darwin" {
		t.Fatalf("expected heartbeat to store webrtc unavailability reason, got %q", got)
	}
	assetEntry, ok, err := sut.assetStore.GetAsset("trusted-node")
	if err != nil {
		t.Fatalf("failed to get trusted asset: %v", err)
	}
	if !ok {
		t.Fatal("expected trusted asset to exist")
	}
	if got := assetEntry.Metadata["webrtc_unavailable_reason"]; got != "unsupported_platform:darwin" {
		t.Fatalf("expected asset metadata to persist webrtc reason, got %q", got)
	}
}
