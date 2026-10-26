package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestManualAlertPreviewEvaluatesConditionWithoutChangingInstances(t *testing.T) {
	for _, tc := range []struct {
		name, scope, ruleStatus, wantStatus string
		metricValue                         float64
		limit                               any
		wantTriggered                       int
	}{
		{"healthy target", alerts.TargetScopeAsset, alerts.RuleStatusActive, alerts.EvaluationStatusOK, 20, 90, 0},
		{"actual breach", alerts.TargetScopeAsset, alerts.RuleStatusActive, alerts.EvaluationStatusTriggered, 95, 90, 1},
		{"global breach without explicit targets", alerts.TargetScopeGlobal, alerts.RuleStatusActive, alerts.EvaluationStatusTriggered, 95, 90, 1},
		{"paused", alerts.TargetScopeAsset, alerts.RuleStatusPaused, alerts.EvaluationStatusSuppressed, 95, 90, 0},
		{"malformed stored condition", alerts.TargetScopeAsset, alerts.RuleStatusActive, alerts.EvaluationStatusError, 95, "invalid", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestAlertingDeps(t)
			if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
				AssetID: "preview-node", Type: "host", Source: "agent", Status: "online",
			}); err != nil {
				t.Fatal(err)
			}
			if err := deps.TelemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{{
				AssetID: "preview-node", Metric: "cpu_used_percent", Unit: "percent", Value: tc.metricValue, CollectedAt: time.Now().UTC(),
			}}); err != nil {
				t.Fatal(err)
			}
			targets := []alerts.RuleTargetInput{{AssetID: "preview-node"}}
			if tc.scope == alerts.TargetScopeGlobal {
				targets = nil
			}
			// Persist directly to include legacy malformed rows that new request
			// validation rejects; the preview must report an evaluation error.
			rule, err := deps.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
				Name: "Preview", Kind: alerts.RuleKindMetricThreshold, Severity: alerts.SeverityHigh,
				Status: tc.ruleStatus, TargetScope: tc.scope, Targets: targets,
				Condition: map[string]any{"metric": "cpu_used_percent", "operator": ">", "value": tc.limit},
			})
			if err != nil {
				t.Fatal(err)
			}
			original, err := deps.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
				RuleID: rule.ID, Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels), Severity: rule.Severity,
			})
			if err != nil {
				t.Fatal(err)
			}
			deps.Broadcast = func(event string, _ map[string]any) {
				t.Errorf("preview must not broadcast an instance transition: %s", event)
			}
			requested := time.Now().UTC().Add(24 * time.Hour)
			body, err := json.Marshal(map[string]any{"at": requested})
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now().UTC()
			rec := httptest.NewRecorder()
			deps.HandleAlertRuleActions(rec, httptest.NewRequest(http.MethodPost,
				"/alerts/rules/"+rule.ID+"/test", bytes.NewReader(body)))
			ended := time.Now().UTC()
			if rec.Code != http.StatusOK {
				t.Fatalf("preview status=%d: %s", rec.Code, rec.Body.String())
			}
			var response struct {
				Evaluation alerts.Evaluation `json:"evaluation"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			evaluation := response.Evaluation
			if evaluation.Status != tc.wantStatus || evaluation.TriggeredCount != tc.wantTriggered {
				t.Fatalf("preview status=%s count=%d, want %s/%d", evaluation.Status, evaluation.TriggeredCount, tc.wantStatus, tc.wantTriggered)
			}
			if (evaluation.Error != "") != (tc.wantStatus == alerts.EvaluationStatusError) {
				t.Fatalf("unexpected preview error: %q", evaluation.Error)
			}
			if evaluation.EvaluatedAt.Before(started) || evaluation.EvaluatedAt.After(ended) || evaluation.Details["requested_at"] == nil {
				t.Fatal("current evaluation was backdated/forward-dated or lost its request annotation")
			}
			stored, found, err := deps.AlertStore.GetAlertRule(rule.ID)
			if err != nil || !found || stored.LastEvaluatedAt == nil || !stored.LastEvaluatedAt.Equal(evaluation.EvaluatedAt) {
				t.Fatalf("actual evaluation timestamp not retained: found=%v err=%v", found, err)
			}
			history, err := deps.AlertStore.ListAlertEvaluations(rule.ID, 10)
			if err != nil || len(history) != 1 || history[0].ID != evaluation.ID || history[0].Status != tc.wantStatus {
				t.Fatalf("preview history differs: %+v %v", history, err)
			}
			instances, err := deps.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{RuleID: rule.ID, Limit: 10})
			if err != nil || len(instances) != 1 || !reflect.DeepEqual(instances[0], original) {
				t.Fatalf("preview created, resolved or refired an instance: %+v %v", instances, err)
			}
		})
	}
}
