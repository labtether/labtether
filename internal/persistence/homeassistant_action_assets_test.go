package persistence

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

func TestMemoryHomeAssistantActionAssets(t *testing.T) {
	s := NewMemoryAssetStore()
	checkHomeAssistantActionAssetPages(t, s, s, "ha-memory-")
}

// Real SQL must retain both source aliases, exact collector/entity ownership
// and byte-order continuation. This is never allowed to migrate a shared DB.
func TestPostgresHomeAssistantActionAssets(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM assets`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("HA query fixture requires empty owned asset table: %d/%v", count, err)
	}
	prefix := "ha-query-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	checkHomeAssistantActionAssetPages(t, s, s, prefix)
}

func checkHomeAssistantActionAssetPages(t *testing.T, store AssetStore, lookup HomeAssistantActionAssetStore, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var owned []string
	t.Cleanup(func() {
		if pg, ok := store.(*PostgresStore); ok {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if _, err := pg.pool.Exec(cleanupCtx, `DELETE FROM assets WHERE id=ANY($1::text[])`, owned); err != nil {
				t.Errorf("delete exact HA fixture assets: %v", err)
			}
			var remains int
			if err := pg.pool.QueryRow(cleanupCtx, `SELECT (SELECT COUNT(*) FROM assets WHERE id=ANY($1::text[]))+
			 (SELECT COUNT(*) FROM asset_heartbeats WHERE asset_id=ANY($1::text[]))`, owned).Scan(&remains); err != nil || remains != 0 {
				t.Errorf("owned HA query cleanup incomplete: %d/%v", remains, err)
			}
			return
		}
		for _, id := range owned {
			if err := store.DeleteAsset(id); err != nil {
				t.Errorf("delete exact HA fixture %s: %v", id, err)
			}
		}
		for _, id := range owned {
			if _, found, err := store.GetAsset(id); err != nil || found {
				t.Errorf("HA fixture remains %s: %v/%v", id, found, err)
			}
		}
	})
	collector, foreign, entity := prefix+"A", prefix+"B", "light.room_2"
	seed := func(id, source, kind, owner, rawEntity string) string {
		t.Helper()
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, prefix) {
			t.Fatal("HA fixture escaped reserved namespace")
		}
		if _, found, err := store.GetAsset(id); err != nil || found {
			t.Fatalf("HA fixture collision %s: %v", id, err)
		}
		owned = append(owned, id)
		_, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: kind, Source: source, Name: id,
			Metadata: map[string]string{"collector_id": owner, "entity_id": rawEntity}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	var want, exact []string
	for i := 0; i < 130; i++ {
		source, rawEntity := "homeassistant", fmt.Sprintf("sensor.sample_%03d", i)
		if i%2 == 0 {
			source = "home-assistant"
		}
		if i == 129 {
			rawEntity = entity
		}
		id := seed(fmt.Sprintf("%srow-%03d", prefix, i), source, "ha-entity", " "+collector+" ", rawEntity)
		want = append(want, id)
		if rawEntity == entity {
			exact = append(exact, id)
		}
	}
	for _, suffix := range []string{"!", "A", "_", "a", "é", "中"} {
		id := seed(prefix+suffix, " HOME-ASSISTANT ", "ha-entity", collector, " "+entity+" ")
		want, exact = append(want, id), append(exact, id)
	}
	for _, id := range []string{prefix + "legacy", assetid.ScopeCollectorAssetID(prefix+"unowned", collector)} {
		id = seed(id, "homeassistant", "ha-entity", "", entity)
		want, exact = append(want, id), append(exact, id)
	}
	for _, row := range []struct{ id, source, kind, owner, raw string }{
		{prefix + "foreign", "homeassistant", "ha-entity", foreign, entity},
		{assetid.ScopeCollectorAssetID(prefix+"foreign-scope", foreign), "home-assistant", "ha-entity", "", entity},
		{prefix + "wrong-source", "truenas", "ha-entity", collector, entity},
		{prefix + "parent", "homeassistant", "connector-cluster", collector, entity},
	} {
		seed(row.id, row.source, row.kind, row.owner, row.raw)
	}
	read := func(raw string) []string {
		t.Helper()
		var ids []string
		after := ""
		for page := 0; page < 20; page++ {
			rows, err := lookup.ListHomeAssistantActionAssets(ctx, collector, raw, after, 17)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.ID <= after {
					t.Fatalf("cursor did not advance in byte order: %q after %q", row.ID, after)
				}
				ids, after = append(ids, row.ID), row.ID
			}
			if len(rows) < 17 {
				return ids
			}
		}
		t.Fatal("bounded HA page fixture did not finish")
		return nil
	}
	sort.Strings(want)
	sort.Strings(exact)
	if got := read(""); !reflect.DeepEqual(got, want) {
		t.Fatalf("all owner pages=%v want=%v", got, want)
	}
	if got := read(entity); !reflect.DeepEqual(got, exact) {
		t.Fatalf("raw entity owners=%v want=%v", got, exact)
	}
}
