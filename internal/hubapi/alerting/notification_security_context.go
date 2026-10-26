package alerting

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type notificationChannelContextStore interface {
	GetNotificationChannelContext(context.Context, string) (notifications.Channel, bool, error)
	CompareAndSwapNotificationChannelConfigContext(context.Context, string, map[string]any, map[string]any) (bool, error)
}

// Keep the existing decrypt/migrate/reload policy in one owner. Its store calls
// are bound to the claim context; missing context support never falls back.
type notificationContextStore struct {
	persistence.NotificationStore
	bounded notificationChannelContextStore
	ctx     context.Context
}

func (s notificationContextStore) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	if err := s.ctx.Err(); err != nil {
		return notifications.Channel{}, false, err
	}
	return s.bounded.GetNotificationChannelContext(s.ctx, id)
}

func (s notificationContextStore) CompareAndSwapNotificationChannelConfig(id string, expected, replacement map[string]any) (bool, error) {
	if err := s.ctx.Err(); err != nil {
		return false, err
	}
	return s.bounded.CompareAndSwapNotificationChannelConfigContext(s.ctx, id, expected, replacement)
}

func (d *Deps) getNotificationChannelForRuntimeContext(ctx context.Context, id string) (notifications.Channel, bool, error) {
	store, ok := d.NotificationStore.(notificationChannelContextStore)
	if !ok {
		return notifications.Channel{}, false, fmt.Errorf("bounded channel lookup unavailable")
	}
	bound := *d
	bound.NotificationStore = notificationContextStore{NotificationStore: d.NotificationStore, bounded: store, ctx: ctx}
	channel, found, err := bound.getNotificationChannelForRuntime(id)
	if err == nil {
		err = ctx.Err()
	}
	return channel, found, err
}
