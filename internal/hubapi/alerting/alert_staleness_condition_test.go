package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/assetstatus"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type stalenessDisconnectedAgents struct{}

func (stalenessDisconnectedAgents) IsConnected(string) bool { return false }

// The real status decorator marks disconnected agents offline. That must not
// hide a stale heartbeat, and unrelated telemetry must not satisfy a CPU rule.
func TestStalenessRulesHonorUIConditionsAndLegacyFallback(t *testing.T) {
	t.Run("heartbeat age with production offline view", func(t *testing.T) {
		deps, raw := stalenessContractDeps(t)
		rule := createStalenessRule(t, deps, alerts.RuleKindHeartbeatStale, 300, map[string]any{"max_stale_seconds": 60})
		raw.BackdateLastSeenAt("staleness-node", time.Now().Add(-120*time.Second))
		view, found, err := deps.AssetStore.GetAsset("staleness-node")
		if err != nil || !found || view.Status != "offline" {
			t.Fatalf("production disconnected view: %+v/%v/%v", view, found, err)
		}
		evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusTriggered, true)
		if _, err := raw.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "staleness-node", Type: "host", Source: "agent", Status: "online"}); err != nil {
			t.Fatal(err)
		}
		evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusOK, false)
	})
	t.Run("selected metric silence overrides window", func(t *testing.T) {
		deps, _ := stalenessContractDeps(t)
		rule := createStalenessRule(t, deps, alerts.RuleKindMetricDeadman, 300, map[string]any{"metric": "cpu_used_percent", "max_silence_seconds": 60})
		now := time.Now().UTC()
		appendStalenessSamples(t, deps,
			stalenessSample("staleness-node", "cpu_used_percent", now.Add(-120*time.Second)),
			stalenessSample("staleness-node", "memory_used_percent", now),
			stalenessSample("other-node", "cpu_used_percent", now),
			stalenessSample("staleness-node", "cpu_used_percent", now.Add(time.Hour)))
		evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusTriggered, true)
		appendStalenessSamples(t, deps, stalenessSample("staleness-node", "cpu_used_percent", now))
		evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusOK, false)
	})
	t.Run("explicit longer silence and noncatalog metric", func(t *testing.T) {
		deps, _ := stalenessContractDeps(t)
		rule := createStalenessRule(t, deps, alerts.RuleKindMetricDeadman, 30, map[string]any{"metric": "custom_probe_count", "max_silence_seconds": "300"})
		appendStalenessSamples(t, deps, stalenessSample("staleness-node", "custom_probe_count", time.Now().Add(-120*time.Second)))
		evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusOK, false)
	})
	t.Run("missing keys retain window and any metric", func(t *testing.T) {
		deps, raw := stalenessContractDeps(t)
		raw.BackdateLastSeenAt("staleness-node", time.Now().Add(-120*time.Second))
		appendStalenessSamples(t, deps, stalenessSample("staleness-node", "memory_used_percent", time.Now().Add(-120*time.Second)))
		for _, kind := range []string{alerts.RuleKindHeartbeatStale, alerts.RuleKindMetricDeadman} {
			for _, window := range []int{0, 300, 60} {
				// Old persisted rows may have no condition at all. Zero keeps the
				// original 300-second default, not an immediate stale result.
				rule, err := deps.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "legacy", Kind: kind, Severity: "high", TargetScope: "asset", Targets: []alerts.RuleTargetInput{{AssetID: "staleness-node"}}, WindowSeconds: &window})
				if err != nil {
					t.Fatal(err)
				}
				want := alerts.EvaluationStatusOK
				if window == 60 {
					want = alerts.EvaluationStatusTriggered
				}
				evaluateStalenessRule(t, deps, rule, want, window == 60)
			}
		}
	})
	t.Run("malformed explicit conditions do not write or resolve", func(t *testing.T) {
		for _, kind := range []string{alerts.RuleKindHeartbeatStale, alerts.RuleKindMetricDeadman} {
			field := "max_stale_seconds"
			if kind == alerts.RuleKindMetricDeadman {
				field = "max_silence_seconds"
			}
			invalid := []map[string]any{{field: nil}, {field: 0}, {field: -1}, {field: 1.5}, {field: "NaN"}, {field: alerts.MaxDurationSeconds + 1}}
			if kind == alerts.RuleKindMetricDeadman {
				invalid = append(invalid, map[string]any{"metric": " "}, map[string]any{"metric": 7}, map[string]any{"metric": nil})
			}
			for _, condition := range invalid {
				deps, raw := stalenessContractDeps(t)
				raw.BackdateLastSeenAt("staleness-node", time.Now().Add(-120*time.Second))
				rule := createStalenessRule(t, deps, kind, 300, map[string]any{field: 60})
				for _, request := range []struct {
					method, path string
					payload      map[string]any
				}{
					{http.MethodPost, "/alerts/rules", stalenessRulePayload(kind, 300, condition)},
					{http.MethodPatch, "/alerts/rules/" + rule.ID, map[string]any{"condition": condition}},
				} {
					response := metricContractRequest(t, deps, request.method, request.path, request.payload)
					if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "staleness condition") {
						t.Fatalf("invalid %s condition %v accepted: %d %s", kind, condition, response.Code, response.Body.String())
					}
				}
				stored, found, err := deps.AlertStore.GetAlertRule(rule.ID)
				rows, listErr := deps.AlertStore.ListAlertRules(persistence.AlertRuleFilter{Limit: 10})
				if err != nil || !found || listErr != nil || len(rows) != 1 || !reflect.DeepEqual(stored.Condition, rule.Condition) {
					t.Fatalf("invalid condition changed stored rules: %v/%v/%v", found, err, listErr)
				}
				evaluateStalenessRule(t, deps, rule, alerts.EvaluationStatusTriggered, true)
				stored, err = deps.AlertStore.UpdateAlertRule(rule.ID, alerts.UpdateRuleRequest{Condition: &condition})
				if err != nil {
					t.Fatal(err)
				}
				// Even without telemetry, malformed persisted conditions must
				// report error rather than return healthy and resolve an alert.
				deps.TelemetryStore = nil
				evaluateStalenessRule(t, deps, stored, alerts.EvaluationStatusError, true)
			}
		}
	})
}

