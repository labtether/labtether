package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

// This separately gated owner proves real SQL filtering and C-order cursors.
// A Memory pass is never treated as execution of these queries.
func TestPostgresIncidentCorrelationPagesFilterAndResume(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := newCorrelationPageFixture("correlation-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-")
	tables := []string{"incidents", "action_runs", "update_runs", "update_plans", "alert_instances", "alert_rules", "audit_events", "groups", "assets"}
	for _, table := range tables {
		var count int
		if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("requires empty owned %s: %d/%v", table, count, err)
		}
	}
	ids := map[string][]string{"update_plans": {f.prefix + "plan"}, "alert_rules": {f.prefix + "rule"}, "groups": {f.scope.GroupID}, "assets": {f.scope.AssetIDs[0]}}
	for _, row := range f.incidents {
		ids["incidents"] = append(ids["incidents"], row.ID)
	}
	for _, row := range f.actions {
		ids["action_runs"] = append(ids["action_runs"], row.ID)
	}
	ids["action_runs"] = append(ids["action_runs"], f.prefix+"zz-late")
	for _, row := range f.updates {
		ids["update_runs"] = append(ids["update_runs"], row.ID)
	}
	for _, row := range f.alerts {
		ids["alert_instances"] = append(ids["alert_instances"], row.ID)
	}
	for _, row := range f.audits {
		ids["audit_events"] = append(ids["audit_events"], row.ID)
	}
	// Declare all row intents before writes. Parent deletion also owns its exact
	// incident-event FK cascade; no table-wide truncation is used for cleanup.
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, table := range tables {
			if _, err := s.pool.Exec(cleanup, "DELETE FROM "+table+" WHERE id=ANY($1::text[])", ids[table]); err != nil {
				t.Errorf("owned %s cleanup: %v", table, err)
			}
			var left int
			if err := s.pool.QueryRow(cleanup, "SELECT COUNT(*) FROM "+table+" WHERE id=ANY($1::text[])", ids[table]).Scan(&left); err != nil || left != 0 {
				t.Errorf("owned %s remains: %d/%v", table, left, err)
			}
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT COUNT(*) FROM incident_events WHERE incident_id=ANY($1::text[])`, ids["incidents"]).Scan(&left); err != nil || left != 0 {
			t.Errorf("event cascade remains: %d/%v", left, err)
		}
	})
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO groups(id,name,slug,created_at,updated_at) VALUES($1,$1,$1,$2,$2)`, f.scope.GroupID, f.at)
	exec(`INSERT INTO assets(id,type,name,source,status,created_at,updated_at,last_seen_at) VALUES($1,'host',$1,'manual','online',$2,$2,$2)`, f.scope.AssetIDs[0], f.at)
	exec(`INSERT INTO update_plans(id,name,targets,scopes,created_at,updated_at) VALUES($1,$1,'[]','[]',$2,$2)`, f.prefix+"plan", f.at)
	exec(`INSERT INTO alert_rules(id,name,status,kind,severity,target_scope,condition,created_by,created_at,updated_at) VALUES($1,$1,'active','metric_threshold','high','global','{}','page-fixture',$2,$2)`, f.prefix+"rule", f.at)
	for _, row := range f.incidents {
		exec(`INSERT INTO incidents(id,title,status,severity,source,created_by,group_id,primary_asset_id,opened_at,created_at,updated_at) VALUES($1,$1,$2,'medium','manual','page-fixture',NULLIF($3,''),NULLIF($4,''),$5,$6,$6)`, row.ID, row.Status, row.GroupID, row.PrimaryAssetID, row.OpenedAt, row.CreatedAt)
	}
	for _, row := range f.actions {
		exec(`INSERT INTO action_runs(id,type,actor_id,target,status,created_at,updated_at) VALUES($1,$2,'page-fixture',$3,$4,$5,$5)`, row.ID, row.Type, row.Target, row.Status, row.CreatedAt)
	}
	for _, row := range f.updates {
		payload, err := json.Marshal(row.Results)
		if err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO update_runs(id,plan_id,plan_name,actor_id,status,results,created_at,updated_at) VALUES($1,$2,$2,'page-fixture',$3,$4::jsonb,$5,$5)`, row.ID, row.PlanID, row.Status, payload, row.CreatedAt)
	}
	exec(`UPDATE update_runs SET results='"malformed"'::jsonb WHERE id=$1`, f.prefix+"update-malformed")
	for _, row := range f.alerts {
		var payload []byte
		if row.FiringScope != nil {
			var err error
			payload, err = json.Marshal(row.FiringScope)
			if err != nil {
				t.Fatal(err)
			}
		}
		exec(`INSERT INTO alert_instances(id,rule_id,fingerprint,status,severity,started_at,last_fired_at,created_at,updated_at,first_fired_at,firing_scope) VALUES($1,$2,$3,'resolved',$4,$5,$5,$5,$5,$6,$7::jsonb)`, row.ID, row.RuleID, row.Fingerprint, row.Severity, f.at, row.FirstFiredAt, payload)
	}
	for _, row := range f.audits {
		exec(`INSERT INTO audit_events(id,type,actor_id,target,timestamp,details) VALUES($1,$2,$3,$4,$5,'{"private":"omitted"}')`, row.ID, row.Type, row.ActorID, row.Target, row.Timestamp)
	}
	// Legacy JSON forms must be rejected before LIMIT and cannot poison the
	// matching history behind them. Includes scalar/array/null/string version.
	for i, body := range []string{`[]`, `"bad"`, `null`, `{"version":"1","asset_ids":[]}`, `{"version":1,"asset_ids":[1]}`, `{"version":1,"asset_ids":"bad"}`} {
		exec(`UPDATE alert_instances SET firing_scope=$2::jsonb WHERE id=$1`, fmt.Sprintf("%s0-foreign-%03d", f.prefix, i), body)
	}
	checkCorrelationPageContract(t, ctx, f, correlationPageOwners{s, s, s, s, s}, func() {
		exec(`UPDATE action_runs SET updated_at=$2 WHERE id=$1`, f.actions[1].ID, f.at.Add(time.Hour))
		exec(`INSERT INTO action_runs(id,type,actor_id,target,status,created_at,updated_at) VALUES($1,'command','page-fixture',$2,'failed',$3,$3)`, f.prefix+"zz-late", f.scope.AssetIDs[0], f.at)
	})
	req := incidents.CreateIncidentEventRequest{IncidentID: f.scope.IncidentID, EventType: "action_run", SourceRef: "action:" + f.actions[0].ID, Summary: "owned projection", OccurredAt: f.at}
	first, err := s.UpsertIncidentEventContext(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.UpsertIncidentEventContext(ctx, req)
	if err != nil || second.ID != first.ID {
		t.Fatalf("retry replayed event: %v", err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, _, _, err := s.ReadActionCorrelationPage(canceled, f.window, f.scope, CorrelationCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("SQL read lost context: %v", err)
	}
	if _, err := s.UpsertIncidentEventContext(canceled, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("SQL effect lost context: %v", err)
	}
	if s.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("page or canceled effect retained a connection")
	}
}
