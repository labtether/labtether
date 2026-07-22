package alerting

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	incidentPushTitleMaxBytes   = 160
	incidentPushSummaryMaxBytes = 1_024
	incidentPushFieldMaxBytes   = 64
	incidentPushIDMaxBytes      = 256
)

// fallbackNotificationDispatchSem keeps reduced runtimes and unit harnesses
// bounded even when they do not inject the production notification semaphore.
// Production uses Deps.NotificationSem, which currently permits 32 dispatches.
var fallbackNotificationDispatchSem = make(chan struct{}, 8)

// dispatchIncidentNotificationAsync queues an APNs-only incident delivery.
// Payload construction and wait-group registration happen before returning so
// callers can safely wait for all accepted work during shutdown and in tests.
// Network I/O always runs outside the HTTP request goroutine.
func (d *Deps) dispatchIncidentNotificationAsync(incident incidents.Incident, event string) {
	if d == nil || strings.TrimSpace(incident.ID) == "" {
		return
	}
	normalizedEvent := strings.ToLower(strings.TrimSpace(event))
	if strings.HasPrefix(normalizedEvent, "incident.") {
		d.broadcastEvent(normalizedEvent, map[string]any{
			"incident_id": incident.ID,
			"status":      incident.Status,
			"severity":    incident.Severity,
		})
	}
	if d.NotificationStore == nil && d.DispatchIncidentLiveActivity == nil {
		return
	}
	payload := buildIncidentNotificationPayload(incident, event)
	if payload == nil && d.DispatchIncidentLiveActivity == nil {
		return
	}

	limiter := d.NotificationSem
	if limiter == nil {
		limiter = fallbackNotificationDispatchSem
	}
	if d.NotificationWG != nil {
		d.NotificationWG.Add(1)
	}
	go func() {
		if d.NotificationWG != nil {
			defer d.NotificationWG.Done()
		}
		// The callback only schedules its own bounded delivery work. Invoke it in
		// this accepted worker, never in the incident HTTP request goroutine, and
		// before normal notification capacity can block.
		if d.DispatchIncidentLiveActivity != nil {
			d.DispatchIncidentLiveActivity(incident, event)
		}
		if d.NotificationStore == nil || payload == nil {
			return
		}
		limiter <- struct{}{}
		defer func() { <-limiter }()
		d.dispatchIncidentNotificationSync(payload)
	}()
}

func (d *Deps) dispatchIncidentNotificationSync(payload map[string]any) {
	if d.NotificationStore == nil || !isIncidentNotificationPayload(payload) {
		return
	}
	// Re-check immediately before delivery. A maintenance window may have begun
	// after the incident mutation was accepted but before this goroutine ran.
	if d.maintenanceSuppressesGroupIDs(notificationPayloadGroupIDs(payload)) {
		return
	}

	channels, err := d.NotificationStore.ListNotificationChannels(notificationRouteScanLimit)
	if err != nil {
		log.Printf("notifications: failed to list APNs channels for incident delivery: %v", err)
		return
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].ID < channels[j].ID })
	seen := make(map[string]struct{}, len(channels))
	for _, listed := range channels {
		channelID := strings.TrimSpace(listed.ID)
		if channelID == "" || !listed.Enabled || notifications.NormalizeChannelType(listed.Type) != notifications.ChannelTypeAPNs {
			continue
		}
		if _, duplicate := seen[channelID]; duplicate {
			continue
		}
		seen[channelID] = struct{}{}

		channel, ok, loadErr := d.getNotificationChannelForRuntime(channelID)
		if loadErr != nil {
			log.Printf("notifications: failed to load APNs channel %s for incident delivery: %v", channelID, loadErr)
			continue
		}
		// Re-check after the runtime reload so a concurrent disable or channel-type
		// edit cannot turn this dedicated path into a non-APNs delivery.
		if !ok || !channel.Enabled || notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
			continue
		}

		sendCtx, cancel := context.WithTimeout(context.Background(), notificationDispatchTimeout)
		sendErr := d.sendNotification(sendCtx, channel, payload)
		cancel()
		if sendErr != nil {
			safeError := sanitizeNotificationDeliveryError(channel, sendErr)
			failedPayload := payload
			if targetedPayload, targeted := payloadWithAPNsRetryTargets(payload, sendErr); targeted {
				failedPayload = targetedPayload
			}
			record := d.recordNotificationHistoryWithRetry(
				channel.ID,
				"",
				"",
				notifications.RecordStatusFailed,
				safeError,
				failedPayload,
			)
			securityruntime.Logf("notifications: incident APNs channel %s send failed (retry %d/%d)", channel.ID, record.RetryCount, record.MaxRetries)
			continue
		}
		d.recordNotificationHistory(
			channel.ID,
			"",
			"",
			notifications.RecordStatusSent,
			"",
			payload,
		)
	}
}

func buildIncidentNotificationPayload(incident incidents.Incident, event string) map[string]any {
	incidentID := boundedIncidentPushText(incident.ID, incidentPushIDMaxBytes)
	event = normalizeIncidentNotificationEvent(event)
	if incidentID == "" || event == "" {
		return nil
	}

	title := boundedIncidentPushText(incident.Title, incidentPushTitleMaxBytes)
	if title == "" {
		title = "LabTether Incident"
	}
	summary := boundedIncidentPushText(incident.Summary, incidentPushSummaryMaxBytes)
	status := boundedIncidentPushText(incidents.NormalizeStatus(incident.Status), incidentPushFieldMaxBytes)
	severity := boundedIncidentPushText(incidents.NormalizeSeverity(incident.Severity), incidentPushFieldMaxBytes)
	text := summary
	if text == "" {
		text = fmt.Sprintf("Incident is %s with %s severity.", firstNonBlank(status, "open"), firstNonBlank(severity, "unknown"))
	}

	groupIDs := make([]string, 0, 1)
	if groupID := strings.TrimSpace(incident.GroupID); groupID != "" {
		groupIDs = append(groupIDs, groupID)
	}
	return map[string]any{
		"event":         event,
		"incident_id":   incidentID,
		"severity":      severity,
		"status":        status,
		"title":         title,
		"summary":       summary,
		"text":          boundedIncidentPushText(text, incidentPushSummaryMaxBytes),
		"occurred_at":   time.Now().UTC().Format(time.RFC3339),
		"group_ids":     groupIDs,
		"deep_link":     "labtether://incidents/" + url.PathEscape(incidentID),
		"apns_category": "LT_INCIDENT_ACTIONS",
	}
}

func normalizeIncidentNotificationEvent(event string) string {
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "incident.created":
		return "incident.created"
	case "incident.status_changed":
		return "incident.status_changed"
	case "incident.severity_changed":
		return "incident.severity_changed"
	case "incident.resolved":
		return "incident.resolved"
	default:
		return ""
	}
}

func incidentMaterialTransitionEvent(before, after incidents.Incident) string {
	statusChanged := incidents.NormalizeStatus(before.Status) != incidents.NormalizeStatus(after.Status)
	severityChanged := incidents.NormalizeSeverity(before.Severity) != incidents.NormalizeSeverity(after.Severity)
	if !statusChanged && !severityChanged {
		return ""
	}
	if statusChanged {
		if incidents.NormalizeStatus(after.Status) == incidents.StatusResolved {
			return "incident.resolved"
		}
		return "incident.status_changed"
	}
	return "incident.severity_changed"
}

func isIncidentNotificationPayload(payload map[string]any) bool {
	if strings.TrimSpace(payloadString(payload, "incident_id")) == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(payloadString(payload, "event")), "incident.")
}

func boundedIncidentPushText(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 {
		return ""
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}
