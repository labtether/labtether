package alerting

import (
	"slices"
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
// An issue's own events use the product's words and name the status:
// "incident.updated" with a status change → "Set to Investigating by alice".
func auditWords(eventType string, details map[string]any, actor string) string {
	line := issueAuditWords(eventType, details)
	if line == "" {
		words := strings.ToLower(strings.Join(strings.FieldsFunc(eventType, func(r rune) bool {
			return r == '.' || r == '_' || r == '-' || r == ' '
		}), " "))
		if words == "" {
			words = "change"
		}
		line = strings.ToUpper(words[:1]) + words[1:]
	}
	if actor = strings.TrimSpace(actor); actor != "" {
		line += " by " + actor
	}
	return line
}

// issueWriteUpFields are the parts of an issue's write-up; a change to only
// these reads "Write-up edited".
var issueWriteUpFields = map[string]bool{
	"summary": true, "root_cause": true, "action_items": true, "lessons_learned": true,
}

// issueAuditWords is the line for an issue's own audit event, or "" for any
// other event. The audit type keeps its stored name ("incident.*"); people
// see "issue".
func issueAuditWords(eventType string, details map[string]any) string {
	switch eventType {
	case "incident.created":
		return "Opened"
	case "incident.deleted":
		return "Deleted"
	case "incident.updated":
		changed := detailStrings(details["changed"])
		if slices.Contains(changed, "status") {
			if status := statusWord(details["status"]); status != "" {
				return "Set to " + status
			}
		}
		if len(changed) > 0 && !slices.ContainsFunc(changed, func(field string) bool { return !issueWriteUpFields[field] }) {
			return "Write-up edited"
		}
		return "Issue updated"
	}
	if rest, ok := strings.CutPrefix(eventType, "incident."); ok && rest != "" {
		return "Issue " + strings.ReplaceAll(strings.ReplaceAll(rest, "_", " "), ".", " ")
	}
	return ""
}

// statusWord: "investigating" → "Investigating".
func statusWord(value any) string {
	status, _ := value.(string)
	status = strings.TrimSpace(status)
	if status == "" {
		return ""
	}
	return strings.ToUpper(status[:1]) + strings.ToLower(status[1:])
}

// detailStrings reads a string list from audit details, which hold []string
// when written and []any once read back from the database.
func detailStrings(value any) []string {
	switch list := value.(type) {
	case []string:
		return list
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}
