package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

// Memory cannot prove the SQL keyset, C ordering or cancellation/transaction
// release. This stays separately gated to a job-stamped disposable database.
func TestPostgresIncidentScopeReadsCompleteAndCancellable(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM incidents)+
	 (SELECT COUNT(*) FROM incident_assets)+(SELECT COUNT(*) FROM incident_alert_links)+
	 (SELECT COUNT(*) FROM assets)+(SELECT COUNT(*) FROM groups)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("requires empty owned scope tables: %d/%v", count, err)
	}
	prefix := "scope-reader-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	incidentIDs := []string{prefix + "incident", prefix + "foreign-incident"}
	groupIDs := []string{prefix + "group", prefix + "foreign-group"}
	assetIDs := make([]string, 0, 514)
	for i := 0; i < 513; i++ {
		assetIDs = append(assetIDs, prefix+fmt.Sprintf("asset-%04d", i))
	}
	assetIDs[0] = prefix + "asset-Ä"
	assetIDs = append(assetIDs, prefix+"foreign-asset")
	// All exact IDs are declared before writing. Link rows belong only to these
	// incidents and are removed by their FK cascade, never a broad table wipe.
	t.Cleanup(func() {
		cleanup, end := context.WithTimeout(context.Background(), 5*time.Second)
		defer end()
		for _, deletion := range []struct {
			query string
			ids   []string
		}{
			{`DELETE FROM incidents WHERE id=ANY($1::text[])`, incidentIDs},
			{`DELETE FROM assets WHERE id=ANY($1::text[])`, assetIDs},
			{`DELETE FROM groups WHERE id=ANY($1::text[])`, groupIDs},
		} {
			if _, err := s.pool.Exec(cleanup, deletion.query, deletion.ids); err != nil {
				t.Errorf("owned scope cleanup: %v", err)
			}
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT
		 (SELECT COUNT(*) FROM incidents WHERE id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM incident_assets WHERE incident_id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM incident_alert_links WHERE incident_id=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM assets WHERE id=ANY($2::text[]))+
		 (SELECT COUNT(*) FROM groups WHERE id=ANY($3::text[]))`, incidentIDs, assetIDs, groupIDs).Scan(&left); err != nil || left != 0 {
			t.Errorf("scope cleanup incomplete: %d/%v", left, err)
		}
	})
	for _, id := range groupIDs {
		if _, err := s.pool.Exec(ctx, `INSERT INTO groups(id,name,slug,created_at,updated_at) VALUES($1,$1,$1,now(),now())`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range incidentIDs {
		if _, err := s.pool.Exec(ctx, `INSERT INTO incidents(id,title,status,severity,source,created_by,opened_at,created_at,updated_at)
		 VALUES($1,$1,'open','medium','manual','scope-fixture',now(),now(),now())`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO assets(id,type,name,source,status,group_id,created_at,updated_at,last_seen_at)
	 SELECT id,'host',id,'manual','online',CASE WHEN ord<=513 THEN $2 ELSE $3 END,now(),now(),now()
	 FROM unnest($1::text[]) WITH ORDINALITY AS item(id,ord)`, assetIDs, groupIDs[0], groupIDs[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO incident_assets(id,incident_id,asset_id,role,created_at)
	 SELECT $1 || 'asset-link-' || ord::text,CASE WHEN ord<=513 THEN $2 ELSE $3 END,id,'impacted',now()
	 FROM unnest($4::text[]) WITH ORDINALITY AS item(id,ord)`, prefix, incidentIDs[0], incidentIDs[1], assetIDs); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO incident_alert_links(id,incident_id,alert_instance_id,alert_fingerprint,link_type,created_by,created_at)
	 SELECT $1 || 'alert-link-' || ord::text,CASE WHEN ord<=513 THEN $2 ELSE $3 END,
	 id,$1 || 'fingerprint-' || ord::text,'related','scope-fixture',now()
	 FROM unnest($4::text[]) WITH ORDINALITY AS item(id,ord)`, prefix, incidentIDs[0], incidentIDs[1], assetIDs); err != nil {
		t.Fatal(err)
	}
	want := append([]string(nil), assetIDs[:513]...)
	sort.Strings(want)
	var got []string
	previous := ""
	err := s.ReadIncidentAssetsContext(ctx, incidentIDs[0], func(row incidents.IncidentAsset) error {
		if row.ID <= previous || row.IncidentID != incidentIDs[0] {
			return fmt.Errorf("wrong asset link owner/order")
		}
		previous = row.ID
		got = append(got, row.AssetID)
		return nil
	})
	sort.Strings(got)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("complete asset scope: %d/%v", len(got), err)
	}
	got, previous = nil, ""
	err = s.ReadIncidentAlertLinksContext(ctx, incidentIDs[0], func(row incidents.AlertLink) error {
		if row.ID <= previous || row.IncidentID != incidentIDs[0] || row.AlertFingerprint == "" {
			return fmt.Errorf("wrong alert link owner/order/discriminator")
		}
		previous = row.ID
		got = append(got, row.AlertInstanceID)
		return nil
	})
	sort.Strings(got)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("complete alert scope: %d/%v", len(got), err)
	}
	got = nil
	err = s.ReadGroupAssetIDsContext(ctx, groupIDs[0], func(id string) error { got = append(got, id); return nil })
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("group C order/exact membership: %d/%v", len(got), err)
	}
	canceled, stop := context.WithCancel(ctx)
	visits := 0
	err = s.ReadIncidentAlertLinksContext(canceled, incidentIDs[0], func(incidents.AlertLink) error {
		visits++
		if visits == 129 {
			stop()
		}
		return nil
	})
	stop()
	if !errors.Is(err, context.Canceled) || visits != 129 {
		t.Fatalf("SQL partial scope acknowledged after cancellation: %d/%v", visits, err)
	}
	requirePostgresPoolReleased(t, ctx, s.pool)
}
