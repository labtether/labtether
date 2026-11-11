package persistence

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

// Every rule kind and target scope the Hub accepts must fit the database's
// alert_rules checks. synthetic_check and group did not, so a health-check
// rule or a rule for a group failed with "An internal error occurred.".
func TestAlertRuleChecksAllowEveryKindAndScopeTheHubAccepts(t *testing.T) {
	all := postgresSchemaMigrations()
	last := all[len(all)-1]
	sql := ""
	for _, m := range all {
		if m.Name == "alert_rule_kind_and_scope_checks" {
			sql = strings.Join(m.Statements, "\n")
		}
	}
	if sql == "" || last.Version < 109 {
		t.Fatalf("alert_rule_kind_and_scope_checks migration missing (last %d %s)", last.Version, last.Name)
	}
	for _, kind := range []string{alerts.RuleKindMetricThreshold, alerts.RuleKindMetricDeadman, alerts.RuleKindHeartbeatStale,
		alerts.RuleKindLogPattern, alerts.RuleKindComposite, alerts.RuleKindSyntheticCheck} {
		if alerts.NormalizeRuleKind(kind) != kind || !strings.Contains(sql, "'"+kind+"'") {
			t.Fatalf("rule kind %q is accepted by the Hub but not by the alert_rules check", kind)
		}
	}
	for _, scope := range []string{alerts.TargetScopeAsset, alerts.TargetScopeGroup, alerts.TargetScopeGlobal} {
		if alerts.NormalizeTargetScope(scope) != scope || !strings.Contains(sql, "'"+scope+"'") {
			t.Fatalf("target scope %q is accepted by the Hub but not by the alert_rules check", scope)
		}
	}
}
