package alerting

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
)

type routedNotificationStore struct {
	*apnsRetryNotificationStore
	routes []notifications.Route
}

func (s *routedNotificationStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return s.routes, nil
}

func sentChannels(t *testing.T, store *routedNotificationStore) map[string]int {
	t.Helper()
	records, err := store.ListNotificationHistory(100, "")
	if err != nil {
		t.Fatal(err)
	}
	sent := map[string]int{}
	for _, record := range records {
		if record.Status == notifications.RecordStatusSent {
			sent[record.ChannelID]++
		}
	}
	return sent
}

func scopedChannelRequest(method, target, body string, scopes ...string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(apiv2.ContextWithScopes(req.Context(), scopes))
}

// "Send every alert here" sends every alert to a channel, which alert routes
// guard with alerts:write. A key with only notifications:write must not turn
// it on (at create or later), or it could send every alert to its own server.
func TestOnlyAlertsWriteMayTurnOnSendEveryAlert(t *testing.T) {
	store := newNotificationSecurityStore()
	deps := newTestAlertingDeps(t)
	deps.NotificationStore = store
	deps.NotificationSecrets = testutil.TestSecretsManager(t)
	webhook := func(allAlerts string) string {
		return `{"name":"hook","type":"webhook","all_alerts":` + allAlerts + `,"config":{"url":"https://example.invalid/hook"}}`
	}

	rec := httptest.NewRecorder()
	deps.HandleNotificationChannels(rec, scopedChannelRequest(http.MethodPost, "/notifications/channels", webhook("true"), "notifications:write"))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "alerts:write") {
		t.Fatalf("notifications:write create with every alert = %d %s, want 403 in words", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	deps.HandleNotificationChannels(rec, scopedChannelRequest(http.MethodPost, "/notifications/channels", webhook("false"), "notifications:write"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("notifications:write create with it off = %d %s, want 201", rec.Code, rec.Body.String())
	}
	channels, _ := store.ListNotificationChannels(10, 0)
	if len(channels) != 1 || channels[0].AllAlerts {
		t.Fatalf("channels = %+v, want one channel with every alert off", channels)
	}
	rec = httptest.NewRecorder()
	deps.HandleNotificationChannelActions(rec, scopedChannelRequest(http.MethodPatch, "/notifications/channels/"+channels[0].ID, `{"all_alerts":true}`, "notifications:write"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("notifications:write turning it on later = %d %s, want 403", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	deps.HandleNotificationChannelActions(rec, scopedChannelRequest(http.MethodPatch, "/notifications/channels/"+channels[0].ID, `{"all_alerts":true}`, "notifications:write", "alerts:write"))
	if rec.Code != http.StatusOK {
		t.Fatalf("with alerts:write too = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

// A channel set to get every alert gets each alert once, also when a route
// names it, and keeps getting them after that route is deleted. A channel
// with it off gets only what routes send it.
func TestEveryAlertChannelGetsEachAlertOnceWithOrWithoutRoutes(t *testing.T) {
	d, base, rule, instanceID := newSuppressionDelivery(t)
	store := &routedNotificationStore{apnsRetryNotificationStore: base.apnsRetryNotificationStore}
	d.NotificationStore = store
	for _, channel := range []notifications.Channel{
		{ID: "every", Enabled: true, AllAlerts: true}, {ID: "routed-only", Enabled: true},
	} {
		channel.Type = notifications.ChannelTypeWebhook
		channel.Config = map[string]any{"url": "https://example.invalid/unused"}
		store.seed(channel)
	}
	d.NotificationAdapters[notifications.ChannelTypeWebhook] = &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook}
	dispatch := func() map[string]int {
		d.DispatchAlertNotifications(rule, instanceID, "firing")
		waitDelivery(t, d)
		return sentChannels(t, store)
	}

	store.routes = []notifications.Route{{ID: "all", Enabled: true, ChannelIDs: []string{"every"}}}
	if sent := dispatch(); len(sent) != 1 || sent["every"] != 1 {
		t.Fatalf("with a route naming it sent %v, want the every-alert channel once and nothing to routed-only", sent)
	}
	store.routes = nil // the route is deleted
	if sent := dispatch(); sent["every"] != 2 || sent["routed-only"] != 0 {
		t.Fatalf("after the route was deleted sent %v, want every 2 and routed-only 0", sent)
	}
	if channel, _, _ := store.GetNotificationChannel("every"); !channel.AllAlerts {
		t.Fatal("deleting the route changed the channel's setting")
	}
}
