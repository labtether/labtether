package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/audit"
)

// Creating, changing or deleting an API key, a schedule, an alert rule or a
// group left only an "API request" row, which the console hides, so these
// changes did not show in the audit log. Each must write a named row
// (details.target_name), the delete row included, and a key's secret must
// never be in any row.
func TestConfigChangesWriteNamedAuditRows(t *testing.T) {
	s := newTestAPIServer(t)
	call := func(handler http.HandlerFunc, method, path, body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(contextWithPrincipal(req.Context(), "owner-1", "owner"))
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("%s %s = %d: %s", method, path, rec.Code, rec.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	field := func(m map[string]any, keys ...string) string {
		t.Helper()
		var cur any = m
		for _, key := range keys {
			next, _ := cur.(map[string]any)
			cur = next[key]
		}
		value, _ := cur.(string)
		if value == "" {
			t.Fatalf("response has no %v: %v", keys, m)
		}
		return value
	}

	key := call(s.handleAPIKeys, http.MethodPost, "/api/v2/keys", `{"name":"backup key","role":"operator","scopes":["assets:read"]}`)
	keyID, rawKey := field(key, "data", "id"), field(key, "data", "raw_key")
	call(s.handleAPIKeyActions, http.MethodPatch, "/api/v2/keys/"+keyID, `{"name":"backup key 2"}`)
	call(s.handleAPIKeyActions, http.MethodDelete, "/api/v2/keys/"+keyID, "")

	schedule := call(s.handleV2Schedules, http.MethodPost, "/api/v2/schedules", `{"name":"nightly","cron_expr":"0 2 * * *","command":"echo token=hunter2","targets":["srv1"],"enabled":false}`)
	scheduleID := field(schedule, "data", "id")
	call(s.handleV2ScheduleActions, http.MethodPatch, "/api/v2/schedules/"+scheduleID, `{"name":"nightly 2"}`)
	call(s.handleV2ScheduleActions, http.MethodDelete, "/api/v2/schedules/"+scheduleID, "")

	rule := call(s.handleAlertRules, http.MethodPost, "/alerts/rules", `{"name":"Disk nearly full","kind":"metric_threshold","severity":"high","target_scope":"global","condition":{"metric":"disk_used_percent","operator":">=","value":90,"aggregate":"max"}}`)
	ruleID := field(rule, "rule", "id")
	call(s.handleAlertRuleActions, http.MethodPatch, "/alerts/rules/"+ruleID, `{"name":"Disk full"}`)
	call(s.handleAlertRuleActions, http.MethodDelete, "/alerts/rules/"+ruleID, "")

	group := call(s.handleGroups, http.MethodPost, "/groups", `{"name":"Rack A"}`)
	groupID := field(group, "group", "id")
	call(s.handleGroupActions, http.MethodPatch, "/groups/"+groupID, `{"name":"Rack B"}`)
	call(s.handleGroupActions, http.MethodDelete, "/groups/"+groupID, "")

	events, err := s.auditStore.List(200, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	rows := map[string]audit.Event{}
	for _, event := range events {
		rows[event.Type+" "+event.Target] = event
	}
	want := map[string]string{
		"api_key.created " + keyID:       "backup key",
		"api_key.updated " + keyID:       "backup key 2",
		"api_key.deleted " + keyID:       "backup key 2",
		"schedule.created " + scheduleID: "nightly",
		"schedule.updated " + scheduleID: "nightly 2",
		"schedule.deleted " + scheduleID: "nightly 2",
		"alert.rule.created " + ruleID:   "Disk nearly full",
		"alert.rule.updated " + ruleID:   "Disk full",
		"alert.rule.deleted " + ruleID:   "Disk full",
		"group.created " + groupID:       "Rack A",
		"group.updated " + groupID:       "Rack B",
		"group.deleted " + groupID:       "Rack B",
	}
	for row, name := range want {
		event, ok := rows[row]
		if got, _ := event.Details["target_name"].(string); !ok || got != name || event.ActorID != "owner-1" {
			t.Errorf("audit row %q: present %t, name %q, actor %q; want name %q by owner-1", row, ok, got, event.ActorID, name)
		}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("encode audit: %v", err)
	}
	for label, secret := range map[string]string{"the API key's secret": rawKey, "the schedule's command": "hunter2"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("audit rows contain %s", label)
		}
	}
}
