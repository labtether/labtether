package collectors

import (
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func TestScopedCollectorHeartbeatRequestClonesMetadataWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	original := map[string]string{
		"existing": "preserved",
		"second":   "value",
	}
	req := assets.HeartbeatRequest{
		AssetID:  "proxmox-vm-101",
		Metadata: original,
	}

	got := ScopedCollectorHeartbeatRequest("collector-a", req)
	if got.Metadata["existing"] != "preserved" || got.Metadata["second"] != "value" {
		t.Fatalf("metadata clone lost input entries: %#v", got.Metadata)
	}
	if got.Metadata["collector_id"] != "collector-a" {
		t.Fatalf("collector_id = %q, want collector-a", got.Metadata["collector_id"])
	}
	got.Metadata["existing"] = "changed"
	if original["existing"] != "preserved" {
		t.Fatalf("input metadata was mutated: %#v", original)
	}
	if _, ok := original[collectorNativeAssetIDMetadataKey]; ok {
		t.Fatalf("collector metadata leaked into input map: %#v", original)
	}
}

func TestProcessScopedCollectorHeartbeatKeepsRepeatedNativeIDsDistinct(t *testing.T) {
	t.Parallel()
	store := persistence.NewMemoryAssetStore()
	d := &Deps{
		AssetStore: store,
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			stored, err := store.UpsertAssetHeartbeat(req)
			return &stored, err
		},
	}

	cases := []struct {
		name       string
		nativeID   string
		collectors []string
	}{
		{name: "two Proxmox collectors reuse VMID", nativeID: "proxmox-vm-101", collectors: []string{"collector-proxmox-delta", "collector-proxmox-gamma"}},
		{name: "four Portainers reuse endpoint 2", nativeID: "portainer-endpoint-2", collectors: []string{"collector-portainer-delta", "collector-portainer-zeta", "collector-portainer-omega", "collector-portainer-tau"}},
		{name: "two PBS systems reuse datastore name", nativeID: "pbs-datastore-backup", collectors: []string{"collector-pbs-simba", "collector-pbs-mccann"}},
		{name: "two TrueNAS systems reuse MainPool", nativeID: "truenas-storage-pool-mainpool", collectors: []string{"collector-truenas-omega", "collector-truenas-tau"}},
		{name: "two Home Assistant systems reuse entity ID", nativeID: "ha-entity-sun-sun", collectors: []string{"collector-ha-simba", "collector-ha-mccann"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, collectorID := range tc.collectors {
				stored, err := d.ProcessScopedCollectorHeartbeat(collectorID, assets.HeartbeatRequest{
					AssetID: tc.nativeID,
					Type:    "test-resource",
					Name:    collectorID,
					Source:  "test",
				})
				if err != nil {
					t.Fatalf("ProcessScopedCollectorHeartbeat(%s): %v", collectorID, err)
				}
				wantID := assetid.ScopeCollectorAssetID(tc.nativeID, collectorID)
				if stored.ID != wantID {
					t.Fatalf("stored ID = %q, want %q", stored.ID, wantID)
				}
			}
			for _, collectorID := range tc.collectors {
				id := assetid.ScopeCollectorAssetID(tc.nativeID, collectorID)
				stored, ok, err := store.GetAsset(id)
				if err != nil || !ok {
					t.Fatalf("GetAsset(%s): ok=%v err=%v", id, ok, err)
				}
				if got := stored.Metadata["collector_id"]; got != collectorID {
					t.Fatalf("collector_id = %q, want %q", got, collectorID)
				}
			}
		})
	}
}

func TestProcessScopedCollectorHeartbeatSafelyAdoptsOwnedLegacyAsset(t *testing.T) {
	t.Parallel()
	store := persistence.NewMemoryAssetStore()
	legacyName := "Operator name"
	legacy, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "old name",
		Source:  "proxmox",
		GroupID: "group-a",
		Metadata: map[string]string{
			"collector_id": "collector-a",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	tags := []string{"critical", "production"}
	if _, err := store.UpdateAsset(legacy.ID, assets.UpdateRequest{Name: &legacyName, Tags: &tags}); err != nil {
		t.Fatal(err)
	}

	d := &Deps{
		AssetStore: store,
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			stored, err := store.UpsertAssetHeartbeat(req)
			return &stored, err
		},
	}
	stored, err := d.ProcessScopedCollectorHeartbeat("collector-a", assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "discovered name",
		Source:  "proxmox",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != legacyName || stored.GroupID != "group-a" {
		t.Fatalf("legacy customizations not adopted: %+v", stored)
	}
	if len(stored.Tags) != 2 {
		t.Fatalf("legacy tags not adopted: %#v", stored.Tags)
	}
	if _, ok, err := store.GetAsset("proxmox-vm-101"); err != nil || ok {
		t.Fatalf("legacy asset still exists: ok=%v err=%v", ok, err)
	}
}

func TestProcessScopedCollectorHeartbeatDoesNotAdoptAnotherCollectorsLegacyAsset(t *testing.T) {
	t.Parallel()
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "collector-a",
		Source:  "proxmox",
		Metadata: map[string]string{
			"collector_id": "collector-a",
		},
	}); err != nil {
		t.Fatal(err)
	}
	d := &Deps{
		AssetStore: store,
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			stored, err := store.UpsertAssetHeartbeat(req)
			return &stored, err
		},
	}
	if _, err := d.ProcessScopedCollectorHeartbeat("collector-b", assets.HeartbeatRequest{
		AssetID: "proxmox-vm-101",
		Type:    "vm",
		Name:    "collector-b",
		Source:  "proxmox",
	}); err != nil {
		t.Fatal(err)
	}
	legacy, ok, err := store.GetAsset("proxmox-vm-101")
	if err != nil || !ok || legacy.Metadata["collector_id"] != "collector-a" {
		t.Fatalf("other collector's legacy asset was changed: ok=%v err=%v asset=%+v", ok, err, legacy)
	}
}
