package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
)

type deleteAssetHubCollectorStoreSpy struct {
	collectors []hubcollector.Collector
	deletedIDs []string
}

func (s *deleteAssetHubCollectorStoreSpy) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, errors.New("not implemented")
}

func (s *deleteAssetHubCollectorStoreSpy) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	for _, collector := range s.collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *deleteAssetHubCollectorStoreSpy) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	result := make([]hubcollector.Collector, 0, len(s.collectors))
	for _, collector := range s.collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		result = append(result, collector)
	}
	return result, nil
}

func (s *deleteAssetHubCollectorStoreSpy) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, errors.New("not implemented")
}

func (s *deleteAssetHubCollectorStoreSpy) DeleteHubCollector(id string) error {
	for idx, collector := range s.collectors {
		if collector.ID != id {
			continue
		}
		s.collectors = append(s.collectors[:idx], s.collectors[idx+1:]...)
		s.deletedIDs = append(s.deletedIDs, id)
		return nil
	}
	return hubcollector.ErrCollectorNotFound
}

func (s *deleteAssetHubCollectorStoreSpy) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func TestDeleteAsset_Success(t *testing.T) {
	sut := newTestAPIServer(t)

	// Create an asset via heartbeat first.
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "test-node-01",
		Type:    "server",
		Name:    "Test Node 01",
		Source:  "agent",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	// Verify asset exists.
	_, ok, _ := sut.assetStore.GetAsset("test-node-01")
	if !ok {
		t.Fatal("expected asset to exist before delete")
	}

	// DELETE /assets/test-node-01
	req := httptest.NewRequest(http.MethodDelete, "/assets/test-node-01", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["deleted"] != true {
		t.Fatalf("expected deleted=true, got %v", resp["deleted"])
	}
	if resp["asset_id"] != "test-node-01" {
		t.Fatalf("expected asset_id=test-node-01, got %v", resp["asset_id"])
	}

	// Verify asset is gone.
	_, ok, _ = sut.assetStore.GetAsset("test-node-01")
	if ok {
		t.Fatal("expected asset to be deleted")
	}
}

func TestDeleteAsset_NotFound(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/assets/nonexistent", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAsset_GetStillWorks(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "test-node-02",
		Type:    "server",
		Name:    "Test Node 02",
		Source:  "agent",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	// GET /assets/test-node-02 should still work.
	req := httptest.NewRequest(http.MethodGet, "/assets/test-node-02", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAsset_ConnectedAgentReturnsConflict(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "containervm-deltaserver",
		Type:    "host",
		Name:    "containervm-deltaserver",
		Source:  "agent",
	}); err != nil {
		t.Fatalf("failed to create connected agent asset: %v", err)
	}

	serverConn, cleanup := createTestWSPair(t)
	t.Cleanup(cleanup)
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "containervm-deltaserver", "linux"))

	req := httptest.NewRequest(http.MethodDelete, "/assets/containervm-deltaserver", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must be stopped or uninstalled before deletion") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if _, ok, _ := sut.assetStore.GetAsset("containervm-deltaserver"); !ok {
		t.Fatalf("expected connected agent asset to remain after conflict")
	}
}

func TestDeleteAsset_ConnectedAgentSendsSSHKeyRemoveBeforeConflict(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	sut.hubIdentity = &hubSSHIdentity{
		ProfileID: "cred-hub",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHUB hub",
	}

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "containervm-deltaserver",
		Type:    "host",
		Name:    "containervm-deltaserver",
		Source:  "agent",
	}); err != nil {
		t.Fatalf("failed to create connected agent asset: %v", err)
	}

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "containervm-deltaserver", "linux"))
	defer sut.agentMgr.Unregister("containervm-deltaserver")

	done := make(chan struct{})
	go func() {
		defer close(done)
		clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgSSHKeyRemove {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgSSHKeyRemove)
			return
		}

		var payload agentmgr.SSHKeyRemoveData
		if err := json.Unmarshal(outbound.Data, &payload); err != nil {
			t.Errorf("decode ssh remove payload: %v", err)
			return
		}
		if payload.PublicKey != sut.hubIdentity.PublicKey {
			t.Errorf("public_key=%q, want %q", payload.PublicKey, sut.hubIdentity.PublicKey)
		}
	}()

	req := httptest.NewRequest(http.MethodDelete, "/assets/containervm-deltaserver", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	<-done

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok, _ := sut.assetStore.GetAsset("containervm-deltaserver"); !ok {
		t.Fatalf("expected connected agent asset to remain after conflict")
	}
}

