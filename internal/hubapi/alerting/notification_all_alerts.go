package alerting

import (
	"log"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/servicehttp"
)

// allAlertsScope is what turning on "Send every alert here" needs besides
// notifications:write: it sends every alert, which routes guard with
// alerts:write, so a key that may only add channels cannot route every alert
// to a server of its choice.
const allAlertsScope = "alerts:write"

const allAlertsChannelPageSize = 500

// allowAllAlertsOn answers a request that turns "Send every alert here" on
// for a channel of this type; it reports false after writing the refusal.
func allowAllAlertsOn(w http.ResponseWriter, r *http.Request, channelType string) bool {
	if notifications.NormalizeChannelType(channelType) == notifications.ChannelTypeAPNs {
		servicehttp.WriteError(w, http.StatusBadRequest, "Push channels get alerts through their own route, so they can't be set to get every alert.")
		return false
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), allAlertsScope) {
		servicehttp.WriteError(w, http.StatusForbidden, "Sending every alert to a channel needs the alerts:write permission as well.")
		return false
	}
	return true
}

// allAlertsChannelIDs lists every enabled channel set to get every alert,
// except push channels, which keep their route. The list is read in full;
// when a page cannot be read, the alert goes only where routes send it.
func (d *Deps) allAlertsChannelIDs() []string {
	ids := make([]string, 0, 8)
	for offset := 0; ; offset += allAlertsChannelPageSize {
		channels, err := d.NotificationStore.ListNotificationChannels(allAlertsChannelPageSize, offset)
		if err != nil {
			log.Printf("notifications: failed to list channels that get every alert: %v", err)
			return nil
		}
		for _, channel := range channels {
			id := strings.TrimSpace(channel.ID)
			if id == "" || !channel.Enabled || !channel.AllAlerts ||
				notifications.NormalizeChannelType(channel.Type) == notifications.ChannelTypeAPNs {
				continue
			}
			ids = append(ids, id)
		}
		if len(channels) < allAlertsChannelPageSize {
			return ids
		}
	}
}

// channelReceivesAlerts reports whether alerts go to the stored channel: it is
// set to get every alert, or an alert route names it. When the routes cannot
// be read in full it answers true, so the stricter permission applies.
func (d *Deps) channelReceivesAlerts(channel notifications.Channel) bool {
	if channel.AllAlerts {
		return true
	}
	routes, err := d.NotificationStore.ListAlertRoutes(notificationRouteScanLimit)
	if err != nil || len(routes) >= notificationRouteScanLimit {
		return true
	}
	for _, route := range routes {
		for _, id := range route.ChannelIDs {
			if strings.TrimSpace(id) == channel.ID {
				return true
			}
		}
	}
	return false
}

// allowChannelUpdate decides from the stored channel whether this edit needs
// alerts:write, writing the refusal when it does and is missing. Turning
// "Send every alert here" on always does. On a channel that gets alerts, any
// change to where or whether it sends (config, enabled) does too, so a key
// that may only manage channels cannot point one at its own server or wake
// a disabled one. A new name, or turning every alert off, does not.
func (d *Deps) allowChannelUpdate(w http.ResponseWriter, r *http.Request, current notifications.Channel, req notifications.UpdateChannelRequest) bool {
	if req.AllAlerts != nil && *req.AllAlerts && !current.AllAlerts {
		return allowAllAlertsOn(w, r, current.Type)
	}
	changesDelivery := req.Config != nil || (req.Enabled != nil && *req.Enabled != current.Enabled)
	if !changesDelivery || apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), allAlertsScope) || !d.channelReceivesAlerts(current) {
		return true
	}
	servicehttp.WriteError(w, http.StatusForbidden, "Changing a channel that gets alerts needs the alerts:write permission as well.")
	return false
}
