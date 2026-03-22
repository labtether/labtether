package topology

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) (*PostgresStore, *persistence.PostgresStore) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		t.Skip("DATABASE_URL not set")
	}
	mainStore, err := persistence.NewPostgresStore(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { mainStore.Close() })
	return NewPostgresStore(mainStore.Pool()), mainStore
}

func createTestAsset(t *testing.T, mainStore *persistence.PostgresStore, suffix string) assets.Asset {
	t.Helper()
	ts := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	assetID := "topo-test-" + suffix + "-" + ts

	asset, err := mainStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  assetID,
		Type:     "server",
		Name:     "Topo Test " + suffix,
		Source:   "test",
		Status:   "online",
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("createTestAsset(%s) failed: %v", suffix, err)
	}
	t.Cleanup(func() {
		_ = mainStore.DeleteAsset(asset.ID)
	})
	return asset
}

// getOrCreateTestLayout is a helper that ensures a layout exists and cleans it up.
func getOrCreateTestLayout(t *testing.T, store *PostgresStore) Layout {
	t.Helper()
	layout, err := store.GetOrCreateLayout()
	if err != nil {
		t.Fatalf("GetOrCreateLayout: %v", err)
	}
	return layout
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGetOrCreateLayout(t *testing.T) {
	store, _ := newTestStore(t)

	// First call: creates or retrieves layout.
	layout1, err := store.GetOrCreateLayout()
	if err != nil {
		t.Fatalf("GetOrCreateLayout (1st): %v", err)
	}
	if layout1.ID == "" {
		t.Fatal("expected non-empty layout ID")
	}
	if layout1.Name == "" {
		t.Fatal("expected non-empty layout name")
	}

	// Second call: should return the same layout.
	layout2, err := store.GetOrCreateLayout()
	if err != nil {
		t.Fatalf("GetOrCreateLayout (2nd): %v", err)
	}
	if layout1.ID != layout2.ID {
		t.Fatalf("expected same layout ID, got %q vs %q", layout1.ID, layout2.ID)
	}

	// Update viewport and verify.
	newVP := Viewport{X: 100, Y: 200, Zoom: 1.5}
	if err := store.UpdateViewport(newVP); err != nil {
		t.Fatalf("UpdateViewport: %v", err)
	}
	layout3, err := store.GetOrCreateLayout()
	if err != nil {
		t.Fatalf("GetOrCreateLayout (3rd): %v", err)
	}
	if layout3.Viewport.X != 100 || layout3.Viewport.Y != 200 || layout3.Viewport.Zoom != 1.5 {
		t.Fatalf("viewport mismatch: %+v", layout3.Viewport)
	}
}

func TestZoneCRUD(t *testing.T) {
	store, _ := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	// Create zone.
	zone, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "DMZ",
		Color:      "red",
		Icon:       "shield",
		Position:   Position{X: 10, Y: 20},
		Size:       Size{Width: 400, Height: 300},
		SortOrder:  1,
	})
	if err != nil {
		t.Fatalf("CreateZone: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteZone(zone.ID)
	})

	if zone.ID == "" {
		t.Fatal("expected non-empty zone ID")
	}
	if zone.Label != "DMZ" {
		t.Fatalf("expected label DMZ, got %q", zone.Label)
	}
	if zone.Color != "red" {
		t.Fatalf("expected color red, got %q", zone.Color)
	}
	if zone.Position.X != 10 || zone.Position.Y != 20 {
		t.Fatalf("position mismatch: %+v", zone.Position)
	}
	if zone.Size.Width != 400 || zone.Size.Height != 300 {
		t.Fatalf("size mismatch: %+v", zone.Size)
	}

	// List zones.
	zones, err := store.ListZones(layout.ID)
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	found := false
	for _, z := range zones {
		if z.ID == zone.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created zone not found in ListZones")
	}

	// Update zone.
	zone.Label = "Perimeter"
	zone.Color = "orange"
	zone.Position = Position{X: 50, Y: 60}
	if err := store.UpdateZone(zone); err != nil {
		t.Fatalf("UpdateZone: %v", err)
	}
	zones, err = store.ListZones(layout.ID)
	if err != nil {
		t.Fatalf("ListZones after update: %v", err)
	}
	for _, z := range zones {
		if z.ID == zone.ID {
			if z.Label != "Perimeter" {
				t.Fatalf("expected label Perimeter, got %q", z.Label)
			}
			if z.Color != "orange" {
				t.Fatalf("expected color orange, got %q", z.Color)
			}
			if z.Position.X != 50 || z.Position.Y != 60 {
				t.Fatalf("updated position mismatch: %+v", z.Position)
			}
		}
	}

	// Delete zone.
	if err := store.DeleteZone(zone.ID); err != nil {
		t.Fatalf("DeleteZone: %v", err)
	}
	zones, err = store.ListZones(layout.ID)
	if err != nil {
		t.Fatalf("ListZones after delete: %v", err)
	}
	for _, z := range zones {
		if z.ID == zone.ID {
			t.Fatal("zone should have been deleted")
		}
	}
}

