package alerting

import (
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
)

// Both stores cap this listing at 500. A full page cannot prove completeness,
// so uncertainty suppresses the send rather than leaking a matching alert.
const notificationSilenceScanLimit = 500

func (d *Deps) isAlertSilencedAtDelivery(rule alerts.Rule) bool {
	if strings.TrimSpace(rule.ID) == "" || d.AlertInstanceStore == nil {
		return true
	}
	silences, err := d.AlertInstanceStore.ListAlertSilences(notificationSilenceScanLimit, true)
	if err != nil || len(silences) >= notificationSilenceScanLimit {
		return true
	}
	for _, silence := range silences {
		if matchesSilence(rule, silence) {
			return true
		}
	}
	return false
}

func (d *Deps) notificationRetrySuppressed(rec notifications.Record, payload map[string]any) bool {
	if d.maintenanceSuppressesGroupIDs(notificationPayloadGroupIDs(payload)) {
		return true
	}
	// Incident delivery has its own identity and APNs-only policy. It must not
	// be treated as an alert with missing labels.
	if isIncidentNotificationPayload(payload) {
		return false
	}
	rule, ok := d.alertRuleForRetrySuppression(rec, payload)
	return !ok || d.isAlertSilencedAtDelivery(rule)
}

func (d *Deps) alertRuleForRetrySuppression(rec notifications.Record, payload map[string]any) (alerts.Rule, bool) {
	instanceID := strings.TrimSpace(rec.AlertInstanceID)
	if instanceID == "" {
		return alerts.Rule{}, false
	}
	for _, key := range []string{"alert_instance_id", "alert_id"} {
		if raw, present := payload[key]; present {
			id, valid := raw.(string)
			if !valid || strings.TrimSpace(id) != instanceID {
				return alerts.Rule{}, false
			}
		}
	}
	ruleID, _ := payload["rule_id"].(string)
	ruleID = strings.TrimSpace(ruleID)
	labels, labelsKnown := notificationAlertLabels(payload["labels"])
	if ruleID != "" && labelsKnown {
		// Modern records preserve the alert's event labels. A later rule edit
		// must not erase the matching identity of a saved notification.
		return alerts.Rule{ID: ruleID, Labels: labels}, true
	}
	// Legacy records may predate the label snapshot. Resolve their exact
	// instance and rule; missing/deleted/unreadable identity leaves them due.
	if d.AlertInstanceStore == nil || d.AlertStore == nil {
		return alerts.Rule{}, false
	}
	instance, found, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
	if err != nil || !found || instance.ID != instanceID || strings.TrimSpace(instance.RuleID) == "" ||
		(ruleID != "" && ruleID != instance.RuleID) {
		return alerts.Rule{}, false
	}
	rule, found, err := d.AlertStore.GetAlertRule(instance.RuleID)
	if err != nil || !found || rule.ID != instance.RuleID || d.isAlertMaintenanceSuppressed(rule) {
		return alerts.Rule{}, false
	}
	return rule, true
}

func notificationAlertLabels(raw any) (map[string]string, bool) {
	switch values := raw.(type) {
	case map[string]string:
		return values, values != nil
	case map[string]any:
		if values == nil {
			return nil, false
		}
		labels := make(map[string]string, len(values))
		for key, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			labels[key] = text
		}
		return labels, true
	default:
		return nil, false
	}
}
