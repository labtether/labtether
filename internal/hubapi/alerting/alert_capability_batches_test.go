package alerting

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type observedCapabilityBatches struct {
	*persistence.MemoryCanonicalModelStore
	t                  *testing.T
	reads, globalReads int
	failAt             int
	cancelAt           int
	cancel             context.CancelFunc
}

func (s *observedCapabilityBatches) ListCapabilitySetsContext(ctx context.Context, limit int) ([]model.CapabilitySet, error) {
	s.globalReads++
	return s.MemoryCanonicalModelStore.ListCapabilitySetsContext(ctx, limit)
}

func (s *observedCapabilityBatches) GetResourceCapabilitySetsContext(ctx context.Context, ids []string) ([]model.CapabilitySet, error) {
	s.t.Helper()
	s.reads++
	if len(ids) == 0 || len(ids) > persistence.AlertCapabilityBatchSize {
		s.t.Fatalf("unbounded candidate capability batch: %d", len(ids))
	}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("automatic capability lookup lost bounded context")
	}
	if s.reads == s.failAt {
		return nil, errors.New("capability query unavailable")
	}
	if s.reads == s.cancelAt {
		s.cancel()
	}
	return s.MemoryCanonicalModelStore.GetResourceCapabilitySetsContext(ctx, ids)
}

// A global newest5000 listing used to omit the sole matching asset and falsely
// recover this already-firing rule. Exercise the actual automatic worker, then
// prove an incomplete/error read cannot recover it either.
func TestAutomaticCapabilitySelectionBeyond5000PreservesFiring(t *testing.T) {
	d := newTestAlertingDeps(t)
	defer d.WaitForNotificationDispatches()
	d.CapabilityIDsFromSet = resources.CapabilityIDsFromSet
	d.InferCapabilityIDsFromAssetMetadata = resources.InferCapabilityIDsFromAssetMetadata
	d.MergeCapabilityIDs = resources.MergeCapabilityIDs
	canonical := &observedCapabilityBatches{MemoryCanonicalModelStore: d.CanonicalStore.(*persistence.MemoryCanonicalModelStore), t: t}
	d.CanonicalStore = canonical
	now := time.Now().UTC()
	const matchingID = "cap-00000"
	const capability = "canonical.only.example"
	for i := 0; i < 5001; i++ {
		id := fmt.Sprintf("cap-%05d", i)
		if _, err := d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Source: "agent", Status: "online"}); err != nil {
			t.Fatal(err)
		}
		set := model.CapabilitySet{SubjectType: "resource", SubjectID: id, UpdatedAt: now}
		if id == matchingID {
			set.UpdatedAt = now.Add(-time.Hour)
			set.Capabilities = []model.CapabilitySpec{{ID: capability, Scope: model.CapabilityScopeRead}}
		}
		if _, err := canonical.UpsertCapabilitySet(set); err != nil {
			t.Fatal(err)
		}
	}
	metrics := d.TelemetryStore.(*persistence.MemoryTelemetryStore)
	if err := metrics.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: matchingID, Metric: "cpu_used_percent", Unit: "percent", Value: 96, CollectedAt: now}}); err != nil {
		t.Fatal(err)
	}
	rule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "old canonical match", Kind: alerts.RuleKindMetricThreshold,
		Severity: alerts.SeverityHigh, TargetScope: alerts.TargetScopeAsset,
		Targets:   []alerts.RuleTargetInput{{Selector: map[string]any{"capability": capability}}},
		Condition: map[string]any{"metric": "cpu_used_percent", "value": 90.0}})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID,
		Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels), Severity: rule.Severity})
	if err != nil {
		t.Fatal(err)
	}
	instance, err = d.AlertInstanceStore.(persistence.AlertInstanceFiringStore).FireAlertInstance(context.Background(), instance.ID,
		alerts.FiringScope{Version: 1, AssetIDs: []string{matchingID}})
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(time.Hour)
	d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return at })
	got, found, err := d.AlertInstanceStore.GetAlertInstance(instance.ID)
	history, historyErr := d.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || !found || got.Status != alerts.InstanceStatusFiring || historyErr != nil || len(history) != 1 || history[0].Status != alerts.EvaluationStatusTriggered {
		t.Fatalf("old canonical match falsely recovered/lost: %+v history=%+v err=%v/%v", got, history, err, historyErr)
	}
	if canonical.globalReads != 0 || canonical.reads < 40 {
		t.Fatalf("expected complete bounded candidate traversal, global=%d batches=%d", canonical.globalReads, canonical.reads)
	}
	// Manual preview and a caller's partial prefetch must see the same asset.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	targets, err := d.resolveRuleTargetAssetsContext(ctx, rule, nil, map[string][]string{"not-a-candidate": nil}, true)
	if err != nil || len(targets) != 1 || targets[0].ID != matchingID {
		t.Fatalf("partial prefetch hid a canonical match: %+v %v", targets, err)
	}
	triggered, err := d.EvaluateMetricThreshold(ctx, rule)
	if err != nil || !triggered {
		t.Fatalf("manual/automatic capability disagreement: %v %v", triggered, err)
	}
	// The real condition is now false. A failed second batch must still keep
	// the old firing row instead of treating a partial target set as complete.
	lowMetrics := persistence.NewMemoryTelemetryStore()
	if err := lowMetrics.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: matchingID, Metric: "cpu_used_percent", Unit: "percent", Value: 10, CollectedAt: now}}); err != nil {
		t.Fatal(err)
	}
	d.TelemetryStore = lowMetrics
	prior := got
	canonical.reads, canonical.failAt = 0, 2
	at = at.Add(time.Hour)
	d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return at })
	got, found, err = d.AlertInstanceStore.GetAlertInstance(instance.ID)
	failedHistory, historyErr := d.AlertStore.ListAlertEvaluations(rule.ID, 10)
	newErrors := 0
	for _, row := range failedHistory {
		if row.ID != history[0].ID && row.Status == alerts.EvaluationStatusError {
			newErrors++
		}
	}
	if err != nil || !found || !reflect.DeepEqual(got, prior) || historyErr != nil || len(failedHistory) != 2 || newErrors != 1 || canonical.reads != 2 {
		t.Fatalf("query failure changed active state or lost error: %+v history=%+v reads=%d err=%v/%v", got, failedHistory, canonical.reads, err, historyErr)
	}
	// Cancellation reaches the real Memory batch owner and discards its result.
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	canonical.reads, canonical.failAt, canonical.cancelAt, canonical.cancel = 0, 0, 2, stop
	at = at.Add(time.Hour)
	d.evaluateAlertRulesWithClock(ctx, func() time.Time { return at })
	got, _, err = d.AlertInstanceStore.GetAlertInstance(instance.ID)
	afterCancel, historyErr := d.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || historyErr != nil || !reflect.DeepEqual(got, prior) || len(afterCancel) != 2 || canonical.reads != 2 || ctx.Err() != context.Canceled {
		t.Fatal("canceled capability read produced a condition/state result")
	}
}