func TestZoneNesting(t *testing.T) {
	store, _ := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	// Create parent zone.
	parent, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "Parent",
		Color:      "blue",
		Position:   Position{X: 0, Y: 0},
		Size:       Size{Width: 500, Height: 400},
	})
	if err != nil {
		t.Fatalf("CreateZone (parent): %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteZone(parent.ID)
	})

	// Create child zone under parent.
	child, err := store.CreateZone(Zone{
		TopologyID:   layout.ID,
		ParentZoneID: parent.ID,
		Label:        "Child",
		Color:        "green",
		Position:     Position{X: 10, Y: 10},
		Size:         Size{Width: 200, Height: 150},
	})
	if err != nil {
		t.Fatalf("CreateZone (child): %v", err)
	}
	t.Cleanup(func() {
		_ = store.DeleteZone(child.ID)
	})

	if child.ParentZoneID != parent.ID {
		t.Fatalf("expected child parent_zone_id=%s, got %s", parent.ID, child.ParentZoneID)
	}

	// Delete parent -> child should re-parent to nil.
	if err := store.DeleteZone(parent.ID); err != nil {
		t.Fatalf("DeleteZone (parent): %v", err)
	}

	zones, err := store.ListZones(layout.ID)
	if err != nil {
		t.Fatalf("ListZones after parent delete: %v", err)
	}
	for _, z := range zones {
		if z.ID == child.ID {
			if z.ParentZoneID != "" {
				t.Fatalf("expected child parent_zone_id to be empty after parent deletion, got %q", z.ParentZoneID)
			}
			return
		}
	}
	t.Fatal("child zone not found after parent deletion")
}

