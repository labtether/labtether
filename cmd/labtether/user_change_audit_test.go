package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/persistence"
)

// editableMaintenanceStore adds the edit and delete that the shared double
// leaves out.
type editableMaintenanceStore struct {
	maintenanceOnlyGroupMaintenanceStore
}

func (m *editableMaintenanceStore) UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	for i, window := range m.windows[groupID] {
		if window.ID == windowID {
			window.Name = req.Name
			m.windows[groupID][i] = window
			return window, nil
		}
	}
	return groupmaintenance.MaintenanceWindow{}, groupmaintenance.ErrMaintenanceWindowNotFound
}

func (m *editableMaintenanceStore) DeleteGroupMaintenanceWindow(groupID, _ string) error {
	delete(m.windows, groupID)
	return nil
}

// Device edits, maintenance windows, alert acknowledge / resolve / silence,
// alert routes, issue edits, update plans and terminal bookmarks left only an
// "API request" row, which the console hides, so none of them showed in the
// audit log. Each must write a named row (details.target_name), the delete row
// included, and no row may carry a note's text or a bookmark's user name. An
// alert row must also say which device the alert was for: one rule fires on
// many devices, and the rule's name alone does not tell them apart.
func TestUserChangesWriteNamedAuditRows(t *testing.T) {
	s := newTestAPIServer(t)
	s.notificationStore = newNotificationStoreStub()
	s.terminalBookmarkStore = newMemoryTerminalBookmarkStore()
	s.groupMaintenanceStore = &editableMaintenanceStore{}
	call := func(handler http.HandlerFunc, method, path, body string) map[string]any {
		t.Helper()
		return auditedCall(t, handler, method, path, body)
	}
	field := func(m map[string]any, keys ...string) string {
		t.Helper()
		return answerField(t, m, keys...)
	}

	if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "nas01", Type: "host", Name: "nas01", Source: "agent", Status: "online"}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	groupID := field(call(s.handleGroups, http.MethodPost, "/groups", `{"name":"Rack A"}`), "group", "id")
	call(s.handleAssetActions, http.MethodPatch, "/assets/nas01", `{"name":"nas-main","group_id":"`+groupID+`"}`)
	call(s.handleAssetActions, http.MethodPatch, "/assets/nas01", `{"name":"nas-main"}`)
	call(s.handleV2AssetActions, http.MethodPatch, "/api/v2/assets/nas01", `{"tags":["storage"]}`)

	windows := "/groups/" + groupID + "/maintenance-windows"
	windowID := field(call(s.handleGroupActions, http.MethodPost, windows, `{"name":"Disk swap","start_at":"2026-12-01T10:00:00Z","end_at":"2026-12-01T12:00:00Z","suppress_alerts":true}`), "window", "id")
	call(s.handleGroupActions, http.MethodPatch, windows+"/"+windowID, `{"name":"Disk swap 2","start_at":"2026-12-01T10:00:00Z","end_at":"2026-12-01T13:00:00Z"}`)
	call(s.handleGroupActions, http.MethodDelete, windows+"/"+windowID, "")

	ruleID := field(call(s.handleAlertRules, http.MethodPost, "/alerts/rules", `{"name":"Disk nearly full","kind":"metric_threshold","severity":"high","target_scope":"global","condition":{"metric":"disk_used_percent","operator":">=","value":90,"aggregate":"max"}}`), "rule", "id")
	// One rule, two alerts: the first fired on one device, the second on two.
	if _, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "db01", Type: "host", Name: "db01", Source: "agent", Status: "online"}); err != nil {
		t.Fatalf("seed second device: %v", err)
	}
	alertIDs := make([]string, 2)
	for i, devices := range [][]string{{"nas01"}, {"nas01", "db01"}} {
		inst, err := s.alertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: ruleID, Fingerprint: "fp-" + string(rune('a'+i)), Severity: "high"})
		if err == nil {
			_, err = s.alertInstanceStore.(persistence.AlertInstanceFiringStore).FireAlertInstance(context.Background(), inst.ID, alerts.FiringScope{Version: 1, AssetIDs: devices})
		}
		if err != nil {
			t.Fatalf("seed alert: %v", err)
		}
		alertIDs[i] = inst.ID
	}
	call(s.handleAlertInstanceActions, http.MethodPost, "/alerts/instances/"+alertIDs[0]+"/ack", "")
	call(s.handleAlertInstanceActions, http.MethodPost, "/alerts/instances/"+alertIDs[0]+"/resolve", "")
	call(s.handleAlertInstanceActions, http.MethodDelete, "/alerts/instances/"+alertIDs[1], "")

	ends := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	silenceID := field(call(s.handleAlertSilences, http.MethodPost, "/alerts/silences", `{"matchers":{"rule_id":"`+ruleID+`"},"reason":"Disk swap","starts_at":"`+time.Now().UTC().Format(time.RFC3339)+`","ends_at":"`+ends+`"}`), "silence", "id")
	call(s.handleAlertSilenceActions, http.MethodDelete, "/alerts/silences/"+silenceID, "")

	routeID := field(call(s.handleAlertRoutes, http.MethodPost, "/alerts/routes", `{"name":"critical-phone"}`), "route", "id")
	call(s.handleAlertRouteActions, http.MethodPatch, "/alerts/routes/"+routeID, `{"name":"critical-pager"}`)
	call(s.handleAlertRouteActions, http.MethodDelete, "/alerts/routes/"+routeID, "")

	issueID := field(call(s.handleIncidents, http.MethodPost, "/incidents", `{"title":"NAS offline","severity":"high"}`), "incident", "id")
	call(s.handleIncidentActions, http.MethodPatch, "/incidents/"+issueID, `{"assignee":"sam","status":"resolved","summary":"swapped drive bay three"}`)
	call(s.handleIncidentActions, http.MethodDelete, "/incidents/"+issueID, "")

	planID := field(call(s.handleUpdatePlans, http.MethodPost, "/updates/plans", `{"name":"Monthly patches","targets":["nas01"]}`), "plan", "id")
	call(s.handleUpdatePlanActions, http.MethodDelete, "/updates/plans/"+planID, "")

	bookmarkID := field(call(s.handleBookmarks, http.MethodPost, "/terminal/bookmarks", `{"title":"NAS shell","host":"192.0.2.10","username":"svc-backup"}`), "bookmark", "id")
	call(s.handleBookmarkActions, http.MethodPut, "/terminal/bookmarks/"+bookmarkID, `{"title":"NAS admin shell"}`)
	call(s.handleBookmarkActions, http.MethodDelete, "/terminal/bookmarks/"+bookmarkID, "")

	encoded := checkNamedAuditRows(t, s, map[string]string{
		"asset.updated nas01":                     "nas-main",
		"maintenance_window.created " + windowID:  "Disk swap",
		"maintenance_window.updated " + windowID:  "Disk swap 2",
		"maintenance_window.deleted " + windowID:  "Disk swap 2",
		"alert.acknowledged " + alertIDs[0]:       "Disk nearly full",
		"alert.resolved " + alertIDs[0]:           "Disk nearly full",
		"alert.deleted " + alertIDs[1]:            "Disk nearly full",
		"alert.silence.created " + silenceID:      "Disk swap",
		"alert.silence.deleted " + silenceID:      "Disk swap",
		"alert.route.created " + routeID:          "critical-phone",
		"alert.route.updated " + routeID:          "critical-pager",
		"alert.route.deleted " + routeID:          "critical-pager",
		"incident.created " + issueID:             "NAS offline",
		"incident.updated " + issueID:             "NAS offline",
		"incident.deleted " + issueID:             "NAS offline",
		"updates.plan.created " + planID:          "Monthly patches",
		"updates.plan.deleted " + planID:          "Monthly patches",
		"terminal.bookmark.created " + bookmarkID: "NAS shell",
		"terminal.bookmark.updated " + bookmarkID: "NAS admin shell",
		"terminal.bookmark.deleted " + bookmarkID: "NAS admin shell",
	})
	for label, private := range map[string]string{"an issue note's text": "drive bay three", "a bookmark's user name": "svc-backup"} {
		if strings.Contains(encoded, private) {
			t.Errorf("audit rows contain %s", label)
		}
	}

	// A device edit says what changed, and an edit that changes nothing is not a row.
	var rows []struct {
		Type    string         `json:"type"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	var deviceEdits []string
	for _, row := range rows {
		switch row.Type {
		case "asset.updated":
			changed, _ := json.Marshal(row.Details["changed"])
			deviceEdits = append(deviceEdits, string(changed)+" was "+strings.TrimSpace(asString(row.Details["previous_name"]))+" group "+asString(row.Details["group_name"]))
		case "alert.acknowledged", "alert.resolved":
			// The device carries the name it has now: nas01 was renamed above.
			if row.Details["device_id"] != "nas01" || row.Details["device_name"] != "nas-main" {
				t.Errorf("%s row does not say which device the alert was for: %v", row.Type, row.Details)
			}
		case "alert.deleted":
			if devices, _ := json.Marshal([]any{row.Details["device_count"], row.Details["device_ids"], row.Details["device_names"]}); string(devices) != `[2,["db01","nas01"],["db01","nas-main"]]` {
				t.Errorf("row for an alert on two devices names them as %s", devices)
			}
		case "incident.updated":
			if changed, _ := json.Marshal(row.Details["changed"]); string(changed) != `["summary","status","assignee"]` || row.Details["assignee"] != "sam" || row.Details["status"] != "resolved" {
				t.Errorf("issue edit row details = %v", row.Details)
			}
		}
	}
	wantEdits := []string{`["tags"] was  group `, `["name","group"] was nas01 group Rack A`}
	if strings.Join(deviceEdits, "|") != strings.Join(wantEdits, "|") && strings.Join(deviceEdits, "|") != wantEdits[1]+"|"+wantEdits[0] {
		t.Errorf("device edit rows = %q, want %q in either order", deviceEdits, wantEdits)
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}
