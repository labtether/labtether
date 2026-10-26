package alerting

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type observedRulePages struct {
	*persistence.MemoryAlertStore
	considered           map[string]int
	attemptTimes         []time.Time
	pageSizes            []int
	evaluated, stopAfter int
	stop                 context.CancelFunc
	claimErrorID         string
	pageError            bool
}

func (s *observedRulePages) PageActiveAlertRules(ctx context.Context, cursor persistence.AlertRuleEvaluationCursor, limit int) (persistence.AlertRuleEvaluationPage, error) {
	deadline, bounded := ctx.Deadline()
	if !bounded || time.Until(deadline) > 5*time.Second {
		return persistence.AlertRuleEvaluationPage{}, errors.New("page context is not bounded")
	}
	if s.pageError {
		return persistence.AlertRuleEvaluationPage{}, errors.New("page unavailable")
	}
	page, err := s.MemoryAlertStore.PageActiveAlertRules(ctx, cursor, limit)
	s.pageSizes = append(s.pageSizes, len(page.Rules))
	return page, err
}

func (s *observedRulePages) ClaimAlertRuleEvaluation(ctx context.Context, id string, at time.Time) (bool, error) {
	s.considered[id]++
	s.attemptTimes = append(s.attemptTimes, at)
	if id == s.claimErrorID {
		return false, errors.New("one rule claim unavailable")
	}
	return s.MemoryAlertStore.ClaimAlertRuleEvaluation(ctx, id, at)
}

func (s *observedRulePages) RecordAlertEvaluationContext(ctx context.Context, id string, value alerts.Evaluation) (alerts.Evaluation, error) {
	row, err := s.MemoryAlertStore.RecordAlertEvaluationContext(ctx, id, value)
	if err == nil {
		s.evaluated++
		if s.evaluated == s.stopAfter && s.stop != nil {
			s.stop()
		}
	}
	return row, err
}

