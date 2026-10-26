package persistence

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func TestMemoryAlertFirstFireScopeIsAtomicAndImmutable(t *testing.T) {
	rules, instances := NewMemoryAlertStore(), NewMemoryAlertInstanceStore()
	checkAlertFirstFireScope(t, rules, instances)
	checkFiringSnapshotRead(t, rules, instances)
}

func checkAlertFirstFireScope(t *testing.T, rules AlertStore, store AlertInstanceStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rule, err := rules.CreateAlertRule(alerts.CreateRuleRequest{Name: "scope-owned", Kind: alerts.RuleKindMetricThreshold, Severity: "high", TargetScope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rules.DeleteAlertRule(rule.ID); err != nil {
			t.Error(err)
		}
	})
	create := func(fp string) alerts.AlertInstance {
		t.Helper()
		inst, err := store.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: fp})
		if err != nil {
			t.Fatal(err)
		}
		return inst
	}
	owner := store.(AlertInstanceFiringStore)
	inst := create("atomic")
	scopes := []alerts.FiringScope{{Version: 1, AssetIDs: []string{"a"}, GroupIDs: []string{"ga"}}, {Version: 1, AssetIDs: []string{"b"}, GroupIDs: []string{"gb"}}}
	var wg sync.WaitGroup
	rows := make(chan alerts.AlertInstance, 2)
	errs := make(chan error, 2)
	for _, scope := range scopes {
		wg.Add(1)
		go func(scope alerts.FiringScope) {
			defer wg.Done()
			row, err := owner.FireAlertInstance(ctx, inst.ID, scope)
			rows <- row
			errs <- err
		}(scope)
	}
	wg.Wait()
	close(rows)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	saved, found, err := store.GetAlertInstance(inst.ID)
	if err != nil || !found || saved.FirstFiredAt == nil || saved.FiringScope == nil {
		t.Fatalf("atomic first fire missing: %+v/%v", saved, err)
	}
	if !reflect.DeepEqual(*saved.FiringScope, scopes[0]) && !reflect.DeepEqual(*saved.FiringScope, scopes[1]) {
		t.Fatalf("mixed scope: %+v", saved.FiringScope)
	}
	for row := range rows {
		if !reflect.DeepEqual(row.FiringScope, saved.FiringScope) || !row.FirstFiredAt.Equal(*saved.FirstFiredAt) {
			t.Fatal("concurrent first fire rewrote identity")
		}
	}
	original := alerts.CloneFiringScope(saved.FiringScope)
	saved.FiringScope.AssetIDs[0] = "caller-mutation"
	if _, err := store.UpdateAlertInstanceStatus(inst.ID, alerts.InstanceStatusAcknowledged); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAlertInstanceStatus(inst.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}
	again, err := owner.FireAlertInstance(ctx, inst.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{"replacement"}})
	if err != nil || !reflect.DeepEqual(again.FiringScope, original) || !again.FirstFiredAt.Equal(*saved.FirstFiredAt) {
		t.Fatalf("refire changed scope: %+v/%v", again, err)
	}
	pending := create("never-fired")
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := owner.FireAlertInstance(canceled, pending.ID, scopes[0]); err == nil {
		t.Fatal("canceled first fire accepted")
	}
	if _, err := owner.FireAlertInstance(ctx, pending.ID, alerts.FiringScope{Version: 2}); err == nil {
		t.Fatal("unknown scope accepted")
	}
	unchanged, _, err := store.GetAlertInstance(pending.ID)
	if err != nil || unchanged.Status != alerts.InstanceStatusPending || unchanged.FirstFiredAt != nil || unchanged.FiringScope != nil {
		t.Fatal("rejected fire partially wrote state")
	}
	legacy := create("unknown-origin")
	if _, err := store.UpdateAlertInstanceStatus(legacy.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}
	unknown, err := owner.FireAlertInstance(ctx, legacy.ID, scopes[0])
	if err != nil || unknown.FiringScope != nil {
		t.Fatal("legacy firing gained guessed scope")
	}
}

// More than one SQL cursor page, with exact selection and cancellation during
// the read. The same owner check runs against Memory and owned PostgreSQL.
func checkFiringSnapshotRead(t *testing.T, rules AlertStore, instances AlertInstanceStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rule, err := rules.CreateAlertRule(alerts.CreateRuleRequest{Name: "scope-pages", Kind: alerts.RuleKindMetricThreshold, Severity: "high", TargetScope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rules.DeleteAlertRule(rule.ID); err != nil {
			t.Error(err)
		}
	})
	expected := map[string]bool{}
	for i := 0; i < 129; i++ {
		row, err := instances.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: fmt.Sprintf("snapshot-%03d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = instances.(AlertInstanceFiringStore).FireAlertInstance(ctx, row.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{"snapshot-asset"}}); err != nil {
			t.Fatal(err)
		}
		expected[row.ID] = true
	}
	store := instances.(AlertFiringSnapshotStore)
	after := ""
	if err := store.ReadFiringAlertInstances(ctx, []string{rule.ID}, func(row alerts.AlertInstance) error {
		if !expected[row.ID] || row.ID <= after || row.RuleID != rule.ID || row.FirstFiredAt == nil || row.FiringScope == nil || row.Status != alerts.InstanceStatusFiring {
			return fmt.Errorf("wrong snapshot row: %s", row.ID)
		}
		delete(expected, row.ID)
		after = row.ID
		return nil
	}); err != nil || len(expected) != 0 {
		t.Fatalf("incomplete snapshot: %d/%v", len(expected), err)
	}
	interrupted, stop := context.WithCancel(ctx)
	visits := 0
	err = store.ReadFiringAlertInstances(interrupted, []string{rule.ID}, func(alerts.AlertInstance) error { visits++; stop(); return nil })
	if !errors.Is(err, context.Canceled) || visits != 1 {
		t.Fatalf("read ignored cancellation: %d/%v", visits, err)
	}
}
