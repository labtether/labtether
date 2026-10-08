package alerting

import (
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

func buildAlertNotificationPayload(rule alerts.Rule, instanceID, state string, groupIDs map[string]struct{}) map[string]any {
	now := time.Now().UTC()
	alertID := strings.TrimSpace(instanceID)
	targets := make([]map[string]string, 0, len(rule.Targets))
	for _, target := range rule.Targets {
		targets = append(targets, map[string]string{
			"id":       strings.TrimSpace(target.ID),
			"asset_id": strings.TrimSpace(target.AssetID),
			"group_id": strings.TrimSpace(target.GroupID),
		})
	}

	groupList := make([]string, 0, len(groupIDs))
	for groupID := range groupIDs {
		groupList = append(groupList, groupID)
	}
	sort.Strings(groupList)

	title := fmt.Sprintf("[%s] %s (%s)", strings.ToUpper(strings.TrimSpace(rule.Severity)), strings.TrimSpace(rule.Name), strings.TrimSpace(state))
	description := strings.TrimSpace(rule.Description)
	if description == "" {
		description = "No description provided."
	}

	payload := map[string]any{
		"event":             "alert." + strings.TrimSpace(state),
		"state":             strings.TrimSpace(state),
		"alert_instance_id": alertID,
		"alert_id":          alertID,
		"rule_id":           strings.TrimSpace(rule.ID),
		"rule_name":         strings.TrimSpace(rule.Name),
		"rule_kind":         strings.TrimSpace(rule.Kind),
		"severity":          strings.TrimSpace(rule.Severity),
		"description":       description,
		"title":             title,
		"text":              fmt.Sprintf("%s\nRule: %s (%s)\nState: %s\nInstance: %s\nOccurred: %s", description, rule.Name, rule.ID, state, instanceID, now.Format(time.RFC3339)),
		"occurred_at":       now.Format(time.RFC3339),
		"target_scope":      strings.TrimSpace(rule.TargetScope),
		"group_ids":         groupList,
		"targets":           targets,
		"labels":            cloneAlertLabels(rule.Labels),
		"deep_link":         fmt.Sprintf("labtether://alerts/%s", alertID),
	}
	if strings.EqualFold(strings.TrimSpace(state), "firing") {
		payload["apns_category"] = "LT_ALERT_ACTIONS"
	}
	return payload
}

func buildEmailNotificationPayload(config map[string]any, payload map[string]any) (map[string]any, error) {
	to := firstNonBlank(
		configString(config, "to"),
		configString(config, "recipients"),
		configString(config, "email_to"),
	)
	if to == "" {
		return nil, fmt.Errorf("email channel config missing recipient address")
	}

	severity := strings.ToUpper(payloadString(payload, "severity"))
	ruleName := firstNonBlank(
		payloadString(payload, "rule_name"),
		payloadString(payload, "title"),
		"LabTether notification",
	)
	state := payloadString(payload, "state")
	subject := ruleName
	if severity != "" {
		subject = fmt.Sprintf("[%s] %s", severity, subject)
	}
	if state != "" {
		subject = fmt.Sprintf("%s (%s)", subject, state)
	}
	if prefix := configString(config, "subject_prefix"); prefix != "" {
		subject = strings.TrimSpace(prefix + " " + subject)
	}

	body := payloadString(payload, "text")
	if body == "" {
		body = fmt.Sprintf("Alert %s for rule %s (%s)", state, ruleName, severity)
	}

	return map[string]any{
		"to":      to,
		"subject": subject,
		"body":    body,
	}, nil
}

func buildSlackNotificationPayload(payload map[string]any) map[string]any {
	return map[string]any{
		"title": payloadString(payload, "title"),
		"text":  payloadString(payload, "text"),
	}
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(notificationAnyToString(payload[key]))
}

func notificationPayloadBool(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	switch typed := payload[key].(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	return strings.TrimSpace(notificationAnyToString(config[key]))
}

func notificationAnyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return ""
		}
		if typed >= minInt64AsFloat && typed < maxInt64ExclusiveAsFloat && math.Trunc(typed) == typed {
			return fmt.Sprintf("%d", int64(typed))
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

const (
	minInt64AsFloat          = -9223372036854775808.0
	maxInt64ExclusiveAsFloat = 9223372036854775808.0
)

func cloneAlertLabels(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
