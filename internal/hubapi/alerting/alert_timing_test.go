package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/persistence"
)

// Manual Test updates history but must not postpone automatic attempts. Invalid
// stored conditions still consume one due attempt, rather than a hot retry loop.
func TestAlertTimingCreateAndAutomaticCadence(t *testing.T) {
	d := newTestAlertingDeps(t)
	create := func(extra map[string]any) alerts.Rule {
		body := map[string]any{"name": "timing", "kind": "metric_threshold", "severity": "high", "target_scope": "global", "condition": map[string]any{"metric": "cpu_used_percent", "operator": ">", "value": 90}}
		for key, value := range extra {
			body[key] = value
		}
		rec := metricContractRequest(t, d, http.MethodPost, "/alerts/rules", body)
		var response struct {
			Rule alerts.Rule `json:"rule"`
		}
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &response) != nil || response.Rule.ID == "" {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		return response.Rule
	}
	defaults := create(map[string]any{"status": "paused"})
	if defaults.CooldownSeconds != 300 || defaults.ReopenAfterSeconds != 60 || defaults.EvaluationIntervalSeconds != 30 || defaults.WindowSeconds != 300 {
		t.Fatalf("omitted defaults: %+v", defaults)
	}
	rule := create(map[string]any{"cooldown_seconds": 0, "reopen_after_seconds": 0, "evaluation_interval_seconds": 0, "window_seconds": 0})
	if rule.CooldownSeconds != 0 || rule.ReopenAfterSeconds != 0 || rule.EvaluationIntervalSeconds != 30 || rule.WindowSeconds != 300 {
		t.Fatalf("explicit zero contract: %+v", rule)
	}
	update := metricContractRequest(t, d, http.MethodPatch, "/alerts/rules/"+rule.ID, map[string]any{"evaluation_interval_seconds": 120})
	if update.Code != http.StatusOK {
		t.Fatalf("set interval: %d", update.Code)
	}
	at := time.Now().UTC()
	evaluate := func(when time.Time) {
		d.evaluateAlertRulesWithClock(context.Background(), func() time.Time { return when })
	}
	evaluate(at)
	assertAttempt := func(want time.Time, count int) {
		t.Helper()
		stored, ok, err := d.AlertStore.GetAlertRule(rule.ID)
		history, listErr := d.AlertStore.ListAlertEvaluations(rule.ID, 20)
		if err != nil || !ok || listErr != nil || stored.LastAutoAttemptAt == nil || !stored.LastAutoAttemptAt.Equal(want) || len(history) != count {
			t.Fatalf("attempt/history: %+v %d %v/%v", stored, len(history), err, listErr)
		}
	}
	assertAttempt(at, 1)
	preview := metricContractRequest(t, d, http.MethodPost, "/alerts/rules/"+rule.ID+"/test", map[string]any{"at": at.Add(24 * time.Hour)})
	if preview.Code != http.StatusOK {
		t.Fatalf("manual preview: %d", preview.Code)
	}
	assertAttempt(at, 2)
	evaluate(at.Add(119 * time.Second))
	assertAttempt(at, 2)
	bad := map[string]any{"metric": "cpu_used_percent", "value": "broken"}
	if _, err := d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &bad}); err != nil {
		t.Fatal(err)
	}
	evaluate(at.Add(120 * time.Second))
	assertAttempt(at.Add(120*time.Second), 3)
	history, _ := d.AlertStore.ListAlertEvaluations(rule.ID, 20)
	errorsFound := 0
	for _, row := range history {
		if row.Status == alerts.EvaluationStatusError {
			errorsFound++
		}
	}
	if errorsFound != 1 {
		t.Fatalf("automatic malformed condition errors=%d", errorsFound)
	}
	evaluate(at.Add(121 * time.Second))
	assertAttempt(at.Add(120*time.Second), 3)
	update = metricContractRequest(t, d, http.MethodPatch, "/alerts/rules/"+rule.ID, map[string]any{"evaluation_interval_seconds": 60, "cooldown_seconds": 0, "reopen_after_seconds": 0})
	if update.Code != http.StatusOK {
		t.Fatalf("change interval: %d", update.Code)
	}
	evaluate(at.Add(180 * time.Second))
	assertAttempt(at.Add(180*time.Second), 4)
	paused := alerts.RuleStatusPaused
	if _, err := d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Status: &paused}); err != nil {
		t.Fatal(err)
	}
	evaluate(at.Add(240 * time.Second))
	assertAttempt(at.Add(180*time.Second), 4)
	active := alerts.RuleStatusActive
	if _, err := d.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Status: &active}); err != nil {
		t.Fatal(err)
	}
	evaluate(at.Add(240 * time.Second))
	assertAttempt(at.Add(240*time.Second), 5)
	// Each serial rule owns its actual attempt time, not one early tick time.
	extra := create(map[string]any{"evaluation_interval_seconds": 120})
	next := at.Add(time.Hour)
	d.evaluateAlertRulesWithClock(context.Background(), func() time.Time {
		value := next
		next = next.Add(time.Second)
		return value
	})
	firstStored, _, _ := d.AlertStore.GetAlertRule(rule.ID)
	extraStored, _, _ := d.AlertStore.GetAlertRule(extra.ID)
	if firstStored.LastAutoAttemptAt == nil || extraStored.LastAutoAttemptAt == nil || firstStored.LastAutoAttemptAt.Equal(*extraStored.LastAutoAttemptAt) {
		t.Fatal("serial attempts reused the batch timestamp")
	}
}

type unavailableAlertTiming struct {
	*persistence.MemoryAlertInstanceStore
}

