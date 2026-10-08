package main

import (
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"strings"
	"testing"
	"time"
)

func TestParseStorageInsightsWindow(t *testing.T) {
	if got := parseStorageInsightsWindow("7d"); got != 7*24*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(7d) = %s", got)
	}
	if got := parseStorageInsightsWindow("36h"); got != 36*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(36h) = %s", got)
	}
	if got := parseStorageInsightsWindow("1h"); got != 7*24*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(1h) should clamp to fallback 7d, got %s", got)
	}
}

func TestProxmoxStorageAssetBelongsToNode(t *testing.T) {
	assetWithNodeMetadata := assets.Asset{
		Metadata: map[string]string{
			"node":       "pve01",
			"storage_id": "storage/pve01/local-zfs",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetWithNodeMetadata, "pve01") {
		t.Fatalf("expected storage asset with node metadata to belong to pve01")
	}

	assetByStorageID := assets.Asset{
		Metadata: map[string]string{
			"storage_id": "storage/pve02/fast-ssd",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetByStorageID, "pve02") {
		t.Fatalf("expected storage/pve02/fast-ssd to belong to pve02")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetByStorageID, "pve01") {
		t.Fatalf("did not expect storage/pve02/fast-ssd to belong to pve01")
	}

	legacyStorageID := assets.Asset{
		Metadata: map[string]string{
			"storage_id": "pve03/archive",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(legacyStorageID, "pve03") {
		t.Fatalf("expected legacy storage id pve03/archive to belong to pve03")
	}

	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{"node": "pve01"}}, "") {
		t.Fatalf("did not expect empty target node to match")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{}}, "pve01") {
		t.Fatalf("did not expect asset with no node/storage metadata to match")
	}
	prefixOnly := assets.Asset{Metadata: map[string]string{"storage_id": "pve04/archive"}}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(prefixOnly, "pve04") {
		t.Fatalf("expected two-segment storage id to match pve04")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{"storage_id": "local-zfs"}}, "pve01") {
		t.Fatalf("did not expect single-segment storage id to match unrelated node")
	}
}

func TestProxmoxStoragePoolNameFromAsset(t *testing.T) {
	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Metadata: map[string]string{"storage_id": "storage/pve01/local-zfs"},
		Name:     "storage/pve01/wrong",
	}); got != "local-zfs" {
		t.Fatalf("expected pool name from storage_id, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Metadata: map[string]string{"storage_id": "storage/pve01/"},
		Name:     "storage/pve01/fallback",
	}); got != "fallback" {
		t.Fatalf("expected fallback pool name from asset name, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Name: "plain-pool",
	}); got != "plain-pool" {
		t.Fatalf("expected plain name fallback, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{}); got != "" {
		t.Fatalf("expected empty pool name for empty asset, got %q", got)
	}
}

func TestStorageInsightsWindowHelpers(t *testing.T) {
	if got := formatStorageInsightsWindow(48 * time.Hour); got != "2d" {
		t.Fatalf("expected 48h => 2d, got %q", got)
	}
	if got := formatStorageInsightsWindow(90 * time.Minute); got != "1h30m0s" {
		t.Fatalf("expected 90m => 1h30m0s, got %q", got)
	}

	if got := proxmoxpkg.StorageInsightsStep(8 * 24 * time.Hour); got != time.Hour {
		t.Fatalf("expected >=7d window to use 1h step, got %s", got)
	}
	if got := proxmoxpkg.StorageInsightsStep(24 * time.Hour); got != 30*time.Minute {
		t.Fatalf("expected >=24h window to use 30m step, got %s", got)
	}
	if got := proxmoxpkg.StorageInsightsStep(6 * time.Hour); got != 5*time.Minute {
		t.Fatalf("expected short window to use 5m step, got %s", got)
	}
}

func TestProxmoxTaskTimelineStorageRelevanceAndMessages(t *testing.T) {
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{EndTime: 200, StartTime: 100}); got != 200 {
		t.Fatalf("expected EndTime priority, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{StartTime: 150}); got != 150 {
		t.Fatalf("expected StartTime fallback, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{}); got != 0 {
		t.Fatalf("expected empty timeline ts 0, got %d", got)
	}

	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "zfs-scrub"}, false) {
		t.Fatalf("expected zfs task type to always be storage relevant")
	}
	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: ""}, true) {
		t.Fatalf("expected empty task type to be storage relevant when workload mapped")
	}
	if proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "vzdump"}, false) {
		t.Fatalf("did not expect workload task without mapped workload to be storage relevant")
	}
	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "vzdump"}, true) {
		t.Fatalf("expected workload task with mapped workload to be storage relevant")
	}
	if proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "qmstart"}, true) {
		t.Fatalf("did not expect unrelated task type to be storage relevant")
	}

	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "running"}); got != "info" {
		t.Fatalf("expected running severity info, got %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "error"}); got != "critical" {
		t.Fatalf("expected error severity critical, got %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "stopped", ExitStatus: "FAIL"}); got != "critical" {
		t.Fatalf("expected failing exitstatus severity critical, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", Status: "running"}, 101); !strings.Contains(got, "running for VM/CT 101") {
		t.Fatalf("unexpected running message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", ExitStatus: "OK"}, 101); !strings.Contains(got, "completed for VM/CT 101") {
		t.Fatalf("unexpected OK completion message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", ExitStatus: "failed"}, 0); got != "vzdump finished with failed" {
		t.Fatalf("unexpected failure message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", Status: "stopped"}, 0); got != "vzdump status stopped" {
		t.Fatalf("unexpected explicit status message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{}, 0); got != "task completed" {
		t.Fatalf("unexpected default task message: %q", got)
	}
}

func TestProxmoxStoragePoolStateAndNameEdgeBranches(t *testing.T) {
	states := proxmoxpkg.BuildProxmoxStoragePoolStates([]assets.Asset{
		{
			ID:     "empty-pool-name",
			Type:   "storage-pool",
			Source: "proxmox",
			Metadata: map[string]string{
				"storage_id": "storage/pve01/",
			},
		},
	}, proxmoxSessionTarget{Kind: "node", Node: "pve01"}, "")
	if len(states) != 0 {
		t.Fatalf("expected empty pool name asset to be skipped, got %+v", states)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{Name: "pool/"}); got != "pool/" {
		t.Fatalf("expected trailing slash fallback to return original name, got %q", got)
	}
}
