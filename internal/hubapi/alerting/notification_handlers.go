package alerting

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	notificationChannelCreatedAuditType = "notification.channel.created"
	notificationChannelUpdatedAuditType = "notification.channel.updated"
	notificationChannelDeletedAuditType = "notification.channel.deleted"
	notificationChannelTestedAuditType  = "notification.channel.tested"
	notificationChannelAuditWarning     = "api warning: failed to append notification channel audit event"
)

func (d *Deps) appendNotificationChannelAudit(
	r *http.Request,
	eventType, channelID, action, channelType, decision, outcome string,
) {
	event := audit.NewEvent(eventType)
	event.ActorID = d.principalActorID(r.Context())
	event.Target = strings.TrimSpace(channelID)
	event.Decision = decision
	event.Details = map[string]any{
		"resource_type": "notification_channel",
		"action":        action,
	}
	if normalizedType := notifications.NormalizeChannelType(channelType); normalizedType != "" {
		event.Details["channel_type"] = normalizedType
	}
	if outcome != "" {
		event.Details["outcome"] = outcome
	}
	d.appendAuditEventBestEffort(event, notificationChannelAuditWarning)
}

func (d *Deps) HandleNotificationChannels(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobal(w, r, "notification channels") {
		return
	}
	if r.URL.Path != "/notifications/channels" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		channels, err := d.listNotificationChannelsForAPI(parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list notification channels")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"channels": channels,
			"capabilities": map[string]bool{
				"smtp_insecure_transport_allowed": securityruntime.InsecureTransportAllowed(),
			},
		})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "notifications.channels.create", 60, time.Minute) {
			return
		}
		var req notifications.CreateChannelRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid channel payload")
			return
		}
		if err := ValidateCreateChannelRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		ch, err := d.createSecureNotificationChannel(req)
		if err != nil {
			var validationErr *notificationChannelValidationError
			if errors.As(err, &validationErr) {
				servicehttp.WriteError(w, http.StatusBadRequest, validationErr.Error())
				return
			}
			if errors.Is(err, errNotificationSecretsUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification secret encryption unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create notification channel")
			return
		}
		d.appendNotificationChannelAudit(
			r, notificationChannelCreatedAuditType, ch.ID, "create", ch.Type, "applied", "",
		)
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"channel": ch})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) RouteNotificationChannelActions(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobal(w, r, "notification channels") {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/notifications/channels/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "test" {
		d.WrapAdmin(d.HandleNotificationChannelTest)(w, r)
		return
	}
	d.HandleNotificationChannelActions(w, r)
}

func (d *Deps) HandleNotificationChannelTest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/notifications/channels/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "test" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.EnforceRateLimit(w, r, "notifications.channels.test", 30, time.Minute) {
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	channelID := strings.TrimSpace(parts[0])
	if channelID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
		return
	}

	channel, ok, err := d.getNotificationChannelForRuntime(channelID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load channel")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
		return
	}
	if !channel.Enabled {
		d.appendNotificationChannelAudit(
			r, notificationChannelTestedAuditType, channel.ID, "test", channel.Type, "denied", "channel_disabled",
		)
		servicehttp.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"success": false, "error": "channel is disabled"})
		return
	}

	payload := map[string]any{
		"event":             "notification.test",
		"title":             "[TEST] LabTether Notification Test",
		"text":              fmt.Sprintf("Test notification from LabTether to channel %q at %s", channel.Name, time.Now().Format(time.RFC3339)),
		"message":           fmt.Sprintf("Test notification from LabTether to channel %q", channel.Name),
		"severity":          "low",
		"state":             "test",
		"notification_test": true,
	}
	if notifications.NormalizeChannelType(channel.Type) == notifications.ChannelTypeAPNs {
		// APNs preferences classify deliveries as alerts/incidents. A generic low
		// event is filtered from every default device registration and used to make
		// this endpoint report a false success without contacting APNs.
		payload["event"] = "alert.test"
		payload["severity"] = "critical"
		payload["state"] = "firing"
		payload["alert_id"] = "notification-channel-test"
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationDispatchTimeout)
	defer cancel()
	sendErr := d.sendNotification(ctx, channel, payload)
	if sendErr != nil {
		d.appendNotificationChannelAudit(
			r, notificationChannelTestedAuditType, channel.ID, "test", channel.Type, "failed", "delivery_failed",
		)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"success": false, "error": sanitizeNotificationDeliveryError(channel, sendErr)})
		return
	}
	d.appendNotificationChannelAudit(
		r, notificationChannelTestedAuditType, channel.ID, "test", channel.Type, "succeeded", "delivered",
	)
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (d *Deps) HandleNotificationChannelActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/notifications/channels/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "channel path not found")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "channel path not found")
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	channelID := strings.TrimSpace(parts[0])
	if channelID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "channel path not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		ch, ok, err := d.getNotificationChannelForAPI(channelID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load channel")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"channel": ch})
	case http.MethodPatch, http.MethodPut:
		if !d.EnforceRateLimit(w, r, "notifications.channels.update", 120, time.Minute) {
			return
		}
		var req notifications.UpdateChannelRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid channel payload")
			return
		}
		if err := ValidateUpdateChannelRequest(req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := d.updateSecureNotificationChannel(channelID, req)
		if err != nil {
			if err == notifications.ErrChannelNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
				return
			}
			if errors.Is(err, errNotificationSecretsUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification secret encryption unavailable")
				return
			}
			var validationErr *notificationChannelValidationError
			if errors.As(err, &validationErr) {
				servicehttp.WriteError(w, http.StatusBadRequest, validationErr.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update channel")
			return
		}
		d.appendNotificationChannelAudit(
			r, notificationChannelUpdatedAuditType, updated.ID, "update", updated.Type, "applied", "",
		)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"channel": updated})
	case http.MethodDelete:
		if err := d.NotificationStore.DeleteNotificationChannel(channelID); err != nil {
			if err == notifications.ErrChannelNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete channel")
			return
		}
		d.appendNotificationChannelAudit(
			r, notificationChannelDeletedAuditType, channelID, "delete", "", "applied", "",
		)
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleNotificationHistory(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobal(w, r, "notification history") {
		return
	}
	if r.URL.Path != "/notifications/history" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	records, err := d.NotificationStore.ListNotificationHistory(
		parseLimit(r, 50),
		r.URL.Query().Get("channel_id"),
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list notification history")
		return
	}
	channelCache := make(map[string]notifications.Channel)
	unavailableChannels := make(map[string]struct{})
	for i := range records {
		rawError := records[i].Error
		safeError := notifications.SanitizeDeliveryErrorMessage(rawError)
		channelID := strings.TrimSpace(records[i].ChannelID)
		if channelID != "" && safeError != "" {
			channel, cached := channelCache[channelID]
			if !cached {
				if _, unavailable := unavailableChannels[channelID]; !unavailable {
					loaded, ok, loadErr := d.getNotificationChannelForRuntime(channelID)
					if loadErr == nil && ok {
						channel = loaded
						channelCache[channelID] = loaded
						cached = true
					} else {
						unavailableChannels[channelID] = struct{}{}
					}
				}
			}
			if cached {
				safeError = sanitizeNotificationDeliveryError(channel, errors.New(rawError))
			}
		}
		records[i].Error = safeError
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"history": records})
}
