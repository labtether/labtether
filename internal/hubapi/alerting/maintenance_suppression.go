package alerting

import (
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

// isAlertMaintenanceSuppressed applies group suppress_alerts guardrails to an
// alert rule. Alert instances aggregate all rule targets, so a window on any
// targeted group suppresses the instance as a whole; this avoids leaking a
// group-scoped alert through a multi-target notification.
func (d *Deps) isAlertMaintenanceSuppressed(rule alerts.Rule) bool {
	return d.maintenanceSuppressesGroupIDs(d.collectRuleGroupIDs(rule))
}

func (d *Deps) maintenanceSuppressesGroupIDs(groupIDs map[string]struct{}) bool {
	if d.EvaluateGuardrails == nil || len(groupIDs) == 0 {
		return false
	}

	now := time.Now().UTC()
	for rawGroupID := range groupIDs {
		groupID := strings.TrimSpace(rawGroupID)
		if groupID == "" {
			continue
		}
		guardrails, err := d.EvaluateGuardrails(groupID, now)
		if err != nil {
			// Fail closed: a transient store failure must not leak an alert that a
			// maintenance window may be suppressing.
			log.Printf("alert evaluator: failed to evaluate maintenance suppression for group %s: %v", groupID, err)
			return true
		}
		if guardrails.SuppressAlerts {
			return true
		}
	}
	return false
}

func notificationPayloadGroupIDs(payload map[string]any) map[string]struct{} {
	out := make(map[string]struct{})
	if payload == nil {
		return out
	}
	switch values := payload["group_ids"].(type) {
	case []string:
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				out[value] = struct{}{}
			}
		}
	case []any:
		for _, value := range values {
			stringValue, ok := value.(string)
			if ok {
				if stringValue = strings.TrimSpace(stringValue); stringValue != "" {
					out[stringValue] = struct{}{}
				}
			}
		}
	}
	return out
}
