package alerting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/notifications"
)

func TestNotificationChannelListPagesPastStoreCapAndRedactsSecrets(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	updated := time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)
	for index := range 505 {
		id := fmt.Sprintf("channel-%03d", index)
		store.seed(notifications.Channel{
			ID: id, Type: notifications.ChannelTypeSlack, UpdatedAt: updated,
			Config: map[string]any{"api_token": "synthetic-secret", "label": id},
		})
	}

	for _, page := range []struct {
		path            string
		count           int
		firstID, lastID string
	}{
		{"/notifications/channels", 50, "channel-504", "channel-455"},
		{"/notifications/channels?limit=500&offset=0", 500, "channel-504", "channel-005"},
		{"/notifications/channels?limit=500&offset=500", 5, "channel-004", "channel-000"},
		{"/notifications/channels?limit=500&offset=505", 0, "", ""},
	} {
		recorder := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodGet, page.path, "")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status=%d", page.path, recorder.Code)
		}
		var body struct {
			Channels []notifications.Channel `json:"channels"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode: %v", page.path, err)
		}
		if len(body.Channels) != page.count {
			t.Fatalf("%s: got %d channels, want %d", page.path, len(body.Channels), page.count)
		}
		if page.count == 0 {
			continue
		}
		if body.Channels[0].ID != page.firstID || body.Channels[len(body.Channels)-1].ID != page.lastID {
			t.Fatalf("%s: unexpected page boundaries", page.path)
		}
		for _, channel := range body.Channels {
			if _, present := channel.Config["api_token"]; present {
				t.Fatalf("%s: page returned a secret field", page.path)
			}
			if channel.Config["label"] != channel.ID {
				t.Fatalf("%s: page lost non-secret config", page.path)
			}
		}
	}
}

func TestNotificationChannelListRejectsInvalidPagination(t *testing.T) {
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = newNotificationSecurityStore()
	for _, query := range []string{
		"limit=0", "limit=501", "limit=bad", "limit=1&limit=2",
		"offset=-1", "offset=bad", "offset=1&offset=2", "offset=999999999999999999999999",
	} {
		recorder := runNotificationHandlerRequest(t, deps.HandleNotificationChannels, http.MethodGet,
			"/notifications/channels?"+query, "")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d, want 400", query, recorder.Code)
		}
	}
}
