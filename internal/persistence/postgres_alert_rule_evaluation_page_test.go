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
)

// Real SQL proof is separately gated; Memory cannot prove C ordering, target
// joins or transaction release. No SQL runs in the ordinary focused package.
func TestPostgresAlertRuleEvaluationPagesResumeWithinFixedSweep(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM alert_rules)+
 (SELECT COUNT(*) FROM alert_rule_targets)+(SELECT COUNT(*) FROM alert_evaluations)+
 (SELECT COUNT(*) FROM alert_instances)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("requires empty owned alert tables: %d/%v", count, err)
	}
	prefix := "rule-page-" + strings.ToLower(os.Getenv("LABTETHER_JOB_ID")) + "-"
	ids := make([]string, 520)
	for i := range ids {
		ids[i] = prefix + fmt.Sprintf("rule-%04d", i)
	}
	ids[0], ids[1], ids[2] = prefix+"rule-Ä", prefix+"rule_Z", prefix+"rule.A"
	sort.Strings(ids)
	lateIDs := []string{prefix + "!late", prefix + "Ω-late"}
	ownedIDs := append(append([]string(nil), ids...), lateIDs...)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `DELETE FROM alert_rules WHERE id=ANY($1::text[])`, ownedIDs); err != nil {
			t.Errorf("owned rule cleanup: %v", err)
		}
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT
 (SELECT COUNT(*) FROM alert_rules WHERE id=ANY($1::text[]))+
 (SELECT COUNT(*) FROM alert_rule_targets WHERE rule_id=ANY($1::text[]))`, ownedIDs).Scan(&left); err != nil || left != 0 {
			t.Errorf("owned rule/target cleanup incomplete: %d/%v", left, err)
		}
	})
	seed := func(values []string) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, `INSERT INTO alert_rules(id,name,status,kind,severity,target_scope,
 condition,created_by,created_at,updated_at,last_evaluated_at,last_auto_attempt_at)
 SELECT id,id,'active','metric_threshold','high','global','{}'::jsonb,'page-fixture',
 now(),now(),now(),now() FROM unnest($1::text[]) AS item(id)`, values); err != nil {
			t.Fatal(err)
		}
	}
	seed(ids)
	if _, err := s.pool.Exec(ctx, `INSERT INTO alert_rule_targets(id,rule_id,selector,created_at)
 VALUES($1,$2,'{"source":"fixture"}'::jsonb,now())`, prefix+"target", ids[0]); err != nil {
		t.Fatal(err)
	}
	cursor := AlertRuleEvaluationCursor{}
	page, err := s.PageActiveAlertRules(ctx, cursor, AlertRuleEvaluationPageSize)
	if err != nil || len(page.Rules) != 128 || page.Done || page.ThroughID != ids[len(ids)-1] {
		t.Fatalf("first SQL page: %+v/%v", page, err)
	}
	if page.Rules[0].ID != ids[0] || len(page.Rules[0].Targets) != 1 ||
		page.Rules[0].Targets[0].RuleID != ids[0] || page.Rules[0].Targets[0].Selector["source"] != "fixture" ||
		page.Rules[0].LastAutoAttemptAt == nil || page.Rules[0].LastEvaluatedAt == nil {
		t.Fatal("page lost exact target or timing fields")
	}
	got := make([]string, 0, len(ids))
	for _, row := range page.Rules {
		got = append(got, row.ID)
	}
	cursor = AlertRuleEvaluationCursor{Started: true, AfterID: got[len(got)-1], ThroughID: page.ThroughID}
	// An edited old row must not move back into the unread range. New IDs on
	// either side of this range belong to the next finite sweep.
	if _, err := s.pool.Exec(ctx, `UPDATE alert_rules SET updated_at=now()+interval '1 day' WHERE id=$1`, ids[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1`, ids[200]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE alert_rules SET status='paused' WHERE id=$1`, ids[201]); err != nil {
		t.Fatal(err)
	}
	seed(lateIDs)
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := s.PageActiveAlertRules(canceled, cursor, 128); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SQL page was accepted: %v", err)
	}
	if s.pool.Stat().AcquiredConns() != 0 {
		t.Fatal("canceled page retained a pool connection")
	}
	readRest := func() {
		t.Helper()
		for n := 0; n < 8; n++ {
			page, err := s.PageActiveAlertRules(ctx, cursor, 128)
			if err != nil || len(page.Rules) > 128 || (cursor.Started && page.ThroughID != cursor.ThroughID) {
				t.Fatalf("SQL continuation failed: %+v/%v", cursor, err)
			}
			cursor.Started, cursor.ThroughID = true, page.ThroughID
			for _, row := range page.Rules {
				if row.ID <= cursor.AfterID || row.ID > cursor.ThroughID || row.Status != "active" {
					t.Fatal("SQL page order/ownership changed")
				}
				got = append(got, row.ID)
				cursor.AfterID = row.ID
			}
			if page.Done {
				return
			}
		}
		t.Fatal("SQL finite sweep did not finish")
	}
	readRest()
	want := append(append([]string(nil), ids[:200]...), ids[202:]...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("first sweep lost/duplicated active IDs: %d/%d", len(got), len(want))
	}
	got, cursor = nil, AlertRuleEvaluationCursor{}
	readRest()
	want = append(want, lateIDs...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new IDs starved across sweeps: %d/%d", len(got), len(want))
	}
}
