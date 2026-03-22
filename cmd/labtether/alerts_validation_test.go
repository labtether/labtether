package main

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

func TestValidateCreateAlertRuleRequestAcceptsSyntheticCheckKind(t *testing.T) {
	req := alerts.CreateRuleRequest{
		Name:        "Synthetic Check Rule",
		Kind:        alerts.RuleKindSyntheticCheck,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeGlobal,
		Condition: map[string]any{
			"check_id":             "check-1",
			"consecutive_failures": float64(3),
		},
	}

	if err := validateCreateAlertRuleRequest(req); err != nil {
		t.Fatalf("expected synthetic_check kind to validate, got error: %v", err)
	}
}

func TestValidateUpdateAlertRuleRequestAllowsZeroWindowAndEvaluationInterval(t *testing.T) {
	existing := alerts.Rule{TargetScope: alerts.TargetScopeAsset}
	zero := 0
	req := alerts.UpdateRuleRequest{
		WindowSeconds:             &zero,
		EvaluationIntervalSeconds: &zero,
	}

	if err := validateUpdateAlertRuleRequest(existing, req); err != nil {
		t.Fatalf("expected zero values to be accepted for update, got error: %v", err)
	}
}

func TestValidateUpdateAlertRuleRequestRejectsNegativeWindowAndEvaluationInterval(t *testing.T) {
	existing := alerts.Rule{TargetScope: alerts.TargetScopeAsset}
	negative := -1

	err := validateUpdateAlertRuleRequest(existing, alerts.UpdateRuleRequest{
		WindowSeconds: &negative,
	})
	if err == nil || !strings.Contains(err.Error(), "window_seconds must be >= 0") {
		t.Fatalf("expected window_seconds validation error, got: %v", err)
	}

	err = validateUpdateAlertRuleRequest(existing, alerts.UpdateRuleRequest{
		EvaluationIntervalSeconds: &negative,
	})
	if err == nil || !strings.Contains(err.Error(), "evaluation_interval_seconds must be >= 0") {
		t.Fatalf("expected evaluation_interval_seconds validation error, got: %v", err)
	}
}