func TestDeleteAsset_AgentDeleteCascadesAttachedDockerAssets(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-01",
		Type:    "server",
		Name:    "Agent 01",
		Source:  "agent",
	})
	if err != nil {
		t.Fatalf("failed to create agent asset: %v", err)
	}

	attached := []assets.HeartbeatRequest{
		{
			AssetID: "docker-host-agent-01",
			Type:    "container-host",
			Name:    "docker-agent-01",
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "agent-01",
			},
		},
		{
			AssetID: "docker-ct-agent-01-abc123def456",
			Type:    "docker-container",
			Name:    "nginx",
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "agent-01",
			},
		},
		{
			AssetID: autoDockerCollectorAssetID("agent-01"),
			Type:    "connector-cluster",
			Name:    "docker-cluster-agent-01",
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "agent-01",
			},
		},
		{
			// Legacy/fallback path: attached by ID convention even if agent_id metadata is missing.
			AssetID: "docker-stack-agent-01-web",
			Type:    "compose-stack",
			Name:    "web",
			Source:  "docker",
		},
	}
	for _, req := range attached {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to create attached docker asset %s: %v", req.AssetID, err)
		}
	}

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-host-agent-02",
		Type:    "container-host",
		Name:    "docker-agent-02",
		Source:  "docker",
		Metadata: map[string]string{
			"agent_id": "agent-02",
		},
	}); err != nil {
		t.Fatalf("failed to create unrelated docker asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/agent-01", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{
		"agent-01",
		"docker-host-agent-01",
		"docker-ct-agent-01-abc123def456",
		autoDockerCollectorAssetID("agent-01"),
		"docker-stack-agent-01-web",
	} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}

	if _, ok, _ := sut.assetStore.GetAsset("docker-host-agent-02"); !ok {
		t.Fatalf("expected unrelated docker asset to remain")
	}
}

func TestDeleteAsset_NonAgentDeleteCascadesAttachedDockerAssetsAndWebServices(t *testing.T) {
	sut := newTestAPIServer(t)

	// Parent asset is not agent-sourced (for example connector-discovered VM).
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "containervm-deltaserver",
		Type:    "vm",
		Name:    "containervm-deltaserver",
		Source:  "proxmox",
	})
	if err != nil {
		t.Fatalf("failed to create parent asset: %v", err)
	}

	attached := []assets.HeartbeatRequest{
		{
			AssetID: "docker-stack-containervm-deltaserver-scrypted",
			Type:    "compose-stack",
			Name:    "scrypted",
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "containervm-deltaserver",
			},
		},
		{
			AssetID: "docker-ct-containervm-deltaserver-abc123def456",
			Type:    "docker-container",
			Name:    "go2rtc",
			Source:  "docker",
			Metadata: map[string]string{
				"agent_id": "containervm-deltaserver",
			},
		},
	}
	for _, req := range attached {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to create attached docker asset %s: %v", req.AssetID, err)
		}
	}

	// Seed web service cache for the same host to verify immediate removal.
	raw, _ := json.Marshal(agentmgr.WebServiceReportData{
		HostAssetID: "containervm-deltaserver",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-scrypted",
				Name:        "scrypted",
				HostAssetID: "containervm-deltaserver",
				Status:      "up",
			},
		},
	})
	sut.webServiceCoordinator.HandleReport("containervm-deltaserver", agentmgr.Message{
		Type: agentmgr.MsgWebServiceReport,
		Data: raw,
	})
	if got := len(sut.webServiceCoordinator.ListByHost("containervm-deltaserver")); got != 1 {
		t.Fatalf("expected 1 cached web service before delete, got %d", got)
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/containervm-deltaserver", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{
		"containervm-deltaserver",
		"docker-stack-containervm-deltaserver-scrypted",
		"docker-ct-containervm-deltaserver-abc123def456",
	} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}

	if got := len(sut.webServiceCoordinator.ListByHost("containervm-deltaserver")); got != 0 {
		t.Fatalf("expected cached web services for deleted host to be removed immediately, got %d", got)
	}
}

