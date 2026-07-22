package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestMemoryAlertMetricsSnapshotExactCountsAndBoundedDuplicateNameSeries(t *testing.T) {
	rules := NewMemoryAlertStore()
	instances := NewMemoryAlertInstanceStore()
	createdRules := make([]alerts.Rule, 0, telemetry.MaxAlertRuleMetricSeries+2)
	for i := 0; i < telemetry.MaxAlertRuleMetricSeries+2; i++ {
		rule, err := rules.CreateAlertRule(alerts.CreateRuleRequest{
			Name:   "duplicate-name",
			Status: alerts.RuleStatusActive,
		})
		if err != nil {
			t.Fatalf("create active rule %d: %v", i, err)
		}
		createdRules = append(createdRules, rule)
		if _, err := rules.RecordAlertEvaluation("  "+rule.ID+"  ", alerts.Evaluation{
			Status:      alerts.EvaluationStatusOK,
			EvaluatedAt: time.Now().UTC().Add(time.Duration(i) * time.Nanosecond),
			DurationMS:  i,
		}); err != nil {
			t.Fatalf("record evaluation %d: %v", i, err)
		}
	}
	if _, err := rules.CreateAlertRule(alerts.CreateRuleRequest{Name: "paused", Status: alerts.RuleStatusPaused}); err != nil {
		t.Fatalf("create paused rule: %v", err)
	}

	for i := 0; i < telemetry.MaxAlertRuleMetricSeries+1; i++ {
		instance, err := instances.CreateAlertInstance(alerts.CreateInstanceRequest{
			RuleID:      createdRules[0].ID,
			Fingerprint: fmt.Sprintf("firing-%03d", i),
		})
		if err != nil {
			t.Fatalf("create firing instance %d: %v", i, err)
		}
		if _, err := instances.UpdateAlertInstanceStatus(instance.ID, alerts.InstanceStatusFiring); err != nil {
			t.Fatalf("mark instance %d firing: %v", i, err)
		}
	}
	pending, err := instances.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: createdRules[0].ID, Fingerprint: "pending"})
	if err != nil || pending.Status != alerts.InstanceStatusPending {
		t.Fatalf("create pending instance: instance=%+v err=%v", pending, err)
	}

	store := NewMemoryAlertMetricsSnapshotStore(rules, instances)
	snapshot, err := store.AlertMetricsSnapshot(context.Background(), telemetry.MaxAlertRuleMetricSeries)
	if err != nil {
		t.Fatalf("load alert metric snapshot: %v", err)
	}
	if snapshot.ActiveRuleCount != int64(telemetry.MaxAlertRuleMetricSeries+2) {
		t.Fatalf("active rule count = %d, want %d", snapshot.ActiveRuleCount, telemetry.MaxAlertRuleMetricSeries+2)
	}
	if snapshot.FiringInstanceCount != int64(telemetry.MaxAlertRuleMetricSeries+1) {
		t.Fatalf("firing instance count = %d, want %d", snapshot.FiringInstanceCount, telemetry.MaxAlertRuleMetricSeries+1)
	}
	if len(snapshot.RuleEvaluations) != telemetry.MaxAlertRuleMetricSeries {
		t.Fatalf("rule evaluation series = %d, want cap %d", len(snapshot.RuleEvaluations), telemetry.MaxAlertRuleMetricSeries)
	}
	seenIDs := make(map[string]struct{}, len(snapshot.RuleEvaluations))
	for _, evaluation := range snapshot.RuleEvaluations {
		if evaluation.RuleID == "" || evaluation.RuleName != "duplicate-name" {
			t.Fatalf("invalid rule evaluation identity: %+v", evaluation)
		}
		if _, duplicate := seenIDs[evaluation.RuleID]; duplicate {
			t.Fatalf("duplicate rule ID in snapshot: %q", evaluation.RuleID)
		}
		seenIDs[evaluation.RuleID] = struct{}{}
	}
	if got := len(rules.rules); got != telemetry.MaxAlertRuleMetricSeries+3 {
		t.Fatalf("whitespace rule ID created duplicate map entry: rules=%d", got)
	}
}

func TestMemoryAlertMetricsSnapshotValidatesBudgetAndContext(t *testing.T) {
	store := NewMemoryAlertMetricsSnapshotStore(NewMemoryAlertStore(), NewMemoryAlertInstanceStore())
	if _, err := store.AlertMetricsSnapshot(context.Background(), telemetry.MaxAlertRuleMetricSeries+1); !errors.Is(err, ErrAlertMetricSnapshotLimitExceeded) {
		t.Fatalf("oversized budget error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AlertMetricsSnapshot(ctx, telemetry.MaxAlertRuleMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v, want context.Canceled", err)
	}
}
