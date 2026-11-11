package alerting

import (
	"strings"
	"unicode"
)

const maxHistoryAlertNameRunes = 200

// historyAlertName is the rule name a sent message carried, on one line and
// bounded, or "" when the message was not for an alert rule.
func historyAlertName(payload map[string]any) string {
	name, _ := payload["rule_name"].(string)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if runes := []rune(name); len(runes) > maxHistoryAlertNameRunes {
		name = string(runes[:maxHistoryAlertNameRunes-1]) + "…"
	}
	return name
}