func TestDeleteAsset_NonDockerInfraDeleteCascadesAttachedInfraChildren(t *testing.T) {
	sut := newTestAPIServer(t)

	// Proxmox parent and children.
	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "proxmox-node-pve01",
			Type:    "hypervisor-node",
			Name:    "pve01",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "pve01",
				"collector_id": "collector-proxmox-1",
			},
		},
		{
			AssetID: "proxmox-vm-101",
			Type:    "vm",
			Name:    "vm-101",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "pve01",
				"collector_id": "collector-proxmox-1",
			},
		},
		{
			AssetID: "proxmox-vm-201",
			Type:    "vm",
			Name:    "vm-201",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "pve02",
				"collector_id": "collector-proxmox-1",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed proxmox asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/proxmox-node-pve01", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"proxmox-node-pve01", "proxmox-vm-101"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("proxmox-vm-201"); !ok {
		t.Fatalf("expected unrelated proxmox child to remain")
	}
}

func TestDeleteAsset_ProxmoxDeleteScopesCascadeByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "proxmox-node-shared",
			Type:    "hypervisor-node",
			Name:    "shared-node",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "shared-node",
				"collector_id": "collector-proxmox-a",
			},
		},
		{
			AssetID: "proxmox-vm-a1",
			Type:    "vm",
			Name:    "vm-a1",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "shared-node",
				"collector_id": "collector-proxmox-a",
			},
		},
		{
			AssetID: "proxmox-vm-b1",
			Type:    "vm",
			Name:    "vm-b1",
			Source:  "proxmox",
			Metadata: map[string]string{
				"node":         "shared-node",
				"collector_id": "collector-proxmox-b",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed proxmox asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/proxmox-node-shared", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"proxmox-node-shared", "proxmox-vm-a1"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("proxmox-vm-b1"); !ok {
		t.Fatalf("expected different-collector proxmox child to remain")
	}
}

func TestDeleteAsset_PortainerDeleteScopesCascadeByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "portainer-endpoint-a",
			Type:    "container-host",
			Name:    "endpoint-a",
			Source:  "portainer",
			Metadata: map[string]string{
				"endpoint_id":  "1",
				"collector_id": "collector-portainer-a",
			},
		},
		{
			AssetID: "portainer-container-a1",
			Type:    "container",
			Name:    "container-a1",
			Source:  "portainer",
			Metadata: map[string]string{
				"endpoint_id":  "1",
				"collector_id": "collector-portainer-a",
			},
		},
		{
			AssetID: "portainer-container-b1",
			Type:    "container",
			Name:    "container-b1",
			Source:  "portainer",
			Metadata: map[string]string{
				"endpoint_id":  "1",
				"collector_id": "collector-portainer-b",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed portainer asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/portainer-endpoint-a", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"portainer-endpoint-a", "portainer-container-a1"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("portainer-container-b1"); !ok {
		t.Fatalf("expected different-collector portainer child to remain")
	}
}

func TestDeleteAsset_PBSDeleteCascadesByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "pbs-root-a",
			Type:    "storage-controller",
			Name:    "pbs-root-a",
			Source:  "pbs",
			Metadata: map[string]string{
				"collector_id": "collector-pbs-a",
			},
		},
		{
			AssetID: "pbs-datastore-a",
			Type:    "storage-pool",
			Name:    "store-a",
			Source:  "pbs",
			Metadata: map[string]string{
				"collector_id": "collector-pbs-a",
			},
		},
		{
			AssetID: "pbs-datastore-b",
			Type:    "storage-pool",
			Name:    "store-b",
			Source:  "pbs",
			Metadata: map[string]string{
				"collector_id": "collector-pbs-b",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed pbs asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/pbs-root-a", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"pbs-root-a", "pbs-datastore-a"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("pbs-datastore-b"); !ok {
		t.Fatalf("expected unrelated pbs child to remain")
	}
}

