package main

import (
	"github.com/labtether/labtether/internal/assets"
	"strings"
	"testing"
)

func TestResolveProxmoxSessionTargetStorageFallbacks(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-local-zfs",
		Type:    "storage-pool",
		Name:    "storage/pve01/local-zfs",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"storage_id":   "storage/pve01/local-zfs",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed storage asset: %v", err)
	}

	target, ok, err := sut.resolveProxmoxSessionTarget("proxmox-storage-local-zfs")
	if err != nil {
		t.Fatalf("resolveProxmoxSessionTarget returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected proxmox storage target to resolve")
	}
	if target.Kind != "storage" || target.Node != "pve01" || target.StorageName != "local-zfs" {
		t.Fatalf("unexpected resolved storage target: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-fast-ssd",
		Type:    "storage-pool",
		Name:    "storage/pve02/fast-ssd",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed second storage asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-fast-ssd")
	if err != nil {
		t.Fatalf("resolveProxmoxSessionTarget returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected second proxmox storage target to resolve")
	}
	if target.Node != "pve02" || target.StorageName != "fast-ssd" {
		t.Fatalf("unexpected fallback target parse: %+v", target)
	}
}

func TestResolveProxmoxSessionTargetStorageMissingNode(t *testing.T) {
	sut := newTestAPIServer(t)
	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-broken",
		Type:    "storage-pool",
		Name:    "",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"storage_id":   "local-zfs",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed broken storage asset: %v", err)
	}

	_, ok, err := sut.resolveProxmoxSessionTarget("proxmox-storage-broken")
	if !ok {
		t.Fatalf("expected proxmox asset detection to be true")
	}
	if err == nil || !strings.Contains(err.Error(), "missing node metadata") {
		t.Fatalf("expected missing node metadata error, got %v", err)
	}
}

func TestResolveProxmoxSessionTargetAdditionalBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	target, ok, err := sut.resolveProxmoxSessionTarget("missing-asset")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected unresolved missing asset, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-host-01",
		Type:    "server",
		Name:    "agent-host-01",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed non-proxmox asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("agent-host-01")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected non-proxmox asset to be ignored, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-110",
		Type:    "vm",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"node": "pve01",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed vm asset: %v", err)
	}
	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-vm-110")
	if err != nil || !ok || target.Kind != "qemu" || target.VMID != "110" {
		t.Fatalf("expected inferred qemu target with vmid 110, got target=%+v ok=%v err=%v", target, ok, err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-",
		Type:    "vm",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"node": "pve01",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed broken vm asset: %v", err)
	}
	_, ok, err = sut.resolveProxmoxSessionTarget("proxmox-vm-")
	if !ok || err == nil || !strings.Contains(err.Error(), "missing vmid metadata") {
		t.Fatalf("expected missing vmid metadata error for broken vm id, got ok=%v err=%v", ok, err)
	}

	withoutAssetStore := newTestAPIServer(t)
	withoutAssetStore.assetStore = nil
	target, ok, err = withoutAssetStore.resolveProxmoxSessionTarget("anything")
	if err != nil || ok || target != (proxmoxSessionTarget{}) {
		t.Fatalf("expected nil asset store to short-circuit, got target=%+v ok=%v err=%v", target, ok, err)
	}
}

func TestResolveProxmoxSessionTargetInferenceBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-ct-200",
		Type:    "container",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed inferred container asset: %v", err)
	}

	target, ok, err := sut.resolveProxmoxSessionTarget("proxmox-ct-200")
	if err != nil || !ok {
		t.Fatalf("expected inferred lxc target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "lxc" || target.VMID != "200" {
		t.Fatalf("expected inferred lxc vmid 200, got %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-fast",
		Type:    "storage-pool",
		Name:    "fast",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"storage_id": "pve02/fast",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed inferred storage asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-fast")
	if err != nil || !ok {
		t.Fatalf("expected inferred storage target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "storage" || target.Node != "pve02" || target.StorageName != "fast" {
		t.Fatalf("unexpected inferred storage target from two-part storage_id: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-inferred",
		Type:    "host",
		Name:    "pve03",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed inferred node asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-node-inferred")
	if err != nil || !ok {
		t.Fatalf("expected inferred node target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Kind != "node" || target.Node != "pve03" {
		t.Fatalf("unexpected inferred node target: %+v", target)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-name-fallback",
		Type:    "storage-pool",
		Name:    "pve04/archive",
		Source:  "proxmox",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed storage name-fallback asset: %v", err)
	}

	target, ok, err = sut.resolveProxmoxSessionTarget("proxmox-storage-name-fallback")
	if err != nil || !ok {
		t.Fatalf("expected storage name-fallback target, got target=%+v ok=%v err=%v", target, ok, err)
	}
	if target.Node != "pve04" || target.StorageName != "archive" {
		t.Fatalf("unexpected storage name fallback parse: %+v", target)
	}
}
