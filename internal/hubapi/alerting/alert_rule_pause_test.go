package alerting

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

// The evaluator skips paused rules, so pausing a rule must close every alert it
// has open, beyond the first page; otherwise they stay firing for good.
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
	// More than one page of open alerts: each resolve pass handles 100.
	ids := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		instance, err := deps.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
			RuleID: rule.ID, Fingerprint: fmt.Sprintf("disk-%d", i), Severity: rule.Severity,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := deps.AlertInstanceStore.UpdateAlertInstanceStatus(instance.ID, alerts.InstanceStatusFiring); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, instance.ID)
	}

	rec := httptest.NewRecorder()
	deps.HandleAlertRuleActions(rec, httptest.NewRequest(http.MethodPatch, "/alerts/rules/"+rule.ID, strings.NewReader(`{"status":"paused"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status=%d: %s", rec.Code, rec.Body.String())
	}
	for _, id := range ids {
		stored, ok, err := deps.AlertInstanceStore.GetAlertInstance(id)
		if err != nil || !ok {
			t.Fatalf("load instance: ok=%v err=%v", ok, err)
		}
		if stored.Status != alerts.InstanceStatusResolved || stored.ResolvedAt == nil {
			t.Fatalf("alert %s after pause: status=%s resolved_at=%v", id, stored.Status, stored.ResolvedAt)
		}
	}
}
