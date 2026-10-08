package main

import (
	"context"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
	"testing"
	"time"
)

// TestAlertLogPatternRule evaluates a log_pattern rule by seeding log events
// with matching messages and verifying the alert fires when min_occurrences is met.
func TestAlertLogPatternRule(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "logpat-node-1", "LOGPAT")

	// Seed log events that match the pattern.
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := sut.logStore.AppendEvent(logs.Event{
			AssetID:   "logpat-node-1",
			Source:    "agent",
			Level:     "error",
			Message:   "FATAL: disk I/O error on sda",
			Timestamp: now.Add(-time.Duration(i*10) * time.Second),
		}); err != nil {
			t.Fatalf("failed to seed log event: %v", err)
		}
	}

	// Create rule requiring >= 2 occurrences of ERROR|FATAL.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Log Pattern Rule",
		Kind:          alerts.RuleKindLogPattern,
		Severity:      alerts.SeverityHigh,
		TargetScope:   alerts.TargetScopeAsset,
		WindowSeconds: 300,
		Condition:     map[string]any{"pattern": "ERROR|FATAL", "min_occurrences": float64(2)},
		Labels:        map[string]string{"env": "prod"},
		Targets:       []alerts.RuleTargetInput{{AssetID: "logpat-node-1"}},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: should have a firing instance.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list firing instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 firing instance for log pattern rule, got %d", len(instances))
	}
}

// TestAlertLogPatternBelowThreshold verifies a log_pattern rule does NOT fire
// when occurrences are below the min_occurrences threshold.
func TestAlertLogPatternBelowThreshold(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "loglow-node-1", "LOGLOW")

	// Seed only 1 log event matching the pattern.
	if err := sut.logStore.AppendEvent(logs.Event{
		AssetID:   "loglow-node-1",
		Source:    "agent",
		Level:     "error",
		Message:   "ERROR: something bad happened",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to seed log event: %v", err)
	}

	// Create rule requiring >= 5 occurrences.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Log Pattern Below Threshold",
		Kind:          alerts.RuleKindLogPattern,
		Severity:      alerts.SeverityMedium,
		TargetScope:   alerts.TargetScopeAsset,
		WindowSeconds: 300,
		Condition:     map[string]any{"pattern": "ERROR", "min_occurrences": float64(5)},
		Labels:        map[string]string{"env": "test"},
		Targets:       []alerts.RuleTargetInput{{AssetID: "loglow-node-1"}},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: should NOT have a firing instance.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 firing instances (below threshold), got %d", len(instances))
	}
}

// TestAlertLogPatternGlobalSourceFieldFilters verifies global log_pattern rules
// can filter by source and field_equals without requiring asset-scoped targets.
func TestAlertLogPatternGlobalSourceFieldFilters(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	appendEvent := func(event logs.Event) {
		t.Helper()
		if err := sut.logStore.AppendEvent(event); err != nil {
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
		WindowSeconds: 300,
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

	sut.evaluateSingleRule(ctx, rule, nil)

	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
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

// TestAlertHeartbeatStaleRule verifies the heartbeat_stale rule kind fires when
// an asset's LastSeenAt is older than the configured window.
func TestAlertHeartbeatStaleRule(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "hbstale-node-1", "HBSTALE")

	// Backdate the asset's LastSeenAt to 10 minutes ago.
	assetStore, ok := sut.assetStore.(*persistence.MemoryAssetStore)
	if !ok {
		t.Fatalf("assetStore is not a MemoryAssetStore")
	}
	assetStore.BackdateLastSeenAt("hbstale-node-1", time.Now().UTC().Add(-10*time.Minute))

	// Create heartbeat_stale rule with 5-minute window.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Heartbeat Stale Rule",
		Kind:          alerts.RuleKindHeartbeatStale,
		Severity:      alerts.SeverityHigh,
		TargetScope:   alerts.TargetScopeGlobal,
		WindowSeconds: 300,
		Condition:     map[string]any{},
		Labels:        map[string]string{"env": "prod"},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: should fire because asset is stale.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list firing instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 firing instance for heartbeat stale, got %d", len(instances))
	}
}

// TestAlertHeartbeatStaleNotFiring verifies the heartbeat_stale rule does NOT
// fire when all assets have recent heartbeats.
func TestAlertHeartbeatStaleNotFiring(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	// Create a fresh asset (LastSeenAt = now).
	seedAssetViaHeartbeat(t, sut, "hbfresh-node-1", "HBFRESH")

	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Heartbeat Fresh Rule",
		Kind:          alerts.RuleKindHeartbeatStale,
		Severity:      alerts.SeverityMedium,
		TargetScope:   alerts.TargetScopeGlobal,
		WindowSeconds: 300,
		Condition:     map[string]any{},
		Labels:        map[string]string{"env": "prod"},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: no firing instances.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 firing instances (all fresh), got %d", len(instances))
	}
}

// TestAlertMetricDeadmanRule verifies the metric_deadman rule fires when NO data
// points exist within the window (the asset has stopped reporting).
func TestAlertMetricDeadmanRule(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "deadman-node-1", "DEADMN")

	// Do NOT seed any metric data — the deadman should fire on absence of data.
	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Metric Deadman Rule",
		Kind:          alerts.RuleKindMetricDeadman,
		Severity:      alerts.SeverityCritical,
		TargetScope:   alerts.TargetScopeAsset,
		WindowSeconds: 300,
		Condition:     map[string]any{},
		Labels:        map[string]string{"env": "prod"},
		Targets:       []alerts.RuleTargetInput{{AssetID: "deadman-node-1"}},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: should fire because no data within window.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list firing instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 firing instance for deadman rule, got %d", len(instances))
	}
}

// TestAlertMetricDeadmanNotFiring verifies the deadman rule does NOT fire when
// data points exist within the window.
func TestAlertMetricDeadmanNotFiring(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	seedAssetViaHeartbeat(t, sut, "deadok-node-1", "DEADOK")
	seedMetricSamples(t, sut, "deadok-node-1", "cpu_used_percent", 42.0)

	rule := mustCreateAlertRule(t, sut, alerts.CreateRuleRequest{
		Name:          "Metric Deadman OK Rule",
		Kind:          alerts.RuleKindMetricDeadman,
		Severity:      alerts.SeverityMedium,
		TargetScope:   alerts.TargetScopeAsset,
		WindowSeconds: 300,
		Condition:     map[string]any{},
		Labels:        map[string]string{"env": "prod"},
		Targets:       []alerts.RuleTargetInput{{AssetID: "deadok-node-1"}},
	})

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: no firing instances (data exists).
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("failed to list instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 firing instances (data present), got %d", len(instances))
	}
}

// TestAlertSyntheticCheckRule verifies the synthetic_check rule fires when
// consecutive failures meet the threshold.
func TestAlertSyntheticCheckRule(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := context.Background()

	// Create a synthetic check via the store.
	synStore, ok := sut.syntheticStore.(*persistence.MemorySyntheticStore)
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

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: should fire because 3 consecutive failures.
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
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
	sut := newTestAPIServer(t)
	ctx := context.Background()

	synStore, ok := sut.syntheticStore.(*persistence.MemorySyntheticStore)
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

	sut.evaluateSingleRule(ctx, rule, nil)

	// Verify: no firing instances (recent success breaks consecutive chain).
	instances, err := sut.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
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
