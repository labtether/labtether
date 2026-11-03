package alerting

import (
	"strings"

	"github.com/labtether/labtether/internal/actions"
)

// timelineNames are the names an issue's timeline line can use. The store
// fills what it knows; a line without a name reads without it, never with
// an ID.
type timelineNames struct {
	rule, device, plan, actor string
}

// alertFiredWords: "High CPU fired on nas01".
func alertFiredWords(rule, device string) string {
	line := "An alert fired"
	if rule = strings.TrimSpace(rule); rule != "" {
		line = rule + " fired"
	}
	if device = strings.TrimSpace(device); device != "" {
		line += " on " + device
	}
	return line
}

// failedRunWords: "Command failed on nas01".
func failedRunWords(runType, device string) string {
	line := "Command failed"
	if runType == actions.RunTypeConnectorAction {
		line = "Integration action failed"
	}
	if device = strings.TrimSpace(device); device != "" {
		line += " on " + device
	}
	return line
}

// failedUpdateWords: "Update failed: Weekly patches".
func failedUpdateWords(plan string) string {
	if plan = strings.TrimSpace(plan); plan != "" {
		return "Update failed: " + plan
	}
	return "Update failed"
}

// auditWords reads an audit type as words, with the person when known:
// "terminal.session.created" by alice → "Terminal session created by alice".
func auditWords(eventType, actor string) string {
	words := strings.ToLower(strings.Join(strings.FieldsFunc(eventType, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == ' '
	}), " "))
	if words == "" {
		words = "change"
	}
	line := strings.ToUpper(words[:1]) + words[1:]
	if actor = strings.TrimSpace(actor); actor != "" {
		line += " by " + actor
	}
	return line
}
