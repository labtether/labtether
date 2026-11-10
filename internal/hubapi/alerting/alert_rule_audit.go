package alerting

import (
	"net/http"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
)

const (
	alertRuleCreatedAuditType = "alert.rule.created"
	alertRuleUpdatedAuditType = "alert.rule.updated"
	alertRuleDeletedAuditType = "alert.rule.deleted"
	alertRuleAuditWarning     = "api warning: failed to append alert rule audit event"
)

// auditAlertRule records who created, changed or deleted an alert rule, with
// the rule's name so the row still reads after the rule is gone.
func (d *Deps) auditAlertRule(r *http.Request, eventType string, rule alerts.Rule, details map[string]any) {
	event := audit.NewEvent(eventType)
	event.ActorID = d.principalActorID(r.Context())
	event.Target = rule.ID
	event.TargetName = rule.Name
	event.Decision = "applied"
	event.Details = details
	d.appendAuditEventBestEffort(event, alertRuleAuditWarning)
}

// alertRuleAuditDetails is what a created or changed rule looks like now.
func alertRuleAuditDetails(rule alerts.Rule) map[string]any {
	return map[string]any{
		"kind":         rule.Kind,
		"severity":     rule.Severity,
		"status":       rule.Status,
		"target_scope": rule.TargetScope,
	}
}
