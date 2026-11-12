package alerting

import "testing"

// The issue page shows these lines as its timeline. People call these
// "issues" and need to see which status was set; the stored audit type
// ("incident.updated") must not show through.
func TestIssueTimelineWords(t *testing.T) {
	cases := []struct {
		name, eventType string
		details         map[string]any
		want            string
	}{
		{"opened", "incident.created", map[string]any{"status": "open"}, "Opened by sam"},
		{"status set", "incident.updated", map[string]any{"changed": []string{"status"}, "status": "investigating"}, "Set to Investigating by sam"},
		{"status set, read back from the database", "incident.updated", map[string]any{"changed": []any{"status", "assignee"}, "status": "mitigated"}, "Set to Mitigated by sam"},
		{"write-up only", "incident.updated", map[string]any{"changed": []any{"action_items", "lessons_learned"}, "status": "closed"}, "Write-up edited by sam"},
		{"other change", "incident.updated", map[string]any{"changed": []any{"severity"}, "status": "open"}, "Issue updated by sam"},
		{"other event keeps its words", "terminal.session.created", nil, "Terminal session created by sam"},
	}
	for _, tc := range cases {
		if got := auditWords(tc.eventType, tc.details, "sam"); got != tc.want {
			t.Errorf("%s: auditWords(%q) = %q, want %q", tc.name, tc.eventType, got, tc.want)
		}
	}
	if got := auditWords("incident.updated", nil, ""); got != "Issue updated" {
		t.Errorf("no details, no person: got %q", got)
	}
}
