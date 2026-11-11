package alerting

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labtether/labtether/internal/notifications"
)

// The Delivery log read "Unknown alert" for every message whose rule or alert
// was deleted, though the stored message still had the rule name. History
// gives that name, on one line.
func TestNotificationHistoryNamesTheAlertAfterItsRuleIsGone(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	if _, err := store.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID:       "nch-gone",
		AlertInstanceID: "alert-deleted",
		Status:          notifications.RecordStatusSent,
		Payload:         map[string]any{"rule_name": "Disk nearly\r\nfull", "title": "[HIGH] Disk nearly full (firing)"},
	}); err != nil {
		t.Fatalf("seed history: %v", err)
	}

	rec := runNotificationHandlerRequest(t, deps.HandleNotificationHistory, http.MethodGet, "/notifications/history", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		History []struct {
			AlertName string `json:"alert_name"`
		} `json:"history"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(response.History) != 1 || response.History[0].AlertName != "Disk nearly full" {
		t.Fatalf("history = %+v, want alert_name %q", response.History, "Disk nearly full")
	}
}
