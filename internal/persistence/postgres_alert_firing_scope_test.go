package persistence

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

// Separately gated owned PostgreSQL proof; source review/Memory is not SQL proof.
func TestPostgresAlertFirstFireScopeIsAtomicAndImmutable(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM alert_rules)+(SELECT COUNT(*) FROM alert_instances)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("requires empty owned alert tables: %d/%v", count, err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		var left int
		if err := s.pool.QueryRow(cleanup, `SELECT (SELECT COUNT(*) FROM alert_rules)+(SELECT COUNT(*) FROM alert_instances)`).Scan(&left); err != nil || left != 0 {
			t.Errorf("owned scope cleanup: %d/%v", left, err)
		}
	})
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, _, err := s.GetAssetContext(canceled, "scope-not-created"); !errors.Is(err, context.Canceled) {
		t.Fatalf("asset context lost: %v", err)
	}
	if _, _, err := s.GetManualWebServiceContext(canceled, "scope-not-created"); !errors.Is(err, context.Canceled) {
		t.Fatalf("manual service context lost: %v", err)
	}
	checkAlertFirstFireScope(t, s, s)
	checkFiringSnapshotRead(t, s, s)
	rule, err := s.CreateAlertRule(alerts.CreateRuleRequest{Name: "scope-rollback", Kind: alerts.RuleKindMetricThreshold, Severity: "high", TargetScope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.DeleteAlertRule(rule.ID); err != nil {
			t.Error(err)
		}
	})
	inst, err := s.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: "rollback"})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^ainst_[a-zA-Z0-9_]+$`).MatchString(inst.ID) {
		t.Fatal("unexpected generated instance identity")
	}
	// Exact row-only fixture constraint; no unrelated rows may be present at entry.
	if _, err := s.pool.Exec(ctx, `ALTER TABLE alert_instances ADD CONSTRAINT ltqa_scope_rollback CHECK (id <> '`+inst.ID+`' OR firing_scope IS NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := s.pool.Exec(cleanup, `ALTER TABLE alert_instances DROP CONSTRAINT IF EXISTS ltqa_scope_rollback`); err != nil {
			t.Error(err)
		}
	})
	if _, err := s.FireAlertInstance(ctx, inst.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{"owned"}}); err == nil {
		t.Fatal("injected write failure absent")
	}
	row, ok, err := s.GetAlertInstance(inst.ID)
	if err != nil || !ok || row.Status != alerts.InstanceStatusPending || row.FirstFiredAt != nil || row.FiringScope != nil {
		t.Fatalf("SQL failure partially wrote firing: %+v/%v", row, err)
	}
}
