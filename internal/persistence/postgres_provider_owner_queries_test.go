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
	"github.com/labtether/labtether/internal/hubcollector"
)

// Memory fixtures cannot prove the SQL owner filters or that a cursor uses the
// same byte order as the caller. This runs only in a job-stamped disposable DB.
func TestPostgresProviderOwnerQueries(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var existingAssets, existingCollectors int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM assets),
	 (SELECT COUNT(*) FROM hub_collectors)`).Scan(&existingAssets, &existingCollectors); err != nil || existingAssets != 0 || existingCollectors != 0 {
		t.Fatalf("provider query fixture requires empty owned tables: assets=%d collectors=%d err=%v", existingAssets, existingCollectors, err)
	}
	prefix := "provider-query-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	var ownedIDs []string
	// Register cleanup before the first write. Only exact reserved asset IDs
	// and collectors attached to those IDs may be removed, even on failure.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if len(ownedIDs) == 0 {
			return
		}
		if _, err := s.pool.Exec(cleanupCtx, `DELETE FROM hub_collectors WHERE asset_id=ANY($1::text[])`, ownedIDs); err != nil {
			t.Errorf("remove owned collectors: %v", err)
		}
		if _, err := s.pool.Exec(cleanupCtx, `DELETE FROM assets WHERE id=ANY($1::text[])`, ownedIDs); err != nil {
			t.Errorf("remove owned assets: %v", err)
		}
		var remains int
		if err := s.pool.QueryRow(cleanupCtx, `SELECT (SELECT COUNT(*) FROM assets WHERE id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM hub_collectors WHERE asset_id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM asset_heartbeats WHERE asset_id=ANY($1::text[]))`, ownedIDs).Scan(&remains); err != nil || remains != 0 {
			t.Errorf("owned fixture cleanup incomplete: %d/%v", remains, err)
		}
	})
	seed := func(t *testing.T, id, kind, source, name string, metadata map[string]string) string {
		t.Helper()
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, prefix) {
			t.Fatal("fixture ID escaped reserved namespace")
		}
		if _, found, err := s.GetAsset(id); err != nil || found {
			t.Fatalf("fixture asset collision: %s/%v", id, err)
		}
		ownedIDs = append(ownedIDs, id)
		asset, err := s.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: kind, Source: source, Name: name, Status: "online", Metadata: metadata})
		if err != nil || asset.ID != id {
			t.Fatalf("seed asset %s: %v", id, err)
		}
		return id
	}
	ids := func(rows []assets.Asset) []string {
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.ID)
		}
		sort.Strings(out)
		return out
	}
	assertIDs := func(t *testing.T, rows []assets.Asset, err error, want ...string) {
		t.Helper()
		sort.Strings(want)
		if err != nil || !reflect.DeepEqual(ids(rows), want) {
			t.Fatalf("owner rows=%v want=%v err=%v", ids(rows), want, err)
		}
	}
	collector, foreign := prefix+"owner", prefix+"foreign"

	t.Run("PBS exact store and bounded aliases", func(t *testing.T) {
		server := seed(t, prefix+"pbs-server", "storage-controller", "pbs", "PBS", map[string]string{"collector_id": collector})
		seed(t, prefix+"pbs-other-server", "storage-controller", "pbs", "other", map[string]string{"collector_id": foreign})
		rows, err := s.FindPBSActionAssets(collector, "storage-controller", "")
		assertIDs(t, rows, err, server)
		store := "Backup.Main"
		own := seed(t, assetid.ScopeCollectorAssetID(prefix+"pbs-data", collector), "storage-pool", "pbs", store, map[string]string{"collector_id": " " + collector + " ", "store": store})
		legacy := seed(t, prefix+"pbs-legacy", "storage-pool", "pbs", store, map[string]string{"store": store})
		for _, item := range []struct{ id, kind, source, owner, store string }{
			{prefix + "pbs-foreign", "storage-pool", "pbs", foreign, store},
			{assetid.ScopeCollectorAssetID(prefix+"pbs-foreign-scope", foreign), "storage-pool", "pbs", "", store},
			{prefix + "pbs-wrong-source", "storage-pool", "truenas", collector, store},
			{prefix + "pbs-wrong-kind", "vm", "pbs", collector, store},
			{prefix + "pbs-lossy-name", "storage-pool", "pbs", collector, "backup-main"},
		} {
			seed(t, item.id, item.kind, item.source, item.store, map[string]string{"collector_id": item.owner, "store": item.store})
		}
		rows, err = s.FindPBSActionAssets(collector, "storage-pool", store)
		assertIDs(t, rows, err, own, legacy)
		// A namespace without metadata is still a candidate; the caller checks
		// its exact scope. A third row must expose ambiguity, never hide it.
		seed(t, assetid.ScopeCollectorAssetID(prefix+"pbs-unowned-scope", collector), "storage-pool", "pbs", store, map[string]string{"store": store})
		seed(t, prefix+"pbs-fourth", "storage-pool", "pbs", store, map[string]string{"collector_id": collector, "store": store})
		rows, err = s.FindPBSActionAssets(collector, "storage-pool", store)
		if err != nil || len(rows) != 3 {
			t.Fatalf("PBS ambiguity sentinel=%d/%v", len(rows), err)
		}
	})

	t.Run("TrueNAS exact share ID protocol and collector", func(t *testing.T) {
		own := seed(t, prefix+"smb-renamed", "share-smb", "truenas", "new display name", map[string]string{"collector_id": collector, "share_id": "7"})
		legacy := seed(t, assetid.ScopeCollectorAssetID(prefix+"smb-scoped", collector), "share-smb", "truenas", "alias", map[string]string{"share_id": "7"})
		for _, item := range []struct{ id, kind, source, owner, share string }{
			{prefix + "smb-foreign", "share-smb", "truenas", foreign, "7"},
			{assetid.ScopeCollectorAssetID(prefix+"smb-foreign-scope", foreign), "share-smb", "truenas", "", "7"},
			{prefix + "smb-other-id", "share-smb", "truenas", collector, "8"},
			{prefix + "nfs-same-id", "share-nfs", "truenas", collector, "7"},
			{prefix + "other-source-share", "share-smb", "pbs", collector, "7"},
		} {
			seed(t, item.id, item.kind, item.source, "share", map[string]string{"collector_id": item.owner, "share_id": item.share})
		}
		rows, err := s.FindTrueNASShareAssets(ctx, collector, "smb", 7)
		assertIDs(t, rows, err, own, legacy)
		seed(t, prefix+"smb-third", "share-smb", "truenas", "legacy", map[string]string{"share_id": "7"})
		seed(t, prefix+"smb-fourth", "share-smb", "truenas", "extra", map[string]string{"collector_id": collector, "share_id": "7"})
		rows, err = s.FindTrueNASShareAssets(ctx, collector, "smb", 7)
		if err != nil || len(rows) != 3 {
			t.Fatalf("share ambiguity sentinel=%d/%v", len(rows), err)
		}
	})

	t.Run("TrueNAS byte-order cursor crosses page boundary", func(t *testing.T) {
		var collation string
		if err := s.pool.QueryRow(ctx, `SELECT datcollate FROM pg_database WHERE datname=current_database()`).Scan(&collation); err != nil {
			t.Fatal(err)
		}
		t.Logf("actual database collation: %s; query must return C byte order", collation)
		var want []string
		for i := 0; i < 130; i++ {
			name := fmt.Sprintf("tank/data/child-%03d", i)
			want = append(want, seed(t, fmt.Sprintf("%sdataset-%03d", prefix, i), "dataset", "truenas", name, map[string]string{"collector_id": collector, "dataset_name": name}))
		}
		for i, suffix := range []string{"!", "-", "A", "_", "a", "é", "中"} {
			name := fmt.Sprintf("tank/data/special-%d", i)
			want = append(want, seed(t, prefix+"dataset-"+suffix, "dataset", "truenas", name, map[string]string{"collector_id": collector, "dataset_name": name}))
		}
		want = append(want, seed(t, prefix+"dataset-root", "dataset", "truenas", "tank/data", map[string]string{"collector_id": collector, "dataset_name": "tank/data"}))
		want = append(want, seed(t, assetid.ScopeCollectorAssetID(prefix+"dataset-scoped", collector), "dataset", "truenas", "tank/data/scoped", nil))
		want = append(want, seed(t, prefix+"dataset-legacy", "dataset", "truenas", "tank/data/legacy", nil))
		// A renamed legacy row is returned as ambiguous for the route to reject;
		// SQL must not silently omit its maintenance group based on display name.
		want = append(want, seed(t, prefix+"dataset-renamed", "dataset", "truenas", "friendly", map[string]string{"name_override": "friendly"}))
		for _, item := range []struct{ id, kind, source, owner, name string }{
			{prefix + "dataset-foreign", "dataset", "truenas", foreign, "tank/data/x"},
			{assetid.ScopeCollectorAssetID(prefix+"dataset-foreign-scope", foreign), "dataset", "truenas", "", "tank/data/x"},
			{prefix + "dataset-sibling", "dataset", "truenas", collector, "tank/database"},
			{prefix + "dataset-wrong-source", "dataset", "pbs", collector, "tank/data/x"},
			{prefix + "dataset-wrong-kind", "storage-pool", "truenas", collector, "tank/data/x"},
		} {
			seed(t, item.id, item.kind, item.source, item.name, map[string]string{"collector_id": item.owner, "dataset_name": item.name})
		}
		sort.Strings(want)
		var got []string
		after := ""
		for page := 0; page < 4; page++ {
			rows, err := s.ListTrueNASDatasetAssets(ctx, collector, "tank/data", after, 128)
			if err != nil || len(rows) > 128 {
				t.Fatalf("dataset page=%d count=%d err=%v", page, len(rows), err)
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				if row.ID <= after {
					t.Fatalf("cursor repeated or reversed: %q after %q", row.ID, after)
				}
				got = append(got, row.ID)
				after = row.ID
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("paged dataset owners differ: got %v want %v", got, want)
		}
	})

	t.Run("collector uniqueness beyond unrelated first page", func(t *testing.T) {
		create := func(suffix, kind string, enabled bool) hubcollector.Collector {
			t.Helper()
			id := seed(t, prefix+"collector-host-"+suffix, "host", "fixture", suffix, nil)
			row, err := s.CreateHubCollector(hubcollector.CreateCollectorRequest{AssetID: id, CollectorType: kind, Enabled: &enabled})
			if err != nil {
				t.Fatal(err)
			}
			return row
		}
		create("disabled-pbs", "pbs", false)
		first := create("hidden-pbs", "pbs", true)
		for i := 0; i < 201; i++ {
			create(fmt.Sprintf("unrelated-%03d", i), "docker", true)
		}
		assertHidden := func(hidden ...string) {
			t.Helper()
			// Place the relevant owners behind the old global inventory page;
			// a regression to ListHubCollectors(200,true) must miss them.
			if _, err := s.pool.Exec(ctx, `UPDATE hub_collectors SET updated_at=$1 WHERE id=ANY($2::text[])`, first.CreatedAt.Add(-time.Hour), hidden); err != nil {
				t.Fatal(err)
			}
			page, err := s.ListHubCollectors(200, true)
			if err != nil || len(page) != 200 {
				t.Fatalf("global page precondition=%d/%v", len(page), err)
			}
			for _, row := range page {
				if row.CollectorType != "docker" {
					t.Fatal("provider owner unexpectedly visible in old global page")
				}
			}
		}
		assertHidden(first.ID)
		rows, err := s.FindEnabledHubCollectorsByType("pbs")
		if err != nil || len(rows) != 1 || rows[0].ID != first.ID {
			t.Fatalf("unique late collector=%+v/%v", rows, err)
		}
		second := create("second-pbs", "pbs", true)
		assertHidden(first.ID, second.ID)
		rows, err = s.FindEnabledHubCollectorsByType("pbs")
		if err != nil || len(rows) != 2 {
			t.Fatalf("ambiguous collector count=%d/%v", len(rows), err)
		}
		got := []string{rows[0].ID, rows[1].ID}
		want := []string{first.ID, second.ID}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("wrong collector identities: %v != %v", got, want)
		}
	})
}
