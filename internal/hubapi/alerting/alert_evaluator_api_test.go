package alerting_test

import (
	"context"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
	"testing"
	"time"
)

// TestAlertLogPatternGlobalSourceFieldFilters verifies global log_pattern rules
// can filter by source and field_equals without requiring asset-scoped targets.
func TestAlertLogPatternGlobalSourceFieldFilters(t *testing.T) {
	sut := hubdeps.New(t)
	ctx := context.Background()

	now := time.Now().UTC()
	appendEvent := func(event logs.Event) {
		t.Helper()
		if err := sut.LogStore.AppendEvent(event); err != nil {
			t.Fatalf("failed to append event: %v", err)
		}
	}

	appendEvent(logs.Event{
		Source:  "mobile_client_telemetry",
		Level:   "warning",
		Message: "mobile client telemetry metric",
		Fields: map[string]string{
			"metric": "reconnect_scheduled",
			"status": "warning",
		},
		Timestamp: now.Add(-20 * time.Second),
	})
	appendEvent(logs.Event{
		Source:  "mobile_client_telemetry",
		Level:   "warning",
		Message: "mobile client telemetry metric",
		Fields: map[string]string{
			"metric": "reconnect_scheduled",
			"status": "warning",
		},
		Timestamp: now.Add(-10 * time.Second),
	})
	// Non-matching telemetry event should not count toward reconnect threshold.
	appendEvent(logs.Event{
		Source:  "mobile_client_telemetry",
		Level:   "error",
		Message: "mobile client telemetry metric",
		Fields: map[string]string{
			"metric": "request.duration",
			"status": "error",
		},
		Timestamp: now.Add(-5 * time.Second),
	})

	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Global Mobile Reconnect Spike",
		Kind:          alerts.RuleKindLogPattern,
		Severity:      alerts.SeverityHigh,
		TargetScope:   alerts.TargetScopeGlobal,
		WindowSeconds: alertTestDuration(300),
		Condition: map[string]any{
			"pattern":         "mobile client telemetry metric",
			"source":          "mobile_client_telemetry",
			"min_occurrences": float64(2),
			"field_equals": map[string]any{
				"metric": "reconnect_scheduled",
			},
		},
		Labels: map[string]string{"channel": "mobile"},
	})

	sut.AlertingDeps().EvaluateSingleRule(ctx, rule, nil)

	instances, err := sut.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list firing instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 firing instance for global source+field log pattern, got %d", len(instances))
	}
}

// TestAlertSyntheticCheckRule verifies the synthetic_check rule fires when
// consecutive failures meet the threshold.
func TestAlertSyntheticCheckRule(t *testing.T) {
	sut := hubdeps.New(t)
	ctx := context.Background()

	// Create a synthetic check via the store.
	synStore, ok := sut.SyntheticStore.(*persistence.MemorySyntheticStore)
	if !ok {
		t.Fatalf("syntheticStore is not a MemorySyntheticStore")
	}
	check, err := synStore.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:      "HTTP health check",
		CheckType: "http",
		Target:    "http://example.com/health",
	})
	if err != nil {
		t.Fatalf("failed to create synthetic check: %v", err)
	}

	// Record 3 consecutive failures (newest first in results).
	for i := 0; i < 3; i++ {
		_, err := synStore.RecordSyntheticResult(check.ID, synthetic.Result{
			Status:    synthetic.ResultStatusFail,
			Error:     "connection refused",
			CheckedAt: time.Now().UTC().Add(-time.Duration(i*15) * time.Second),
		})
		if err != nil {
			t.Fatalf("failed to record synthetic result: %v", err)
		}
	}

	// Create a synthetic_check alert rule.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:        "Synthetic Check Rule",
		Kind:        alerts.RuleKindSyntheticCheck,
		Severity:    alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeGlobal,
		Condition: map[string]any{
			"check_id":             check.ID,
			"consecutive_failures": float64(3),
		},
		Labels: map[string]string{"env": "prod"},
	})

	sut.AlertingDeps().EvaluateSingleRule(ctx, rule, nil)

	// Verify: should fire because 3 consecutive failures.
	instances, err := sut.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list firing instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 firing instance for synthetic check, got %d", len(instances))
	}
}

// TestAlertSyntheticCheckNotFiring verifies the synthetic_check rule does NOT
// fire when the most recent results include successes.
func TestAlertSyntheticCheckNotFiring(t *testing.T) {
	sut := hubdeps.New(t)
	ctx := context.Background()

	synStore, ok := sut.SyntheticStore.(*persistence.MemorySyntheticStore)
	if !ok {
		t.Fatalf("syntheticStore is not a MemorySyntheticStore")
	}
	check, err := synStore.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:      "HTTP health check OK",
		CheckType: "http",
		Target:    "http://example.com/health",
	})
	if err != nil {
		t.Fatalf("failed to create synthetic check: %v", err)
	}

	// Record 2 failures followed by 1 success (most recent).
	// Results are stored newest-first, so record in chronological order
	// (oldest first) and the store prepends.
	_, _ = synStore.RecordSyntheticResult(check.ID, synthetic.Result{
		Status:    synthetic.ResultStatusFail,
		CheckedAt: time.Now().UTC().Add(-30 * time.Second),
	})
	_, _ = synStore.RecordSyntheticResult(check.ID, synthetic.Result{
		Status:    synthetic.ResultStatusFail,
		CheckedAt: time.Now().UTC().Add(-15 * time.Second),
	})
	_, _ = synStore.RecordSyntheticResult(check.ID, synthetic.Result{
		Status:    synthetic.ResultStatusOK,
		CheckedAt: time.Now().UTC(),
	})

	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:        "Synthetic Check OK Rule",
		Kind:        alerts.RuleKindSyntheticCheck,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeGlobal,
		Condition: map[string]any{
			"check_id":             check.ID,
			"consecutive_failures": float64(3),
		},
		Labels: map[string]string{"env": "prod"},
	})

	sut.AlertingDeps().EvaluateSingleRule(ctx, rule, nil)

	// Verify: no firing instances (recent success breaks consecutive chain).
	instances, err := sut.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 firing instances (success breaks chain), got %d", len(instances))
	}
}

func alertTestDuration(value int) *int { return &value }
