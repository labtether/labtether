package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/updates"
)

func TestUpdatePlanDryRunRequestsValidatedAgentPackagePreview(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-preview", "linux"))
	defer sut.agentMgr.Unregister("node-preview")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgPackageList {
			t.Errorf("outbound type = %q, want %q", outbound.Type, agentmgr.MsgPackageList)
			return
		}
		var request struct {
			RequestID string `json:"request_id"`
			Inventory string `json:"inventory"`
		}
		if err := json.Unmarshal(outbound.Data, &request); err != nil {
			t.Errorf("decode package preview request: %v", err)
			return
		}
		if request.RequestID == "" || request.RequestID != outbound.ID {
			t.Errorf("request correlation mismatch: envelope=%q payload=%q", outbound.ID, request.RequestID)
			return
		}
		if request.Inventory != "upgradable" {
			t.Errorf("inventory = %q, want upgradable", request.Inventory)
			return
		}

		data, err := json.Marshal(map[string]any{
			"request_id": request.RequestID,
			"inventory":  "upgradable",
			"packages": []map[string]any{
				{
					"name":              "curl",
					"version":           "8.5.0",
					"available_version": "8.6.0",
					"status":            "upgradable",
				},
			},
		})
		if err != nil {
			t.Errorf("encode package preview response: %v", err)
			return
		}
		sut.processAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-preview"}, agentmgr.Message{
			Type: agentmgr.MsgPackageListed,
			ID:   request.RequestID,
			Data: data,
		})
	}()

	entry := sut.executeUpdateScope(updates.Job{DryRun: true}, "node-preview", updates.ScopeOSPackages)
	<-done

	if entry.Status != updates.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded; entry=%+v", entry.Status, entry)
	}
	if !strings.Contains(entry.Summary, "curl 8.5.0 -> 8.6.0") || !strings.Contains(entry.Summary, "no changes applied") {
		t.Fatalf("summary = %q", entry.Summary)
	}
}

func TestUpdatePlanDryRunRejectsLegacyInstalledInventoryResponse(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-legacy", "linux"))
	defer sut.agentMgr.Unregister("node-legacy")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		var request struct {
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(outbound.Data, &request); err != nil {
			t.Errorf("decode package preview request: %v", err)
			return
		}

		// Legacy agents ignore the inventory discriminator and return the old
		// installed-package shape. The hub must not report this as a preview.
		data, err := json.Marshal(map[string]any{
			"request_id": request.RequestID,
			"packages": []map[string]any{
				{"name": "curl", "version": "8.5.0", "status": "installed"},
			},
		})
		if err != nil {
			t.Errorf("encode legacy response: %v", err)
			return
		}
		sut.processAgentPackageListed(&agentmgr.AgentConn{AssetID: "node-legacy"}, agentmgr.Message{
			Type: agentmgr.MsgPackageListed,
			ID:   request.RequestID,
			Data: data,
		})
	}()

	entry := sut.executeUpdateScope(updates.Job{DryRun: true}, "node-legacy", updates.ScopeOSPackages)
	<-done

	if entry.Status != updates.StatusFailed {
		t.Fatalf("status = %q, want failed; entry=%+v", entry.Status, entry)
	}
	if !strings.Contains(entry.Summary, "does not support upgradable package inventory") || !strings.Contains(entry.Summary, "no changes applied") {
		t.Fatalf("summary = %q", entry.Summary)
	}
}
