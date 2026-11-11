package persistence

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/assets"
)

// Migration 111 turns stored device names (and kept renames) into one line,
// the way names are stored from now on, and leaves plain names alone.
func TestDeviceNamesMigrationMakesStoredNamesOneLine(t *testing.T) {
	store := newTestPostgresStore(t)
	bad := createTestAsset(t, store, "badname")
	plain := createTestAsset(t, store, "plainname")
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `UPDATE assets SET name = $1, metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{name_override}', to_jsonb($1::text)) WHERE id = $2`,
		"nas01\r\n\tBcc:  x@example.com ", bad.ID); err != nil {
		t.Fatalf("store a bad name: %v", err)
	}
	for _, statement := range schemaMigrationDeviceNamesOneLine().Statements {
		if _, err := store.pool.Exec(ctx, statement); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	want := assets.OneLineName("nas01\r\n\tBcc:  x@example.com ")
	got, _, err := store.GetAsset(bad.ID)
	if err != nil || got.Name != want || got.Metadata[assets.MetadataKeyNameOverride] != want {
		t.Fatalf("after migration name=%q override=%q (err %v), want %q", got.Name, got.Metadata[assets.MetadataKeyNameOverride], err, want)
	}
	if kept, _, _ := store.GetAsset(plain.ID); kept.Name != plain.Name {
		t.Fatalf("plain name changed to %q", kept.Name)
	}
}
