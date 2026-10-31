package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

// Devices that existed before the Hub SSH key setting must keep getting the
// key after the upgrade. Devices added afterwards start with it off.
func TestHubSSHKeyMigrationKeepsExistingDevicesOn(t *testing.T) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping Hub SSH key migration integration test")
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	existing := "hub-ssh-key-existing-" + suffix
	added := "hub-ssh-key-added-" + suffix

	store := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: existing, Name: existing, Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		store.Close()
		t.Fatalf("create existing device: %v", err)
	}
	// Replay the upgrade: the device exists before migration 107 runs.
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `DELETE FROM asset_hub_ssh_keys WHERE asset_id = $1`, existing); err != nil {
		store.Close()
		t.Fatalf("clear existing device setting: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = 107`); err != nil {
		store.Close()
		t.Fatalf("delete migration marker: %v", err)
	}
	store.Close()

	upgraded := mustOpenPostgresStoreForMigrationRecovery(t, dbURL)
	t.Cleanup(upgraded.Close)
	t.Cleanup(func() {
		_ = upgraded.DeleteAsset(existing)
		_ = upgraded.DeleteAsset(added)
	})
	if setting, err := upgraded.GetHubSSHKeySetting(existing); err != nil || !setting.Enabled {
		t.Fatalf("existing device after upgrade=%+v err=%v, want on", setting, err)
	}

	if _, err := upgraded.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: added, Name: added, Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatalf("create new device: %v", err)
	}
	if setting, err := upgraded.GetHubSSHKeySetting(added); err != nil || setting.Enabled || setting.RemovalPending {
		t.Fatalf("new device=%+v err=%v, want off", setting, err)
	}
}
