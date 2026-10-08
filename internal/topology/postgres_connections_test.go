package topology

import (
	"testing"
)

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
