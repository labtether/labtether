package alerting

import (
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
	"strings"
	"time"
)

func pushDeviceAllowsPayload(device persistence.PushDevice, payload map[string]any) bool {
	return pushDeviceAllowsPayloadAt(device, payload, time.Now())
}

func pushDeviceAllowsPayloadAt(device persistence.PushDevice, payload map[string]any, now time.Time) bool {
	return pushDeviceAllowsPayloadAtWithQuietHours(device, payload, now, true)
}

func pushDeviceAllowsDigestPayloadAt(device persistence.PushDevice, payload map[string]any, now time.Time) bool {
	return pushDeviceAllowsPayloadAtWithQuietHours(device, payload, now, false)
}

func pushDeviceAllowsPayloadAtWithQuietHours(device persistence.PushDevice, payload map[string]any, now time.Time, enforceQuietHours bool) bool {
	severityRank := pushSeverityRank(payloadString(payload, "severity"))
	minimumRank := pushSeverityRank(device.MinimumSeverity)
	if minimumRank == 0 {
		minimumRank = pushSeverityRank("warning")
	}
	if severityRank < minimumRank {
		return false
	}
	if enforceQuietHours && device.QuietHoursEnabled &&
		severityRank < pushSeverityRank("critical") &&
		pushDeviceIsInQuietHours(device, now) {
		return false
	}

	event := strings.ToLower(payloadString(payload, "event"))
	isIncident := strings.HasPrefix(event, "incident.") || payloadString(payload, "incident_id") != ""
	isAlert := strings.HasPrefix(event, "alert.") || payloadString(payload, "alert_id") != "" || payloadString(payload, "alert_instance_id") != ""
	category := strings.ToLower(strings.TrimSpace(device.PushCategory))
	if category == "" {
		category = "critical_only"
	}
	switch category {
	case "critical_only":
		if isIncident || severityRank < pushSeverityRank("high") {
			return false
		}
	case "all_alerts":
		if isIncident || !isAlert {
			return false
		}
	case "alerts_and_incidents":
		if !isAlert && !isIncident {
			return false
		}
	default:
		return false
	}

	if isNodeOfflinePush(payload) {
		return device.NotifyNodeOffline
	}
	if isServiceDownPush(payload) {
		return device.NotifyServiceDown
	}
	if isAlert && severityRank >= pushSeverityRank("high") {
		return device.NotifyCriticalAlerts
	}
	return true
}

func pushDeviceIsInQuietHours(device persistence.PushDevice, now time.Time) bool {
	location, err := time.LoadLocation(strings.TrimSpace(device.TimeZone))
	if err != nil {
		// Legacy or corrupt registrations fail open: dropping a critical fleet
		// signal without a trustworthy local clock would be worse than delivery.
		return false
	}
	local := now.In(location)
	currentMinutes := local.Hour()*60 + local.Minute()
	start := device.QuietHoursStartMinutes
	end := device.QuietHoursEndMinutes
	if start < 0 || start > 1439 || end < 0 || end > 1439 || start == end {
		return false
	}
	if start < end {
		return currentMinutes >= start && currentMinutes < end
	}
	return currentMinutes >= start || currentMinutes < end
}

func pushSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 4
	case "high", "major":
		return 3
	case "warning", "medium":
		return 2
	case "info", "low":
		return 1
	default:
		return 0
	}
}

func isNodeOfflinePush(payload map[string]any) bool {
	if strings.EqualFold(payloadString(payload, "rule_kind"), alerts.RuleKindHeartbeatStale) {
		return true
	}
	signal := strings.ToLower(payloadMapString(payload, "labels", "signal"))
	return signal == "offline" || signal == "node_offline" || signal == "agent_offline"
}

func isServiceDownPush(payload map[string]any) bool {
	signal := strings.ToLower(payloadMapString(payload, "labels", "signal"))
	return strings.Contains(signal, "service_down") || strings.Contains(signal, "down_transition")
}

func payloadMapString(payload map[string]any, mapKey, valueKey string) string {
	if payload == nil {
		return ""
	}
	switch values := payload[mapKey].(type) {
	case map[string]string:
		return strings.TrimSpace(values[valueKey])
	case map[string]any:
		return strings.TrimSpace(notificationAnyToString(values[valueKey]))
	default:
		return ""
	}
}