func TestDeleteAsset_HomeAssistantDeleteCascadesByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "ha-cluster-a",
			Type:    "connector-cluster",
			Name:    "ha-cluster-a",
			Source:  "homeassistant",
			Metadata: map[string]string{
				"collector_id": "collector-ha-a",
			},
		},
		{
			AssetID: "ha-entity-light-a",
			Type:    "ha-entity",
			Name:    "light.a",
			Source:  "homeassistant",
			Metadata: map[string]string{
				"collector_id": "collector-ha-a",
			},
		},
		{
			AssetID: "ha-entity-light-b",
			Type:    "ha-entity",
			Name:    "light.b",
			Source:  "homeassistant",
			Metadata: map[string]string{
				"collector_id": "collector-ha-b",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed homeassistant asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/ha-cluster-a", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"ha-cluster-a", "ha-entity-light-a"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("ha-entity-light-b"); !ok {
		t.Fatalf("expected different-collector homeassistant child to remain")
	}
}

func TestDeleteAsset_FutureCollectorClusterDeleteCascadesByCollectorID(t *testing.T) {
	sut := newTestAPIServer(t)

	for _, req := range []assets.HeartbeatRequest{
		{
			AssetID: "future-cluster-a",
			Type:    "connector-cluster",
			Name:    "future-cluster-a",
			Source:  "futureapi",
			Metadata: map[string]string{
				"collector_id": "collector-future-a",
			},
		},
		{
			AssetID: "future-service-a",
			Type:    "service",
			Name:    "service-a",
			Source:  "futureapi",
			Metadata: map[string]string{
				"collector_id": "collector-future-a",
			},
		},
		{
			AssetID: "future-service-b",
			Type:    "service",
			Name:    "service-b",
			Source:  "futureapi",
			Metadata: map[string]string{
				"collector_id": "collector-future-b",
			},
		},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatalf("failed to seed future collector asset %s: %v", req.AssetID, err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/future-cluster-a", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, id := range []string{"future-cluster-a", "future-service-a"} {
		if _, ok, _ := sut.assetStore.GetAsset(id); ok {
			t.Fatalf("expected %s to be deleted", id)
		}
	}
	if _, ok, _ := sut.assetStore.GetAsset("future-service-b"); !ok {
		t.Fatalf("expected different-collector future source child to remain")
	}
}

func TestDeleteAsset_RemovesAutoDockerCollectorsForDeletedParent(t *testing.T) {
	sut := newTestAPIServer(t)
	hcStore := &deleteAssetHubCollectorStoreSpy{
		collectors: []hubcollector.Collector{
			{
				ID:            "hc-docker-auto-1",
				AssetID:       "docker-cluster-custom",
				CollectorType: hubcollector.CollectorTypeDocker,
				Enabled:       true,
				Config: map[string]any{
					"agent_asset_id": "containervm-delta",
					"provision_mode": "auto",
				},
			},
		},
	}
	sut.hubCollectorStore = hcStore

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "containervm-delta",
		Type:    "vm",
		Name:    "containervm-delta",
		Source:  "proxmox",
	}); err != nil {
		t.Fatalf("failed to seed parent asset: %v", err)
	}
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "docker-cluster-custom",
		Type:    "connector-cluster",
		Name:    "docker-cluster-custom",
		Source:  "docker",
	}); err != nil {
		t.Fatalf("failed to seed docker collector asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/assets/containervm-delta", nil)
	rec := httptest.NewRecorder()
	sut.handleAssetActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(hcStore.deletedIDs) != 1 || hcStore.deletedIDs[0] != "hc-docker-auto-1" {
		t.Fatalf("expected docker collector hc-docker-auto-1 to be deleted, deleted=%v", hcStore.deletedIDs)
	}
	if _, ok, _ := sut.assetStore.GetAsset("docker-cluster-custom"); ok {
		t.Fatalf("expected docker collector asset to be deleted")
	}
}
