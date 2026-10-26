package alerting

import (
	"context"
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/notifications"
)

const (
	relayDefaultBootstrapName = "apns_relay_default_v1"
	relayDefaultChannelName   = "LabTether iOS app"
	relayDefaultRouteName     = "LabTether iOS app: all alerts"
	relayDefaultAuditWarning  = "api warning: failed to append default push relay channel audit event"
)

// relayCapableAdapter is implemented by *notifications.APNsAdapter.
type relayCapableAdapter interface {
	RelayEnabled() bool
}

// EnsureDefaultRelayPushRoute gives a Hub with no APNs route a relay-backed
// channel and a catch-all route, so the official iOS app gets alerts without
// operator setup. It runs when a relay registration is stored and is
// idempotent: the bootstrap marker makes it happen once per Hub, so an
// operator who deletes or edits the channel or route keeps that choice. Per
// device preferences (category, severity, quiet hours) still filter delivery.
func (d *Deps) EnsureDefaultRelayPushRoute(ctx context.Context, markers NotificationBootstrapStore) (bool, error) {
	if d == nil || d.NotificationStore == nil || markers == nil || !d.pushRelayEnabled() {
		return false, nil
	}
	routed, channelID, err := d.apnsRoutingState()
	if err != nil || routed {
		return false, err
	}
	claimed, err := markers.ClaimNotificationBootstrap(ctx, relayDefaultBootstrapName)
	if err != nil || !claimed {
		return false, err
	}
	release := func(cause error, cleanup ...error) (bool, error) {
		released := markers.ReleaseNotificationBootstrap(context.WithoutCancel(ctx), relayDefaultBootstrapName)
		return false, errors.Join(append([]error{cause, released}, cleanup...)...)
	}
	enabled := true
	createdChannel := channelID == ""
	if createdChannel {
		channel, createErr := d.createSecureNotificationChannel(notifications.CreateChannelRequest{
			Name:    relayDefaultChannelName,
			Type:    notifications.ChannelTypeAPNs,
			Enabled: &enabled,
			Config: map[string]any{
				"transport":  notifications.APNsTransportRelay,
				"bundle_id":  notifications.OfficialAppBundleID,
				"production": true,
			},
		})
		if createErr != nil {
			return release(createErr)
		}
		channelID = channel.ID
	}
	route, err := d.NotificationStore.CreateAlertRoute(notifications.CreateRouteRequest{
		Name:       relayDefaultRouteName,
		ChannelIDs: []string{channelID},
		Enabled:    &enabled,
	})
	if err != nil {
		if createdChannel {
			return release(err, d.NotificationStore.DeleteNotificationChannel(channelID))
		}
		return release(err)
	}
	event := audit.NewEvent(notificationChannelCreatedAuditType)
	event.ActorID = "system"
	event.Target = channelID
	event.Decision = "applied"
	event.Details = map[string]any{
		"resource_type": "notification_channel",
		"action":        "create",
		"channel_type":  notifications.ChannelTypeAPNs,
		"transport":     notifications.APNsTransportRelay,
		"route_id":      route.ID,
		"reason":        "first iOS device registered through the LabTether push relay",
	}
	d.appendAuditEventBestEffort(event, relayDefaultAuditWarning)
	return true, nil
}

func (d *Deps) pushRelayEnabled() bool {
	adapter, ok := d.NotificationAdapters[notifications.ChannelTypeAPNs].(relayCapableAdapter)
	return ok && adapter.RelayEnabled()
}

// apnsRoutingState reports whether any route, enabled or not, already targets
// an APNs channel of either transport; an operator who set up APNs routing
// keeps full control and gets nothing automatic. Otherwise it returns an
// existing enabled relay channel to reuse, if there is one.
func (d *Deps) apnsRoutingState() (bool, string, error) {
	channels, err := d.NotificationStore.ListNotificationChannels(notificationRouteScanLimit, 0)
	if err != nil {
		return false, "", err
	}
	apnsChannels := make(map[string]struct{}, len(channels))
	relayChannelID := ""
	for _, channel := range channels {
		if notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
			continue
		}
		id := strings.TrimSpace(channel.ID)
		apnsChannels[id] = struct{}{}
		// Stored relay channels carry an explicit transport (see
		// normalizeAPNsChannelConfig); never infer relay from a missing config.
		if relayChannelID == "" && channel.Enabled &&
			strings.EqualFold(configString(channel.Config, "transport"), notifications.APNsTransportRelay) {
			relayChannelID = id
		}
	}
	if len(apnsChannels) == 0 {
		return false, "", nil
	}
	routes, err := d.NotificationStore.ListAlertRoutes(notificationRouteScanLimit)
	if err != nil {
		return false, "", err
	}
	for _, route := range routes {
		for _, channelID := range route.ChannelIDs {
			if _, ok := apnsChannels[strings.TrimSpace(channelID)]; ok {
				return true, "", nil
			}
		}
	}
	return false, relayChannelID, nil
}
