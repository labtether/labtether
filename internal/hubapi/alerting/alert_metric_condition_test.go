package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestMetricRuleConditionContractThroughCreateUpdateAndEvaluation(t *testing.T) {
	for _, tc := range []struct {
		name            string
		limits          map[string]any
		healthy, breach float64
	}{
		{"console value", map[string]any{"value": 90}, 20, 95},
		{"legacy threshold", map[string]any{"threshold": "90"}, 20, 95},
		{"matching aliases", map[string]any{"value": 90, "threshold": 90}, 20, 95},
		{"explicit zero", map[string]any{"value": 0}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := metricContractDeps(t)
			rule := createMetricContractRule(t, deps, metricContractCondition(tc.limits))
			for _, sample := range []struct {
				value  float64
				status string
				active bool
			}{
				{tc.healthy, alerts.EvaluationStatusOK, false},
				{tc.breach, alerts.EvaluationStatusTriggered, true},
			} {
				appendMetricContractCPU(t, deps, sample.value)
				evaluateMetricContractRule(t, deps, rule, sample.status, sample.active)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		limits map[string]any
	}{
		{"missing limit", nil},
		{"null value", map[string]any{"value": nil}},
		{"not numeric", map[string]any{"value": "high"}},
		{"not finite", map[string]any{"value": "NaN"}},
		{"bad legacy alongside value", map[string]any{"value": 90, "threshold": "Inf"}},
		{"conflicting aliases", map[string]any{"value": 90, "threshold": 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := metricContractDeps(t)
			valid := metricContractCondition(map[string]any{"value": 90})
			rule := createMetricContractRule(t, deps, valid)
			invalid := metricContractCondition(tc.limits)
			created := metricContractRequest(t, deps, http.MethodPost, "/alerts/rules", metricContractPayload(invalid))
			if created.Code != http.StatusBadRequest || !strings.Contains(created.Body.String(), "metric condition") {
				t.Fatalf("invalid create was not explicit: %d %s", created.Code, created.Body.String())
			}
			rows, err := deps.AlertStore.ListAlertRules(persistence.AlertRuleFilter{Limit: 10})
			if err != nil || len(rows) != 1 {
				t.Fatalf("invalid create wrote a rule: rows=%d err=%v", len(rows), err)
			}
			updated := metricContractRequest(t, deps, http.MethodPatch, "/alerts/rules/"+rule.ID,
				map[string]any{"condition": invalid})
			if updated.Code != http.StatusBadRequest || !strings.Contains(updated.Body.String(), "metric condition") {
				t.Fatalf("invalid update was not explicit: %d %s", updated.Code, updated.Body.String())
			}
			stored, found, err := deps.AlertStore.GetAlertRule(rule.ID)
			if err != nil || !found || !reflect.DeepEqual(stored.Condition, rule.Condition) {
				t.Fatalf("invalid update changed rule: found=%v err=%v", found, err)
			}

			appendMetricContractCPU(t, deps, 95)
			evaluateMetricContractRule(t, deps, rule, alerts.EvaluationStatusTriggered, true)
			// Model a malformed rule already persisted before request validation.
			stored, err = deps.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &invalid})
			if err != nil {
				t.Fatal(err)
			}
			evaluateMetricContractRule(t, deps, stored, alerts.EvaluationStatusError, true)
		})
	}
}

func metricContractDeps(t *testing.T) *Deps {
	t.Helper()
	deps := newTestAlertingDeps(t)
	if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "metric-contract-node", Type: "host", Status: "online", Source: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	return deps
}

func metricContractCondition(limits map[string]any) map[string]any {
	condition := map[string]any{"metric": "cpu_used_percent", "operator": ">", "aggregate": "max"}
	for key, value := range limits {
		condition[key] = value
	}
	return condition
}

func metricContractPayload(condition map[string]any) map[string]any {
	return map[string]any{
		"name": "CPU contract", "kind": "metric_threshold", "severity": "high", "target_scope": "asset",
		"targets": []map[string]any{{"asset_id": "metric-contract-node"}}, "condition": condition,
		"window_seconds": 300, "cooldown_seconds": 300, "reopen_after_seconds": 120, "evaluation_interval_seconds": 30,
	}
}

func metricContractRequest(t *testing.T, deps *Deps, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if path == "/alerts/rules" {
		deps.HandleAlertRules(rec, req)
	} else {
		deps.HandleAlertRuleActions(rec, req)
	}
	return rec
}

func createMetricContractRule(t *testing.T, deps *Deps, condition map[string]any) alerts.Rule {
	t.Helper()
	rec := metricContractRequest(t, deps, http.MethodPost, "/alerts/rules", metricContractPayload(condition))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Rule.ID == "" {
		t.Fatalf("decode created rule: %v", err)
	}
	return result.Rule
}

func appendMetricContractCPU(t *testing.T, deps *Deps, value float64) {
	t.Helper()
	if err := deps.TelemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{{
		AssetID: "metric-contract-node", Metric: "cpu_used_percent", Unit: "percent", Value: value,
		CollectedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
}

func evaluateMetricContractRule(t *testing.T, deps *Deps, rule alerts.Rule, status string, active bool) {
	t.Helper()
	before, err := deps.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || len(before) >= 10 {
		t.Fatalf("read evaluation baseline: rows=%d err=%v", len(before), err)
	}
	seen := make(map[string]bool, len(before))
	for _, row := range before {
		seen[row.ID] = true
	}
	deps.EvaluateSingleRule(context.Background(), rule, nil)
	rows, err := deps.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || len(rows) != len(before)+1 {
		t.Fatalf("expected one recorded evaluation: before=%d rows=%+v err=%v", len(before), rows, err)
	}
	var added []alerts.Evaluation
	for _, row := range rows {
		if !seen[row.ID] {
			added = append(added, row)
		}
	}
	if len(added) != 1 || added[0].Status != status {
		t.Fatalf("new evaluation status %s: added=%+v", status, added)
	}
	if status == alerts.EvaluationStatusError && !strings.Contains(added[0].Error, "metric condition") {
		t.Fatalf("invalid stored condition did not record the error: %+v", added[0])
	}
	inst, found, err := deps.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
	if err != nil || found != active || (found && inst.Status != alerts.InstanceStatusFiring) {
		t.Fatalf("active instance: found=%v status=%s err=%v", found, inst.Status, err)
	}
}
