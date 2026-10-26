package alerting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type evaluatorContextWait struct {
	entered chan struct{}
	once    sync.Once
}

func (w *evaluatorContextWait) wait(ctx context.Context) error {
	w.once.Do(func() { close(w.entered) })
	<-ctx.Done()
	return ctx.Err()
}

type evaluatorAssetWait struct {
	*persistence.MemoryAssetStore
	wait *evaluatorContextWait
}

func (s *evaluatorAssetWait) ListAssetsContext(ctx context.Context) ([]assets.Asset, error) {
	return nil, s.wait.wait(ctx)
}

type evaluatorGroupWait struct {
	*persistence.MemoryGroupStore
	wait *evaluatorContextWait
}

func (s *evaluatorGroupWait) ListGroupsContext(ctx context.Context) ([]groups.Group, error) {
	return nil, s.wait.wait(ctx)
}

type evaluatorMetricWait struct {
	*persistence.MemoryTelemetryStore
	wait *evaluatorContextWait
}

func (s *evaluatorMetricWait) MetricSeriesBatchContext(ctx context.Context, _ []string, _ string, _, _ time.Time, _ time.Duration, _ int) (map[string]telemetry.Series, error) {
	return nil, s.wait.wait(ctx)
}

type evaluatorMetricOverflow struct {
	*persistence.MemoryTelemetryStore
}

func (s *evaluatorMetricOverflow) MetricSeriesBatchContext(ctx context.Context, _ []string, _ string, _, _ time.Time, _ time.Duration, limit int) (map[string]telemetry.Series, error) {
	if limit != alertMetricRawPointLimit {
		return nil, errors.New("wrong raw point budget")
	}
	return nil, persistence.ErrTelemetryQueryLimitExceeded
}