func TestMemberOperations(t *testing.T) {
	store, mainStore := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	a1 := createTestAsset(t, mainStore, "member1")
	a2 := createTestAsset(t, mainStore, "member2")

	// Create two zones.
	zone1, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "Zone A",
		Color:      "blue",
		Position:   Position{X: 0, Y: 0},
		Size:       Size{Width: 300, Height: 200},
	})
	if err != nil {
		t.Fatalf("CreateZone (zone1): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteZone(zone1.ID) })

	zone2, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "Zone B",
		Color:      "green",
		Position:   Position{X: 400, Y: 0},
		Size:       Size{Width: 300, Height: 200},
	})
	if err != nil {
		t.Fatalf("CreateZone (zone2): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteZone(zone2.ID) })

	// Set members in zone1.
	err = store.SetMembers(zone1.ID, []ZoneMember{
		{ZoneID: zone1.ID, AssetID: a1.ID, Position: Position{X: 5, Y: 5}, SortOrder: 0},
		{ZoneID: zone1.ID, AssetID: a2.ID, Position: Position{X: 50, Y: 50}, SortOrder: 1},
	})
	if err != nil {
		t.Fatalf("SetMembers (zone1): %v", err)
	}

	members, err := store.ListMembers(layout.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// Move a1 to zone2 (single-zone constraint enforced by SetMembers).
	err = store.SetMembers(zone2.ID, []ZoneMember{
		{ZoneID: zone2.ID, AssetID: a1.ID, Position: Position{X: 10, Y: 10}, SortOrder: 0},
	})
	if err != nil {
		t.Fatalf("SetMembers (zone2): %v", err)
	}

	// Verify: a1 should be in zone2, a2 should be in zone1.
	members, err = store.ListMembers(layout.ID)
	if err != nil {
		t.Fatalf("ListMembers after move: %v", err)
	}

	memberMap := make(map[string]string) // asset_id -> zone_id
	for _, m := range members {
		memberMap[m.AssetID] = m.ZoneID
	}
	if memberMap[a1.ID] != zone2.ID {
		t.Fatalf("expected a1 in zone2, got zone %q", memberMap[a1.ID])
	}
	if memberMap[a2.ID] != zone1.ID {
		t.Fatalf("expected a2 in zone1, got zone %q", memberMap[a2.ID])
	}

	// Remove member.
	if err := store.RemoveMember(a2.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	members, err = store.ListMembers(layout.ID)
	if err != nil {
		t.Fatalf("ListMembers after remove: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member after remove, got %d", len(members))
	}
	if members[0].AssetID != a1.ID {
		t.Fatalf("expected remaining member to be a1, got %q", members[0].AssetID)
	}
}

func TestConnectionCRUD(t *testing.T) {
	store, mainStore := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	a1 := createTestAsset(t, mainStore, "conn-src")
	a2 := createTestAsset(t, mainStore, "conn-tgt")

	// Create connection.
	conn, err := store.CreateConnection(Connection{
		TopologyID:    layout.ID,
		SourceAssetID: a1.ID,
		TargetAssetID: a2.ID,
		Relationship:  "depends_on",
		UserDefined:   true,
		Label:         "primary link",
	})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if conn.ID == "" {
		t.Fatal("expected non-empty connection ID")
	}
	if conn.Relationship != "depends_on" {
		t.Fatalf("expected relationship depends_on, got %q", conn.Relationship)
	}
	if conn.Label != "primary link" {
		t.Fatalf("expected label 'primary link', got %q", conn.Label)
	}
	if conn.Deleted {
		t.Fatal("expected connection to not be deleted")
	}

	// Update relationship.
	if err := store.UpdateConnection(conn.ID, "runs_on", ""); err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}

	conns, err := store.ListConnections(layout.ID)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	var updated *Connection
	for _, c := range conns {
		if c.ID == conn.ID {
			updated = &c
			break
		}
	}
	if updated == nil {
		t.Fatal("connection not found after update")
	}
	if updated.Relationship != "runs_on" {
		t.Fatalf("expected updated relationship runs_on, got %q", updated.Relationship)
	}
	if updated.Label != "primary link" {
		t.Fatalf("expected label unchanged, got %q", updated.Label)
	}

	// Soft-delete.
	if err := store.DeleteConnection(conn.ID); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}

	// List should include soft-deleted connections.
	conns, err = store.ListConnections(layout.ID)
	if err != nil {
		t.Fatalf("ListConnections after delete: %v", err)
	}
	var softDeleted *Connection
	for _, c := range conns {
		if c.ID == conn.ID {
			softDeleted = &c
			break
		}
	}
	if softDeleted == nil {
		t.Fatal("soft-deleted connection should still appear in ListConnections")
	}
	if !softDeleted.Deleted {
		t.Fatal("expected connection to be marked deleted")
	}

	// Double-delete should fail.
	if err := store.DeleteConnection(conn.ID); err == nil {
		t.Fatal("expected error on double-delete, got nil")
	}

	// Re-create after soft-delete should re-activate.
	reactivated, err := store.CreateConnection(Connection{
		TopologyID:    layout.ID,
		SourceAssetID: a1.ID,
		TargetAssetID: a2.ID,
		Relationship:  "runs_on", // same relationship as the updated version
		UserDefined:   true,
		Label:         "reactivated",
	})
	if err != nil {
		t.Fatalf("CreateConnection (re-activate): %v", err)
	}
	if reactivated.ID != conn.ID {
		t.Fatalf("expected re-activated connection to have same ID %q, got %q", conn.ID, reactivated.ID)
	}
	if reactivated.Deleted {
		t.Fatal("expected re-activated connection to not be deleted")
	}
	if reactivated.Label != "reactivated" {
		t.Fatalf("expected re-activated label 'reactivated', got %q", reactivated.Label)
	}

	// Cleanup.
	_ = store.DeleteConnection(conn.ID)
}

func TestConnectionUniqueConstraint(t *testing.T) {
	store, mainStore := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	a1 := createTestAsset(t, mainStore, "uniq-src")
	a2 := createTestAsset(t, mainStore, "uniq-tgt")

	// Create first connection.
	conn1, err := store.CreateConnection(Connection{
		TopologyID:    layout.ID,
		SourceAssetID: a1.ID,
		TargetAssetID: a2.ID,
		Relationship:  "depends_on",
		UserDefined:   true,
	})
	if err != nil {
		t.Fatalf("CreateConnection (1st): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteConnection(conn1.ID) })

	// Same source+target+type should be rejected by the partial unique index.
	_, err = store.CreateConnection(Connection{
		TopologyID:    layout.ID,
		SourceAssetID: a1.ID,
		TargetAssetID: a2.ID,
		Relationship:  "depends_on",
		UserDefined:   true,
	})
	if err == nil {
		t.Fatal("expected error for duplicate connection, got nil")
	}

	// Different relationship type should be allowed.
	conn2, err := store.CreateConnection(Connection{
		TopologyID:    layout.ID,
		SourceAssetID: a1.ID,
		TargetAssetID: a2.ID,
		Relationship:  "connected_to",
		UserDefined:   true,
	})
	if err != nil {
		t.Fatalf("CreateConnection (different type): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteConnection(conn2.ID) })

	if conn2.Relationship != "connected_to" {
		t.Fatalf("expected relationship connected_to, got %q", conn2.Relationship)
	}
}

func TestDismissedAssets(t *testing.T) {
	store, mainStore := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	a1 := createTestAsset(t, mainStore, "dismiss1")
	a2 := createTestAsset(t, mainStore, "dismiss2")

	// Dismiss assets.
	if err := store.DismissAsset(layout.ID, a1.ID); err != nil {
		t.Fatalf("DismissAsset (a1): %v", err)
	}
	if err := store.DismissAsset(layout.ID, a2.ID); err != nil {
		t.Fatalf("DismissAsset (a2): %v", err)
	}

	// Double dismiss should not error (ON CONFLICT DO NOTHING).
	if err := store.DismissAsset(layout.ID, a1.ID); err != nil {
		t.Fatalf("DismissAsset (a1 duplicate): %v", err)
	}

	// List dismissed.
	dismissed, err := store.ListDismissed(layout.ID)
	if err != nil {
		t.Fatalf("ListDismissed: %v", err)
	}
	found := map[string]bool{}
	for _, id := range dismissed {
		found[id] = true
	}
	if !found[a1.ID] || !found[a2.ID] {
		t.Fatalf("expected both assets dismissed, got %v", dismissed)
	}

	// Undismiss a1.
	if err := store.UndismissAsset(layout.ID, a1.ID); err != nil {
		t.Fatalf("UndismissAsset: %v", err)
	}

	dismissed, err = store.ListDismissed(layout.ID)
	if err != nil {
		t.Fatalf("ListDismissed after undismiss: %v", err)
	}
	for _, id := range dismissed {
		if id == a1.ID {
			t.Fatal("a1 should no longer be dismissed")
		}
	}

	// Undismiss non-dismissed asset should return ErrNotFound.
	if err := store.UndismissAsset(layout.ID, a1.ID); err == nil {
		t.Fatal("expected error undismissing non-dismissed asset, got nil")
	}

	// Cleanup.
	_ = store.UndismissAsset(layout.ID, a2.ID)
}

func TestReorderZones(t *testing.T) {
	store, _ := newTestStore(t)
	layout := getOrCreateTestLayout(t, store)

	z1, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "First",
		Color:      "blue",
		SortOrder:  0,
		Position:   Position{X: 0, Y: 0},
		Size:       Size{Width: 100, Height: 100},
	})
	if err != nil {
		t.Fatalf("CreateZone z1: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteZone(z1.ID) })

	z2, err := store.CreateZone(Zone{
		TopologyID: layout.ID,
		Label:      "Second",
		Color:      "green",
		SortOrder:  1,
		Position:   Position{X: 0, Y: 0},
		Size:       Size{Width: 100, Height: 100},
	})
	if err != nil {
		t.Fatalf("CreateZone z2: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteZone(z2.ID) })

	// Reorder: swap and nest z2 under z1.
	err = store.ReorderZones([]ZoneReorder{
		{ZoneID: z1.ID, SortOrder: 1},
		{ZoneID: z2.ID, ParentZoneID: z1.ID, SortOrder: 0},
	})
	if err != nil {
		t.Fatalf("ReorderZones: %v", err)
	}

	zones, err := store.ListZones(layout.ID)
	if err != nil {
		t.Fatalf("ListZones after reorder: %v", err)
	}

	for _, z := range zones {
		if z.ID == z2.ID {
			if z.ParentZoneID != z1.ID {
				t.Fatalf("expected z2 parent to be z1, got %q", z.ParentZoneID)
			}
			if z.SortOrder != 0 {
				t.Fatalf("expected z2 sort_order 0, got %d", z.SortOrder)
			}
		}
		if z.ID == z1.ID {
			if z.SortOrder != 1 {
				t.Fatalf("expected z1 sort_order 1, got %d", z.SortOrder)
			}
		}
	}
}
