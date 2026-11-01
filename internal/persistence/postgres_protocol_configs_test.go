package persistence

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/protocols"
)

var protocolConfigUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The Connect panel could not save any protocol on PostgreSQL: the Hub sent
// its own "proto_..." ID into the UUID id column, and an empty test status
// the table refuses. PostgreSQL must assign the ID, and saving again must
// update the same row.
func TestSaveProtocolConfigOnPostgres(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	assetID := "protocol-config-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: assetID, Name: assetID, Type: "host", Source: "manual", Status: "online",
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteAsset(assetID) })

	created := &protocols.ProtocolConfig{AssetID: assetID, Protocol: protocols.ProtocolSSH, Host: "192.0.2.10", Port: 22, Username: "operator", Enabled: true}
	if err := store.SaveProtocolConfig(ctx, created); err != nil {
		t.Fatalf("create SSH config: %v", err)
	}
	if !protocolConfigUUIDPattern.MatchString(created.ID) {
		t.Fatalf("id %q is not a database UUID", created.ID)
	}

	updated := &protocols.ProtocolConfig{AssetID: assetID, Protocol: protocols.ProtocolSSH, Host: "192.0.2.11", Port: 2222, Username: "operator", Enabled: true}
	if err := store.SaveProtocolConfig(ctx, updated); err != nil {
		t.Fatalf("update SSH config: %v", err)
	}
	stored, err := store.GetProtocolConfig(ctx, assetID, protocols.ProtocolSSH)
	if err != nil || stored == nil {
		t.Fatalf("load SSH config: %v", err)
	}
	if updated.ID != created.ID || stored.Host != "192.0.2.11" || stored.Port != 2222 || stored.TestStatus != "untested" {
		t.Fatalf("after update: id=%q (was %q) stored=%+v", updated.ID, created.ID, stored)
	}
}
