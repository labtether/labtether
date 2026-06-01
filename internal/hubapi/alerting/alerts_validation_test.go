package alerting

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

func TestValidateCreateAlertRuleRequestRejectsOutOfRangeDurations(t *testing.T) {
	req := alerts.CreateRuleRequest{
		Name:          "CPU",
		Kind:          alerts.RuleKindMetricThreshold,
		Severity:      alerts.SeverityHigh,
		TargetScope:   alerts.TargetScopeGlobal,
		WindowSeconds: alerts.MaxDurationSeconds + 1,
		Condition:     map[string]any{"metric": "cpu", "operator": ">", "threshold": 90},
	}

	err := ValidateCreateAlertRuleRequest(req)
	if err == nil || !strings.Contains(err.Error(), "window_seconds is out of range") {
		t.Fatalf("expected out-of-range window error, got: %v", err)
	}
}

func TestValidateUpdateAlertRuleRequestRejectsOutOfRangeDurations(t *testing.T) {
	value := alerts.MaxDurationSeconds + 1
	err := ValidateUpdateAlertRuleRequest(alerts.Rule{TargetScope: alerts.TargetScopeGlobal}, alerts.UpdateRuleRequest{
		EvaluationIntervalSeconds: &value,
	})
	if err == nil || !strings.Contains(err.Error(), "evaluation_interval_seconds is out of range") {
		t.Fatalf("expected out-of-range evaluation interval error, got: %v", err)
	}
}
