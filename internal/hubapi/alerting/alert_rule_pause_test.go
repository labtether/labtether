package alerting

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

// The evaluator skips paused rules, so pausing a rule must close the alerts it
// has open; otherwise they stay firing for good.
func TestPausingAlertRuleResolvesItsOpenAlerts(t *testing.T) {
	deps := newTestAlertingDeps(t)
	rule, err := deps.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "Disk full", Kind: alerts.RuleKindMetricThreshold, Severity: alerts.SeverityHigh,
		Status: alerts.RuleStatusActive, TargetScope: alerts.TargetScopeGlobal,
		Condition: map[string]any{"metric": "disk_used_percent", "operator": ">", "value": 90},
	})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := deps.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID: rule.ID, Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels), Severity: rule.Severity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deps.AlertInstanceStore.UpdateAlertInstanceStatus(instance.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	deps.HandleAlertRuleActions(rec, httptest.NewRequest(http.MethodPatch, "/alerts/rules/"+rule.ID, strings.NewReader(`{"status":"paused"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status=%d: %s", rec.Code, rec.Body.String())
	}
	stored, ok, err := deps.AlertInstanceStore.GetAlertInstance(instance.ID)
	if err != nil || !ok {
		t.Fatalf("load instance: ok=%v err=%v", ok, err)
	}
	if stored.Status != alerts.InstanceStatusResolved || stored.ResolvedAt == nil {
		t.Fatalf("alert after pause: status=%s resolved_at=%v", stored.Status, stored.ResolvedAt)
	}
}
