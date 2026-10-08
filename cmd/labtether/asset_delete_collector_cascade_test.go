package main

import (
	"github.com/labtether/labtether/internal/assets"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
