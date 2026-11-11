package persistence

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/notifications"
)

// Migration 112 turns "Send every alert here" on only for channels no alert
// route names that a signed-in person made (audit row with a user or the
// owner as actor). Channels an API key made, channels of unknown origin,
// routed channels and push channels stay off.
func TestChannelAllAlertsMigrationFollowsRoutesAndWhoMadeTheChannel(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	user, err := store.CreateUser("ltqa-migration-user-"+strconv.FormatInt(time.Now().UnixNano(), 10), "x")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID) })
	create := func(name, channelType, actor string) notifications.Channel {
		t.Helper()
		channel, err := store.CreateNotificationChannel(notifications.CreateChannelRequest{Name: name, Type: channelType, Config: map[string]any{}})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteNotificationChannel(channel.ID) })
		if actor != "" {
			auditID := "ltqa-audit-" + channel.ID
			if _, err := store.pool.Exec(ctx, `INSERT INTO audit_events (id, type, actor_id, target, timestamp) VALUES ($1, 'notification.channel.created', $2, $3, NOW())`,
				auditID, actor, channel.ID); err != nil {
				t.Fatalf("audit row: %v", err)
			}
			t.Cleanup(func() { _, _ = store.pool.Exec(ctx, `DELETE FROM audit_events WHERE id = $1`, auditID) })
		}
		return channel
	}
	byUser := create("ltqa-by-user", notifications.ChannelTypeWebhook, user.ID)
	byOwner := create("ltqa-by-owner", notifications.ChannelTypeEmail, "owner")
	byKey := create("ltqa-by-key", notifications.ChannelTypeWebhook, "apikey:ltqa")
	unknown := create("ltqa-unknown", notifications.ChannelTypeWebhook, "")
	routed := create("ltqa-routed", notifications.ChannelTypeWebhook, user.ID)
	push := create("ltqa-push", notifications.ChannelTypeAPNs, user.ID)
	route, err := store.CreateAlertRoute(notifications.CreateRouteRequest{Name: "ltqa-route", ChannelIDs: []string{routed.ID}})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteAlertRoute(route.ID) })

	for _, statement := range schemaMigrationChannelAllAlerts().Statements {
		if _, err := store.pool.Exec(ctx, statement); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	for _, want := range []struct {
		channel notifications.Channel
		on      bool
	}{{byUser, true}, {byOwner, true}, {byKey, false}, {unknown, false}, {routed, false}, {push, false}} {
		got, _, err := store.GetNotificationChannel(want.channel.ID)
		if err != nil || got.AllAlerts != want.on {
			t.Fatalf("%s all_alerts = %v (err %v), want %v", want.channel.Name, got.AllAlerts, err, want.on)
		}
	}
}
