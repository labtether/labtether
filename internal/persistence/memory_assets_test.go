package persistence

import (
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

func TestMemoryAssetStoreManualNameOverridePersistsAcrossHeartbeats(t *testing.T) {
	store := NewMemoryAssetStore()

	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Initial Name",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "10.0",
		},
	})
	if err != nil {
		t.Fatalf("upsert initial heartbeat: %v", err)
	}

	renamed, err := store.UpdateAsset("node-1", assets.UpdateRequest{
		Name: ptr("Manual Name"),
	})
	if err != nil {
		t.Fatalf("rename asset: %v", err)
	}
	if renamed.Name != "Manual Name" {
		t.Fatalf("renamed name = %q, want %q", renamed.Name, "Manual Name")
	}
	if renamed.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata = %q, want %q", renamed.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}

	updated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Heartbeat Name",
		Source:  "agent",
		Status:  "online",
		Metadata: map[string]string{
			"cpu_percent": "42.0",
		},
	})
	if err != nil {
		t.Fatalf("upsert second heartbeat: %v", err)
	}

	if updated.Name != "Manual Name" {
		t.Fatalf("asset name after heartbeat = %q, want manual override %q", updated.Name, "Manual Name")
	}
	if updated.Metadata[assets.MetadataKeyNameOverride] != "Manual Name" {
		t.Fatalf("name override metadata after heartbeat = %q, want %q", updated.Metadata[assets.MetadataKeyNameOverride], "Manual Name")
	}
	if updated.Metadata["cpu_percent"] != "42.0" {
		t.Fatalf("expected latest heartbeat metadata to be preserved, got cpu_percent=%q", updated.Metadata["cpu_percent"])
	}
}

func TestMemoryAssetStoreGroupPersistsAcrossHeartbeatsWithoutGroupID(t *testing.T) {
	store := NewMemoryAssetStore()

	_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Initial Name",
		Source:  "agent",
		GroupID: "group-a",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("upsert initial heartbeat: %v", err)
	}

	updated, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		Type:    "host",
		Name:    "Refreshed Name",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("upsert second heartbeat: %v", err)
	}

	if updated.GroupID != "group-a" {
		t.Fatalf("group_id after heartbeat = %q, want %q", updated.GroupID, "group-a")
	}
}

func ptr(value string) *string {
	return &value
}