func stalenessContractDeps(t *testing.T) (*Deps, *persistence.MemoryAssetStore) {
	t.Helper()
	deps := newTestAlertingDeps(t)
	raw := persistence.NewMemoryAssetStore()
	for _, id := range []string{"staleness-node", "other-node"} {
		if _, err := raw.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "host", Source: "agent", Status: "online"}); err != nil {
			t.Fatal(err)
		}
	}
	deps.AssetStore = assetstatus.NewStore(raw, stalenessDisconnectedAgents{})
	return deps, raw
}

func stalenessRulePayload(kind string, window int, condition map[string]any) map[string]any {
	return map[string]any{"name": "staleness contract", "kind": kind, "severity": "high", "target_scope": "asset", "targets": []map[string]any{{"asset_id": "staleness-node"}}, "condition": condition, "window_seconds": window}
}

func createStalenessRule(t *testing.T, deps *Deps, kind string, window int, condition map[string]any) alerts.Rule {
	t.Helper()
	response := metricContractRequest(t, deps, http.MethodPost, "/alerts/rules", stalenessRulePayload(kind, window, condition))
	if response.Code != http.StatusCreated {
		t.Fatalf("create staleness rule: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Rule alerts.Rule `json:"rule"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Rule.ID == "" {
		t.Fatalf("decode created staleness rule: %v", err)
	}
	return result.Rule
}

func stalenessSample(id, metric string, at time.Time) telemetry.MetricSample {
	return telemetry.MetricSample{AssetID: id, Metric: metric, Unit: "count", Value: 1, CollectedAt: at}
}

func appendStalenessSamples(t *testing.T, deps *Deps, samples ...telemetry.MetricSample) {
	t.Helper()
	if err := deps.TelemetryStore.AppendSamples(context.Background(), samples); err != nil {
		t.Fatal(err)
	}
}

func evaluateStalenessRule(t *testing.T, deps *Deps, rule alerts.Rule, want string, active bool) {
	t.Helper()
	before, err := deps.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || len(before) >= 10 {
		t.Fatalf("history baseline: %d/%v", len(before), err)
	}
	seen := make(map[string]bool, len(before))
	for _, row := range before {
		seen[row.ID] = true
	}
	deps.EvaluateSingleRule(context.Background(), rule, nil)
	rows, err := deps.AlertStore.ListAlertEvaluations(rule.ID, 10)
	if err != nil || len(rows) != len(before)+1 {
		t.Fatalf("history count: %d/%v", len(rows), err)
	}
	var added []alerts.Evaluation
	for _, row := range rows {
		if !seen[row.ID] {
			added = append(added, row)
		}
	}
	if len(added) != 1 || added[0].Status != want {
		t.Fatalf("new evaluation: %+v, want %s", added, want)
	}
	if want == alerts.EvaluationStatusError && !strings.Contains(added[0].Error, "staleness condition") {
		t.Fatalf("condition error missing: %+v", added[0])
	}
	instance, found, err := deps.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
	if err != nil || found != active || (found && instance.Status != alerts.InstanceStatusFiring) {
		t.Fatalf("instance: %+v/%v/%v", instance, found, err)
	}
}