// Drive the production ticker loop and registered manual handler. A stopped
// worker must release its waiter/mutex and never recover an alert using an
// incomplete or canceled read; a new caller must still be able to use the owner.
func TestAlertEvaluatorCancellationPreservesStateAndResumes(t *testing.T) {
	for _, phase := range []string{"global inventory", "condition", "manual condition", "manual authorization groups", "manual authorization assets"} {
		t.Run(phase, func(t *testing.T) {
			d := newTestAlertingDeps(t)
			rawAssets := d.AssetStore.(*persistence.MemoryAssetStore)
			rawMetrics := d.TelemetryStore.(*persistence.MemoryTelemetryStore)
			rawGroups := d.GroupStore.(*persistence.MemoryGroupStore)
			manual := strings.HasPrefix(phase, "manual")
			restricted := strings.HasPrefix(phase, "manual authorization")
			if _, err := rawAssets.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "cancel-node", Type: "host", Source: "agent", Status: "online"}); err != nil {
				t.Fatal(err)
			}
			if err := rawMetrics.AppendSamples(context.Background(), []telemetry.MetricSample{{AssetID: "cancel-node", Metric: "cpu_used_percent", Unit: "percent", Value: 10, CollectedAt: time.Now().UTC()}}); err != nil {
				t.Fatal(err)
			}
			request := alerts.CreateRuleRequest{Name: "cancellable", Kind: alerts.RuleKindMetricThreshold, Severity: alerts.SeverityHigh,
				TargetScope: alerts.TargetScopeAsset, Targets: []alerts.RuleTargetInput{{AssetID: "cancel-node"}}, Condition: map[string]any{"metric": "cpu_used_percent", "value": 80.0}}
			if phase == "global inventory" {
				request.TargetScope, request.Targets = alerts.TargetScopeGlobal, nil
			}
			rule, err := d.AlertStore.CreateAlertRule(request)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels), Severity: rule.Severity})
			if err != nil {
				t.Fatal(err)
			}
			instance, err = d.AlertInstanceStore.(persistence.AlertInstanceFiringStore).FireAlertInstance(context.Background(), instance.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{"cancel-node"}})
			if err != nil {
				t.Fatal(err)
			}
			wait := &evaluatorContextWait{entered: make(chan struct{})}
			if phase == "global inventory" || phase == "manual authorization assets" {
				d.AssetStore = &evaluatorAssetWait{rawAssets, wait}
			} else if phase == "manual authorization groups" {
				d.GroupStore = &evaluatorGroupWait{rawGroups, wait}
			} else {
				d.TelemetryStore = &evaluatorMetricWait{rawMetrics, wait}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if restricted {
				ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"cancel-node"})
			}
			done := make(chan struct{})
			broadcasts := 0
			d.Broadcast = func(string, map[string]any) { broadcasts++ }
			ticks := make(chan time.Time, 1)
			rec := httptest.NewRecorder()
			go func() {
				defer close(done)
				if manual {
					req := httptest.NewRequest(http.MethodPost, "/alerts/rules/"+rule.ID+"/test", nil).WithContext(ctx)
					d.HandleAlertRuleActions(rec, req)
				} else {
					d.runAlertEvaluator(ctx, ticks)
				}
			}()
			ticks <- time.Now()
			select {
			case <-wait.entered:
			case <-time.After(time.Second):
				cancel()
				<-done
				t.Fatal("real owner never reached bounded read")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("canceled evaluator did not stop")
			}
			d.WaitForNotificationDispatches()
			got, found, err := d.AlertInstanceStore.GetAlertInstance(instance.ID)
			if err != nil || !found || !reflect.DeepEqual(got, instance) || broadcasts != 0 {
				t.Fatalf("canceled read changed alert: %+v %v broadcasts=%d", got, err, broadcasts)
			}
			history, err := d.AlertStore.ListAlertEvaluations(rule.ID, 10)
			if err != nil || len(history) != 0 {
				t.Fatalf("canceled attempt wrote history: %+v %v", history, err)
			}
			if manual && rec.Code == http.StatusOK {
				t.Fatal("canceled manual preview claimed success")
			}
			if restricted && rec.Code != http.StatusForbidden {
				t.Fatalf("authorization cancellation did not fail closed: %d", rec.Code)
			}
			d.AssetStore, d.TelemetryStore, d.GroupStore = rawAssets, rawMetrics, rawGroups
			// An overflow is a completed error evaluation, not a false condition.
			d.TelemetryStore = &evaluatorMetricOverflow{rawMetrics}
			d.EvaluateSingleRule(context.Background(), rule, nil)
			got, _, _ = d.AlertInstanceStore.GetAlertInstance(instance.ID)
			history, _ = d.AlertStore.ListAlertEvaluations(rule.ID, 10)
			if !reflect.DeepEqual(got, instance) || len(history) != 1 || history[0].Status != alerts.EvaluationStatusError {
				t.Fatal("overflow changed active state or lost error history")
			}
			d.TelemetryStore = rawMetrics
			// Same manual owner succeeds with current reads, while staying history-only.
			rec = httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/alerts/rules/"+rule.ID+"/test", nil)
			if restricted {
				req = req.WithContext(apiv2.ContextWithAllowedAssets(req.Context(), []string{"cancel-node"}))
			}
			d.HandleAlertRuleActions(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("manual recovery: %d %s", rec.Code, rec.Body.String())
			}
			got, _, _ = d.AlertInstanceStore.GetAlertInstance(instance.ID)
			if !reflect.DeepEqual(got, instance) {
				t.Fatal("manual preview transitioned alert")
			}
			// Fresh clock proves the canceled automatic worker released its mutex and
			// keeps its finite cursor usable on the next sweep/attempt.
			d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return time.Now().UTC().Add(time.Hour) })
			got, _, _ = d.AlertInstanceStore.GetAlertInstance(instance.ID)
			if got.Status != alerts.InstanceStatusResolved {
				t.Fatalf("worker did not resume: %+v", got)
			}
		})
	}
}
