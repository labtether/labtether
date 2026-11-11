package persistence

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/notifications"
)

// Migration 112 turns "Send every alert here" on for channels no alert route
// names (made in the console, they never got an alert) and leaves it off for
// channels a route names and for push channels, so those keep what they got.
func TestChannelAllAlertsMigrationFollowsRoutes(t *testing.T) {
	store := newTestPostgresStore(t)
	create := func(name, channelType string) notifications.Channel {
		t.Helper()
		channel, err := store.CreateNotificationChannel(notifications.CreateChannelRequest{Name: name, Type: channelType, Config: map[string]any{}})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteNotificationChannel(channel.ID) })
		return channel
	}
	console := create("ltqa-console-made", notifications.ChannelTypeWebhook)
	routed := create("ltqa-routed", notifications.ChannelTypeWebhook)
	push := create("ltqa-push", notifications.ChannelTypeAPNs)
	route, err := store.CreateAlertRoute(notifications.CreateRouteRequest{Name: "ltqa-route", ChannelIDs: []string{routed.ID}})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteAlertRoute(route.ID) })

	if _, err := store.pool.Exec(context.Background(), `UPDATE notification_channels SET all_alerts = FALSE WHERE id = ANY($1)`,
		[]string{console.ID, routed.ID, push.ID}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range schemaMigrationChannelAllAlerts().Statements {
		if _, err := store.pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	for _, want := range []struct {
		channel notifications.Channel
		on      bool
	}{{console, true}, {routed, false}, {push, false}} {
		got, _, err := store.GetNotificationChannel(want.channel.ID)
		if err != nil || got.AllAlerts != want.on {
			t.Fatalf("%s all_alerts = %v (err %v), want %v", want.channel.Name, got.AllAlerts, err, want.on)
		}
	}
}