func (unavailableAlertTiming) AlertFingerprintTiming(context.Context, string, string) (persistence.AlertFingerprintTiming, error) {
	return persistence.AlertFingerprintTiming{}, errors.New("read failed")
}

// Real instance transitions protect both holdoff anchors, exact fingerprint
// isolation, acknowledgement and maintenance placeholders without time sleeps.
func TestAlertTimingHoldoffUsesOnlyGenuineFires(t *testing.T) {
	ctx := context.Background()
	store := persistence.NewMemoryAlertInstanceStore()
	d := &Deps{AlertInstanceStore: store}
	rule := alerts.Rule{ID: "timing-rule", Name: "Timing", Severity: "high", Labels: map[string]string{"node": "a"}, CooldownSeconds: 120, ReopenAfterSeconds: 60}
	fingerprint := alerts.GenerateFingerprint(rule.ID, rule.Labels)
	active := func() (alerts.AlertInstance, bool) {
		t.Helper()
		v, ok, err := store.GetActiveInstanceByFingerprint(rule.ID, fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		return v, ok
	}
	d.fireOrRefireAlertAt(ctx, rule, nil, time.Now().UTC())
	first, ok := active()
	if !ok || first.FirstFiredAt == nil {
		t.Fatalf("real fire has no anchor: %+v", first)
	}
	if _, err := store.UpdateAlertInstanceStatus(first.ID, alerts.InstanceStatusAcknowledged); err != nil {
		t.Fatal(err)
	}
	d.fireOrRefireAlertAt(ctx, rule, nil, first.FirstFiredAt.Add(30*time.Second))
	ack, ok := active()
	if !ok || ack.ID != first.ID || ack.Status != alerts.InstanceStatusAcknowledged || !ack.FirstFiredAt.Equal(*first.FirstFiredAt) {
		t.Fatalf("active dedup/ack changed: %+v", ack)
	}
	d.resolveStaleInstances(rule, nil)
	resolved, _, _ := store.GetAlertInstance(first.ID)
	if resolved.ResolvedAt == nil {
		t.Fatal("acknowledged recovery not resolved")
	}
	again, err := store.UpdateAlertInstanceStatus(first.ID, alerts.InstanceStatusResolved)
	if err != nil || !again.ResolvedAt.Equal(*resolved.ResolvedAt) {
		t.Fatal("no-op resolution moved anchor")
	}
	until := first.FirstFiredAt.Add(120 * time.Second)
	if other := resolved.ResolvedAt.Add(60 * time.Second); other.After(until) {
		until = other
	}
	d.fireOrRefireAlertAt(ctx, rule, nil, until.Add(-time.Nanosecond))
	if _, ok := active(); ok {
		t.Fatal("fired before cooldown/reopen maximum")
	}
	// A new label fingerprint has independent timing.
	otherRule := rule
	otherRule.Labels = map[string]string{"node": "b"}
	d.fireOrRefireAlertAt(ctx, otherRule, nil, time.Now().UTC())
	if _, ok, err := store.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, otherRule.Labels)); err != nil || !ok {
		t.Fatal("other fingerprint incorrectly blocked")
	}
	d.fireOrRefireAlertAt(ctx, rule, nil, until)
	second, ok := active()
	if !ok || second.ID == first.ID || second.FirstFiredAt == nil {
		t.Fatal("due recurrence did not create a fresh firing row")
	}
	// The reopen delay can also dominate cooldown.
	d.resolveStaleInstances(rule, nil)
	second, _, _ = store.GetAlertInstance(second.ID)
	rule.CooldownSeconds = 0
	rule.ReopenAfterSeconds = 180
	d.fireOrRefireAlertAt(ctx, rule, nil, second.ResolvedAt.Add(179*time.Second))
	if _, ok := active(); ok {
		t.Fatal("reopen delay ignored")
	}
	d.fireOrRefireAlertAt(ctx, rule, nil, second.ResolvedAt.Add(180*time.Second))
	if _, ok := active(); !ok {
		t.Fatal("reopen boundary did not fire")
	}

	placeholderStore := persistence.NewMemoryAlertInstanceStore()
	suppressed := true
	d = &Deps{AlertInstanceStore: placeholderStore, EvaluateGuardrails: func(id string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{GroupID: id, SuppressAlerts: suppressed}, nil
	}}
	rule.Targets = []alerts.RuleTarget{{GroupID: "task-group"}}
	d.fireOrRefireAlertAt(ctx, rule, nil, time.Now().UTC())
	suppressed = false
	d.fireOrRefireAlertAt(ctx, rule, nil, time.Now().UTC())
	rows, err := placeholderStore.ListAlertInstances(persistence.AlertInstanceFilter{RuleID: rule.ID, Limit: 10})
	if err != nil || len(rows) != 2 {
		t.Fatalf("placeholder release: %d/%v", len(rows), err)
	}
	statuses := map[string]int{}
	for _, row := range rows {
		statuses[row.Status]++
		if row.Status == alerts.InstanceStatusResolved && row.FirstFiredAt != nil {
			t.Fatal("placeholder invented fire history")
		}
		if row.Status == alerts.InstanceStatusFiring && row.FirstFiredAt == nil {
			t.Fatal("released fire missing anchor")
		}
	}
	if statuses[alerts.InstanceStatusResolved] != 1 || statuses[alerts.InstanceStatusFiring] != 1 {
		t.Fatalf("release statuses: %v", statuses)
	}
	failed := &Deps{AlertInstanceStore: unavailableAlertTiming{persistence.NewMemoryAlertInstanceStore()}}
	failed.fireOrRefireAlertAt(ctx, rule, nil, time.Now().UTC())
	rows, _ = failed.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{Limit: 10})
	if len(rows) != 0 {
		t.Fatal("history error bypassed holdoff")
	}
}
