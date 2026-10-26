package persistence

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func TestMemoryAlertTimingStateAndAtomicClaims(t *testing.T) {
	rules, instances := NewMemoryAlertStore(), NewMemoryAlertInstanceStore()
	checkAlertTimingStores(t, rules, instances, func(id string) {
		instances.mu.Lock()
		defer instances.mu.Unlock()
		row := instances.instances[id]
		row.FirstFiredAt = nil
		instances.instances[id] = row
	})
}

func checkAlertTimingStores(t *testing.T, rules AlertStore, instances AlertInstanceStore, markLegacy func(string)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	zero := 0
	interval := 120
	rule, err := rules.CreateAlertRule(alerts.CreateRuleRequest{Name: "timing-owned", Kind: alerts.RuleKindMetricThreshold, Severity: "high", TargetScope: "global", CooldownSeconds: &zero, ReopenAfterSeconds: &zero, EvaluationIntervalSeconds: &interval})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rules.DeleteAlertRule(rule.ID); err != nil {
			t.Errorf("delete timing rule: %v", err)
		}
	})
	if rule.CooldownSeconds != 0 || rule.ReopenAfterSeconds != 0 || rule.LastAutoAttemptAt != nil {
		t.Fatalf("zero/initial state: %+v", rule)
	}
	defaults, err := rules.CreateAlertRule(alerts.CreateRuleRequest{Name: "timing-defaults", TargetScope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rules.DeleteAlertRule(defaults.ID); err != nil {
			t.Errorf("delete defaults: %v", err)
		}
	})
	if defaults.CooldownSeconds != 300 || defaults.ReopenAfterSeconds != 60 || defaults.EvaluationIntervalSeconds != 30 {
		t.Fatalf("omitted defaults: %+v", defaults)
	}
	clock := rules.(AlertRuleTimingStore)
	at := time.Now().UTC().Truncate(time.Microsecond)
	type claimResult struct {
		admitted bool
		err      error
	}
	results := make(chan claimResult, 8)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ok, err := clock.ClaimAlertRuleEvaluation(ctx, rule.ID, at)
			results <- claimResult{ok, err}
		}()
	}
	workers.Wait()
	close(results)
	admitted := 0
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.admitted {
			admitted++
		}
	}
	if admitted != 1 {
		t.Fatalf("atomic same-due claims=%d", admitted)
	}
	if _, err := rules.RecordAlertEvaluation(rule.ID, alerts.Evaluation{Status: alerts.EvaluationStatusOK, EvaluatedAt: at.Add(24 * time.Hour), Details: map[string]any{"mode": "manual_test"}}); err != nil {
		t.Fatal(err)
	}
	checkClaim := func(when time.Time, want bool) {
		t.Helper()
		ok, err := clock.ClaimAlertRuleEvaluation(ctx, rule.ID, when)
		if err != nil || ok != want {
			t.Fatalf("claim %s=%v/%v want%v", when, ok, err, want)
		}
	}
	checkClaim(at.Add(119*time.Second), false)
	checkClaim(at.Add(120*time.Second), true)
	stored, found, err := rules.GetAlertRule(rule.ID)
	if err != nil || !found || stored.LastAutoAttemptAt == nil || !stored.LastAutoAttemptAt.Equal(at.Add(120*time.Second)) || stored.LastEvaluatedAt == nil || !stored.LastEvaluatedAt.Equal(at.Add(24*time.Hour)) {
		t.Fatalf("separate timing roundtrip: %+v/%v", stored, err)
	}
	interval = 60
	if _, err := rules.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{EvaluationIntervalSeconds: &interval}); err != nil {
		t.Fatal(err)
	}
	checkClaim(at.Add(180*time.Second), true)
	paused := alerts.RuleStatusPaused
	if _, err := rules.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	checkClaim(at.Add(240*time.Second), false)
	canceled, stop := context.WithCancel(ctx)
	stop()
	if ok, err := clock.ClaimAlertRuleEvaluation(canceled, rule.ID, at.Add(time.Hour)); err == nil || ok {
		t.Fatal("canceled attempt admitted")
	}

	newInstance := func(fingerprint string) alerts.AlertInstance {
		t.Helper()
		row, err := instances.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: fingerprint, Severity: "high"})
		if err != nil {
			t.Fatal(err)
		}
		if row.FirstFiredAt != nil {
			t.Fatal("pending row invented history")
		}
		return row
	}
	transition := func(id, status string) alerts.AlertInstance {
		t.Helper()
		row, err := instances.UpdateAlertInstanceStatus(id, status)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	first := newInstance("exact-fingerprint")
	first = transition(first.ID, alerts.InstanceStatusFiring)
	if first.FirstFiredAt == nil {
		t.Fatal("first transition missing timestamp")
	}
	transition(first.ID, alerts.InstanceStatusAcknowledged)
	refire := transition(first.ID, alerts.InstanceStatusFiring)
	if !refire.FirstFiredAt.Equal(*first.FirstFiredAt) {
		t.Fatal("acknowledgement refire changed first fire")
	}
	resolved := transition(first.ID, alerts.InstanceStatusResolved)
	noOp := transition(first.ID, alerts.InstanceStatusResolved)
	if resolved.ResolvedAt == nil || !resolved.ResolvedAt.Equal(*noOp.ResolvedAt) {
		t.Fatal("no-op resolution moved timestamp")
	}
	placeholder := newInstance("exact-fingerprint")
	transition(placeholder.ID, alerts.InstanceStatusResolved)
	legacy := newInstance("exact-fingerprint")
	transition(legacy.ID, alerts.InstanceStatusFiring)
	markLegacy(legacy.ID)
	transition(legacy.ID, alerts.InstanceStatusAcknowledged)
	legacy = transition(legacy.ID, alerts.InstanceStatusFiring)
	if legacy.FirstFiredAt != nil {
		t.Fatal("legacy acknowledgement invented historical fire")
	}
	transition(legacy.ID, alerts.InstanceStatusResolved)
	foreign := newInstance("other-fingerprint")
	transition(foreign.ID, alerts.InstanceStatusFiring)
	transition(foreign.ID, alerts.InstanceStatusResolved)
	anchors, err := instances.(AlertInstanceTimingStore).AlertFingerprintTiming(ctx, rule.ID, "exact-fingerprint")
	if err != nil || anchors.LastFirstFiredAt == nil || anchors.LastResolvedAt == nil || !anchors.LastFirstFiredAt.Equal(*first.FirstFiredAt) || !anchors.LastResolvedAt.Equal(*resolved.ResolvedAt) {
		t.Fatalf("known exact fingerprint anchors: %+v/%v", anchors, err)
	}
	unknown, err := instances.(AlertInstanceTimingStore).AlertFingerprintTiming(ctx, rule.ID, "absent")
	if err != nil || unknown.LastFirstFiredAt != nil || unknown.LastResolvedAt != nil {
		t.Fatal("absent history guessed")
	}
	legacyOnly := newInstance("legacy-only")
	transition(legacyOnly.ID, alerts.InstanceStatusFiring)
	markLegacy(legacyOnly.ID)
	transition(legacyOnly.ID, alerts.InstanceStatusResolved)
	legacyTiming, err := instances.(AlertInstanceTimingStore).AlertFingerprintTiming(ctx, rule.ID, "legacy-only")
	if err != nil || legacyTiming.LastFirstFiredAt != nil || legacyTiming.LastResolvedAt != nil {
		t.Fatal("legacy-only resolved row invented holdoff history")
	}
}