func TestAutomaticAlertRulePagingResumesBeyond500(t *testing.T) {
	d := newTestAlertingDeps(t)
	store := &observedRulePages{MemoryAlertStore: persistence.NewMemoryAlertStore(), considered: map[string]int{}}
	d.AlertStore = store
	ids := make([]string, 0, 520)
	for i := 0; i < 520; i++ {
		rule, err := store.CreateAlertRule(alerts.CreateRuleRequest{Name: "paged-rule", Kind: alerts.RuleKindMetricThreshold,
			Severity: "high", TargetScope: alerts.TargetScopeGlobal,
			Condition: map[string]any{"metric": "cpu_used_percent", "value": 90.0}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rule.ID)
	}
	sort.Strings(ids)
	next := time.Now().UTC().Add(time.Hour)
	clock := func() time.Time { at := next; next = next.Add(time.Nanosecond); return at }
	canceled, stop := context.WithCancel(context.Background())
	stop()
	d.evaluateAlertRulesWithClock(canceled, clock)
	if len(store.considered) != 0 || d.alertRuleEvaluationCursor.Started {
		t.Fatal("pre-cancel advanced a rule sweep")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.stop, store.stopAfter = cancel, 137
	d.evaluateAlertRulesWithClock(ctx, clock)
	resume := d.alertRuleEvaluationCursor
	if store.evaluated != 137 || !resume.Started || resume.AfterID != ids[136] || resume.ThroughID != ids[len(ids)-1] {
		t.Fatalf("mid-page continuation lost: count=%d cursor=%+v", store.evaluated, resume)
	}
	// Change an old row, remove an unread row and pause another. Ordering by
	// updated_at would move the edited row; immutable IDs must still progress.
	name, paused := "edited behind cursor", alerts.RuleStatusPaused
	if _, err := store.UpdateAlertRule(ids[2], alerts.UpdateRuleRequest{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAlertRule(ids[200]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAlertRule(ids[201], alerts.UpdateRuleRequest{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	store.pageError = true
	d.evaluateAlertRulesWithClock(context.Background(), clock)
	if d.alertRuleEvaluationCursor != resume {
		t.Fatal("failed page lost its exact continuation")
	}
	store.pageError, store.claimErrorID = false, ids[138]
	store.stop = nil
	for i := 0; i < 8 && d.alertRuleEvaluationCursor.Started; i++ {
		d.evaluateAlertRulesWithClock(context.Background(), clock)
	}
	if d.alertRuleEvaluationCursor.Started {
		t.Fatal("finite sweep did not finish")
	}
	for _, id := range ids {
		want := 1
		if id == ids[200] || id == ids[201] {
			want = 0
		}
		if store.considered[id] != want {
			t.Fatalf("rule starved/repeated after updated_at changes: %s count=%d want=%d", id, store.considered[id], want)
		}
		rows, err := store.ListAlertEvaluations(id, 2)
		if id == ids[200] {
			if !errors.Is(err, alerts.ErrRuleNotFound) {
				t.Fatal("deleted rule remained readable")
			}
			continue
		}
		if id == store.claimErrorID {
			want = 0
		}
		if err != nil || len(rows) != want {
			t.Fatalf("actual evaluation history: %s count=%d want=%d err=%v", id, len(rows), want, err)
		}
	}
	for i, at := range store.attemptTimes {
		if i > 0 && !at.After(store.attemptTimes[i-1]) {
			t.Fatal("page reused an early tick clock")
		}
	}
	for _, size := range store.pageSizes {
		if size > persistence.AlertRuleEvaluationPageSize {
			t.Fatal("unbounded rule page")
		}
	}
	// A fresh sweep revisits a failed claim and a newly active rule. The due
	// store prevents duplicate evaluations of the other already-attempted rows.
	active := alerts.RuleStatusActive
	if _, err := store.UpdateAlertRule(ids[201], alerts.UpdateRuleRequest{Status: &active}); err != nil {
		t.Fatal(err)
	}
	store.claimErrorID = ""
	before := store.evaluated
	d.evaluateAlertRulesWithClock(context.Background(), clock)
	for i := 0; i < 8 && d.alertRuleEvaluationCursor.Started; i++ {
		d.evaluateAlertRulesWithClock(context.Background(), clock)
	}
	if d.alertRuleEvaluationCursor.Started || store.evaluated != before+2 {
		t.Fatal("fresh sweep skipped failed/new active rule or bypassed due claims")
	}
}

type observedRuleAssetReads struct {
	*persistence.MemoryAssetStore
	inventoryReads int
	exactIDs       []string
	allowInventory bool
	exactAfterList []string
}

func (s *observedRuleAssetReads) ListAssets() ([]assets.Asset, error) {
	s.inventoryReads++
	return nil, errors.New("unrelated fleet inventory unavailable")
}

func (s *observedRuleAssetReads) ListAssetsContext(ctx context.Context) ([]assets.Asset, error) {
	s.inventoryReads++
	if !s.allowInventory {
		return nil, errors.New("unrelated fleet inventory unavailable")
	}
	return s.MemoryAssetStore.ListAssetsContext(ctx)
}

func (s *observedRuleAssetReads) GetAssetContext(ctx context.Context, id string) (assets.Asset, bool, error) {
	s.exactIDs = append(s.exactIDs, id)
	if s.inventoryReads > 0 {
		s.exactAfterList = append(s.exactAfterList, id)
	}
	return s.MemoryAssetStore.GetAssetContext(ctx, id)
}

// A single-asset rule must still fire when the unrelated fleet cannot be listed.
// Drive the real automatic owner and retain the real Memory asset/metric stores.
func TestAutomaticExplicitAssetRuleAvoidsFleetInventory(t *testing.T) {
	d := newTestAlertingDeps(t)
	defer d.WaitForNotificationDispatches()
	store := &observedRuleAssetReads{MemoryAssetStore: d.AssetStore.(*persistence.MemoryAssetStore)}
	d.AssetStore = store
	for _, id := range []string{"selected-node", "unrelated-a", "unrelated-b"} {
		if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Source: "agent", Status: "online"}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	metrics := d.TelemetryStore.(*persistence.MemoryTelemetryStore)
	if err := metrics.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: "selected-node", Metric: "cpu_used_percent", Unit: "percent", Value: 96, CollectedAt: now}}); err != nil {
		t.Fatal(err)
	}
	rule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "one asset", Kind: alerts.RuleKindMetricThreshold,
		Severity: alerts.SeverityHigh, TargetScope: alerts.TargetScopeAsset,
		Targets: []alerts.RuleTargetInput{{AssetID: "selected-node"}}, Condition: map[string]any{"metric": "cpu_used_percent", "value": 90.0}})
	if err != nil {
		t.Fatal(err)
	}
	d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return now })
	instance, found, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
	if err != nil || !found || instance.Status != alerts.InstanceStatusFiring || instance.FiringScope == nil ||
		len(instance.FiringScope.AssetIDs) != 1 || instance.FiringScope.AssetIDs[0] != "selected-node" {
		t.Fatalf("explicit rule did not fire for its exact asset: %+v %v", instance, err)
	}
	history, err := d.AlertStore.ListAlertEvaluations(rule.ID, 2)
	if err != nil || len(history) != 1 || history[0].Status != alerts.EvaluationStatusTriggered {
		t.Fatalf("explicit rule lost its successful evaluation: %+v %v", history, err)
	}
	if store.inventoryReads != 0 || len(store.exactIDs) == 0 {
		t.Fatalf("worker read fleet inventory: full=%d exact=%v", store.inventoryReads, store.exactIDs)
	}
	for _, id := range store.exactIDs {
		if id != "selected-node" {
			t.Fatalf("worker copied an unrelated asset: %s", id)
		}
	}
	// Selectors need the whole inventory, but share one read within this slice.
	// Sort real store IDs so the last new rule is explicit and runs after both
	// selectors; it must still use an exact read once the snapshot is loaded.
	var rules []alerts.Rule
	for i := 0; i < 3; i++ {
		next, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "mixed targets", Kind: alerts.RuleKindMetricThreshold,
			Severity: alerts.SeverityHigh, TargetScope: alerts.TargetScopeAsset,
			Targets: []alerts.RuleTargetInput{{AssetID: "selected-node"}}, Condition: map[string]any{"metric": "cpu_used_percent", "value": 90.0}})
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, next)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	selectors := []alerts.RuleTargetInput{{Selector: map[string]any{"source": "agent"}}}
	for _, broad := range rules[:2] {
		if _, err := d.AlertStore.UpdateAlertRule(broad.ID, alerts.UpdateRuleRequest{Targets: &selectors}); err != nil {
			t.Fatal(err)
		}
	}
	store.allowInventory = true
	d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return now.Add(time.Hour) })
	if store.inventoryReads != 1 || len(store.exactAfterList) == 0 {
		t.Fatalf("broad rules did not share or explicit rule copied snapshot: lists=%d exact-after=%v", store.inventoryReads, store.exactAfterList)
	}
	for _, id := range store.exactAfterList {
		if id != "selected-node" {
			t.Fatalf("loaded snapshot caused unrelated exact read: %s", id)
		}
	}
	for _, next := range rules {
		got, found, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(next.ID, alerts.GenerateFingerprint(next.ID, next.Labels))
		if err != nil || !found || got.Status != alerts.InstanceStatusFiring || got.FiringScope == nil ||
			len(got.FiringScope.AssetIDs) != 1 || got.FiringScope.AssetIDs[0] != "selected-node" {
			t.Fatalf("mixed slice lost complete matching result: %+v %v", got, err)
		}
	}
}
