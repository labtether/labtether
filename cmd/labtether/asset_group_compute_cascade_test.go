package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateAssetGroupCascadesToProxmoxSubDevices(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Compute Group", "CMP-CASCADE")
	groupTwoID := createGroup("Backup Group", "BKP-CASCADE")

	recordHeartbeat(`{
		"asset_id":"proxmox-node-pve01",
		"type":"hypervisor-node",
		"name":"pve01",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve01"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-vm-101",
		"type":"vm",
		"name":"app-vm",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve01","vmid":"101"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-ct-102",
		"type":"container",
		"name":"svc-ct",
		"source":"proxmox",
		"status":"online",
		"metadata":{"node":"pve01","vmid":"102"}
	}`)
	recordHeartbeat(`{
		"asset_id":"proxmox-vm-201",
		"type":"vm",
		"name":"other-vm",
		"source":"proxmox",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"node":"pve02","vmid":"201"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/proxmox-node-pve01",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating parent host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("proxmox-node-pve01"); got != groupTwoID {
		t.Fatalf("expected parent group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-vm-101"); got != groupTwoID {
		t.Fatalf("expected vm child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-ct-102"); got != groupTwoID {
		t.Fatalf("expected ct child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("proxmox-vm-201"); got != groupOneID {
		t.Fatalf("expected unrelated child to remain on %q, got %q", groupOneID, got)
	}
}

func TestUpdateAssetGroupCascadesToDockerSubDevicesByAgentID(t *testing.T) {
	sut := newTestAPIServer(t)

	createGroup := func(name, slug string) string {
		return mustCreateGroup(t, sut, name, slug)
	}

	recordHeartbeat := func(payload string) {
		req := httptest.NewRequest(http.MethodPost, "/assets/heartbeat", bytes.NewReader([]byte(payload)))
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 heartbeat, got %d", rec.Code)
		}
	}

	getAssetGroupID := func(assetID string) string {
		req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID, nil)
		rec := httptest.NewRecorder()
		sut.handleAssetActions(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 fetching asset %s, got %d", assetID, rec.Code)
		}
		var out struct {
			Asset struct {
				GroupID string `json:"group_id"`
			} `json:"asset"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("failed to decode asset response for %s: %v", assetID, err)
		}
		return out.Asset.GroupID
	}

	groupOneID := createGroup("Docker Group A", "DOCK-A")
	groupTwoID := createGroup("Docker Group B", "DOCK-B")

	recordHeartbeat(`{
		"asset_id":"docker-host-agent-a",
		"type":"container-host",
		"name":"docker-agent-a",
		"source":"docker",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"agent_id":"agent-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"docker-ct-agent-a-web",
		"type":"docker-container",
		"name":"web",
		"source":"docker",
		"status":"online",
		"metadata":{"agent_id":"agent-a"}
	}`)
	recordHeartbeat(`{
		"asset_id":"docker-ct-agent-b-db",
		"type":"docker-container",
		"name":"db",
		"source":"docker",
		"group_id":"` + groupOneID + `",
		"status":"online",
		"metadata":{"agent_id":"agent-b"}
	}`)

	updateReq := httptest.NewRequest(
		http.MethodPatch,
		"/assets/docker-host-agent-a",
		bytes.NewReader([]byte(`{"group_id":"`+groupTwoID+`"}`)),
	)
	updateRec := httptest.NewRecorder()
	sut.handleAssetActions(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating docker host, got %d", updateRec.Code)
	}

	if got := getAssetGroupID("docker-host-agent-a"); got != groupTwoID {
		t.Fatalf("expected host group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("docker-ct-agent-a-web"); got != groupTwoID {
		t.Fatalf("expected attached docker child group_id %q, got %q", groupTwoID, got)
	}
	if got := getAssetGroupID("docker-ct-agent-b-db"); got != groupOneID {
		t.Fatalf("expected unrelated docker child to remain on %q, got %q", groupOneID, got)
	}
}
