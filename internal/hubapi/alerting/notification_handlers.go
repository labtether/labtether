package alerting

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleNotificationChannels(w http.ResponseWriter, r *http.Request) {
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
		channels, err := d.NotificationStore.ListNotificationChannels(parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list notification channels")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"channels": channels})
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
		ch, err := d.NotificationStore.CreateNotificationChannel(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create notification channel")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"channel": ch})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) RouteNotificationChannelActions(w http.ResponseWriter, r *http.Request) {
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
	if d.NotificationStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "notification store unavailable")
		return
	}

	channelID := strings.TrimSpace(parts[0])
	if channelID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
		return
	}

	channel, ok, err := d.NotificationStore.GetNotificationChannel(channelID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load channel")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
		return
	}
	if !channel.Enabled {
		servicehttp.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"success": false, "error": "channel is disabled"})
		return
	}

	payload := map[string]any{
		"event":    "notification.test",
		"title":    "[TEST] LabTether Notification Test",
		"text":     fmt.Sprintf("Test notification from LabTether to channel %q at %s", channel.Name, time.Now().Format(time.RFC3339)),
		"message":  fmt.Sprintf("Test notification from LabTether to channel %q", channel.Name),
		"severity": "low",
		"state":    "test",
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationDispatchTimeout)
	defer cancel()
	sendErr := d.sendNotification(ctx, channel, payload)
	if sendErr != nil {
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"success": false, "error": sendErr.Error()})
		return
	}
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
		ch, ok, err := d.NotificationStore.GetNotificationChannel(channelID)
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
		updated, err := d.NotificationStore.UpdateNotificationChannel(channelID, req)
		if err != nil {
			if err == notifications.ErrChannelNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "channel not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update channel")
			return
		}
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
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleNotificationHistory(w http.ResponseWriter, r *http.Request) {
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
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"history": records})
}
