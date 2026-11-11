package alerting

import (
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func buildAlertNotificationPayload(rule alerts.Rule, instanceID, state string, groupIDs map[string]struct{}, devices []string) map[string]any {
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

	name := strings.TrimSpace(rule.Name)
	state = strings.TrimSpace(state)
	onDevices := alertDeviceSummary(devices)
	subject := name
	if onDevices != "" {
		subject += " on " + onDevices
	}
	title := singleLine(fmt.Sprintf("[%s] %s (%s)", strings.ToUpper(strings.TrimSpace(rule.Severity)), subject, state))

	payload := map[string]any{
		"event":             "alert." + state,
		"state":             state,
		"alert_instance_id": alertID,
		"alert_id":          alertID,
		"rule_id":           strings.TrimSpace(rule.ID),
		"rule_name":         name,
		"rule_kind":         strings.TrimSpace(rule.Kind),
		"severity":          strings.TrimSpace(rule.Severity),
		"description":       strings.TrimSpace(rule.Description),
		"devices":           append([]string(nil), devices...),
		"device_summary":    onDevices,
		"title":             title,
		"text":              alertNotificationText(rule, state, onDevices, now),
		"occurred_at":       now.Format(time.RFC3339),
		"target_scope":      strings.TrimSpace(rule.TargetScope),
		"group_ids":         groupList,
		"targets":           targets,
		"labels":            cloneAlertLabels(rule.Labels),
		"deep_link":         fmt.Sprintf("labtether://alerts/%s", alertID),
	}
	if strings.EqualFold(state, "firing") {
		payload["apns_category"] = "LT_ALERT_ACTIONS"
	}
	return payload
}

// alertNotificationText is what a person reads in an email, Slack, ntfy,
// Gotify or a push: what happened and where, then the rule's own words, the
// severity and the time. Hub-made IDs stay in the payload's fields only.
func alertNotificationText(rule alerts.Rule, state, onDevices string, now time.Time) string {
	headline := fmt.Sprintf("%s is %s", strings.TrimSpace(rule.Name), state)
	if onDevices != "" {
		headline += " on " + onDevices
	}
	lines := []string{headline + "."}
	if description := strings.TrimSpace(rule.Description); description != "" {
		lines = append(lines, description)
	}
	if severity := strings.TrimSpace(rule.Severity); severity != "" {
		lines = append(lines, "Severity: "+severity)
	}
	lines = append(lines, "Time: "+now.Format("2006-01-02 15:04 MST"))
	return strings.Join(lines, "\n")
}

// alertDeviceSummary names up to three devices, then a count: "nas01",
// "nas01, nas02 and nas03", "nas01, nas02 and 4 more".
func alertDeviceSummary(devices []string) string {
	switch n := len(devices); {
	case n == 0:
		return ""
	case n == 1:
		return devices[0]
	case n <= 3:
		return strings.Join(devices[:n-1], ", ") + " and " + devices[n-1]
	default:
		return fmt.Sprintf("%s and %d more", strings.Join(devices[:2], ", "), n-2)
	}
}

// alertNotificationDevices names the devices an alert is for, the way the
// Alerts page shows them: the instance's asset_id label, then the devices it
// fired on. A device that no longer exists is left out.
func (d *Deps) alertNotificationDevices(instanceID string) []string {
	if d == nil || d.AlertInstanceStore == nil || d.AssetStore == nil || strings.TrimSpace(instanceID) == "" {
		return nil
	}
	inst, ok, err := d.AlertInstanceStore.GetAlertInstance(strings.TrimSpace(instanceID))
	if err != nil || !ok {
		return nil
	}
	names := []string{}
	for _, id := range alertInstanceDeviceIDs(inst) {
		entry, found, getErr := d.AssetStore.GetAsset(id)
		if getErr != nil || !found {
			continue
		}
		name := cleanNotificationDeviceName(entry.Name)
		if name == "" && !hubMadeID.MatchString(id) {
			name = cleanNotificationDeviceName(id)
		}
		if name == "" {
			name = "unnamed device"
		}
		names = append(names, name)
	}
	return names
}

// hubMadeID matches the IDs the Hub makes (idgen.New), which never go into
// text a person reads; a host name used as a device ID may.
var hubMadeID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}_[0-9]{10,}(_[0-9]+)?$`)

const maxNotificationDeviceNameRunes = 64

// cleanNotificationDeviceName makes a device name safe for a title, an email
// subject or an HTTP header: an agent or a connector sets the name, so it may
// hold line breaks or other control characters. Those and runs of white space
// become one space, and a long name is cut with "…".
func cleanNotificationDeviceName(raw string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	runes := []rune(b.String())
	if len(runes) > maxNotificationDeviceNameRunes {
		return strings.TrimSpace(string(runes[:maxNotificationDeviceNameRunes-1])) + "…"
	}
	return string(runes)
}

// singleLine turns any line break left in a title or subject into a space.
func singleLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
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
	if onDevices := payloadString(payload, "device_summary"); onDevices != "" {
		subject += " on " + onDevices
	}
	if severity != "" {
		subject = fmt.Sprintf("[%s] %s", severity, subject)
	}
	if state != "" {
		subject = fmt.Sprintf("%s (%s)", subject, state)
	}
	if prefix := configString(config, "subject_prefix"); prefix != "" {
		subject = strings.TrimSpace(prefix + " " + subject)
	}
	subject = singleLine(subject)

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
