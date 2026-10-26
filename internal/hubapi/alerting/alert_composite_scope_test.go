package alerting

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

type failedCompositeSnapshot struct{ persistence.AlertInstanceStore }

func (failedCompositeSnapshot) ReadFiringAlertInstances(context.Context, []string, func(alerts.AlertInstance) error) error {
	return errors.New("snapshot read failed")
}

// Saturation cannot turn a still-active composite into a recovery, and every
// positive branch contributes its saved scope, even beyond the first page.
func TestCompositeFiringSnapshotIsCompleteAndFailClosed(t *testing.T) {
	d, _ := firingScopeDeps(t)
	child := scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60}, false)
	owner := d.AlertInstanceStore.(persistence.AlertInstanceFiringStore)
	for i := 0; i < 501; i++ {
		inst, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: child.ID, Fingerprint: fmt.Sprintf("page-%03d", i)})
		if err != nil {
			t.Fatal(err)
		}
		scope := assetAScope()
		if i == 0 { // Oldest B would be omitted by the old latest-500 query.
			scope.AssetIDs, scope.GroupIDs = []string{"scope-b"}, []string{"group-scope-b"}
		}
		if _, err = owner.FireAlertInstance(context.Background(), inst.ID, scope); err != nil {
			t.Fatal(err)
		}
	}
	if got := d.prefetchFiringInstances(context.Background()); got != nil {
		t.Fatal("saturated prefetch was treated as complete")
	}
	rule := scopeRule(t, d, alerts.RuleKindComposite, map[string]any{"rule_ids": []interface{}{child.ID}, "operator": "and"}, false)
	want := assetAScope()
	want.AssetIDs, want.GroupIDs = append(want.AssetIDs, "scope-b"), append(want.GroupIDs, "group-scope-b")
	inst := evaluateScope(t, d, rule, want)
	original := d.AlertInstanceStore
	d.AlertInstanceStore = failedCompositeSnapshot{original}
	d.EvaluateSingleRule(context.Background(), rule, nil)
	row, found, err := original.GetAlertInstance(inst.ID)
	if err != nil || !found || row.Status != alerts.InstanceStatusFiring {
		t.Fatal("snapshot failure resolved the active composite")
	}
	d.AlertInstanceStore = original
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.evaluateCompositeOutcome(ctx, rule, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}
