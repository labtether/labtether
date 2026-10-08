package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/securityruntime"
	"log"
	"strings"
	"time"
)

func (d *Deps) recordNotificationHistory(channelID, instanceID, routeID, status, errorMessage string, payload map[string]any) {
	if d.NotificationStore == nil {
		return
	}
	_, err := d.NotificationStore.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID:       strings.TrimSpace(channelID),
		AlertInstanceID: strings.TrimSpace(instanceID),
		RouteID:         strings.TrimSpace(routeID),
		Payload:         cloneAnyMap(payload),
		Status:          strings.TrimSpace(status),
		Error:           notifications.SanitizeDeliveryErrorMessage(errorMessage),
	})
	if err != nil {
		log.Printf("notifications: failed to persist notification history (channel=%s route=%s): %v", channelID, routeID, err)
	}
}

// recordNotificationHistoryWithRetry creates a failed notification record and
// schedules it for retry by setting next_retry_at via RetryBackoff(0).
// It returns the created record so the caller can log retry metadata.
func (d *Deps) recordNotificationHistoryWithRetry(channelID, instanceID, routeID, status, errorMessage string, payload map[string]any) notifications.Record {
	if d.NotificationStore == nil {
		return notifications.Record{}
	}
	safeError := notifications.SanitizeDeliveryErrorMessage(errorMessage)
	rec, err := d.NotificationStore.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID:       strings.TrimSpace(channelID),
		AlertInstanceID: strings.TrimSpace(instanceID),
		RouteID:         strings.TrimSpace(routeID),
		Payload:         cloneAnyMap(payload),
		Status:          strings.TrimSpace(status),
		Error:           safeError,
	})
	if err != nil {
		log.Printf("notifications: failed to persist notification history (channel=%s route=%s): %v", channelID, routeID, err)
		return notifications.Record{}
	}
	// Schedule first retry.
	nextRetry := time.Now().UTC().Add(notifications.RetryBackoff(0))
	updateErr := d.NotificationStore.UpdateRetryState(context.Background(), rec.ID, 0, &nextRetry, notifications.RecordStatusFailed, safeError, nil)
	if updateErr != nil {
		log.Printf("notifications: failed to schedule retry for record %s: %v", rec.ID, updateErr)
	}
	rec.NextRetryAt = &nextRetry
	return rec
}

// retryPendingNotifications fetches failed notification records that are due
// for retry and re-dispatches them. Records that exhaust all retries are
// permanently marked failed (next_retry_at cleared).
func (d *Deps) RetryPendingNotifications(ctx context.Context) {
	if d.NotificationStore == nil {
		return
	}
	now := time.Now().UTC()
	records, err := d.NotificationStore.ListPendingRetries(ctx, now, 50)
	if err != nil {
		log.Printf("notifications: failed to list pending retries: %v", err)
		return
	}
	for _, rec := range records {
		channel, ok, loadErr := d.getNotificationChannelForRuntime(rec.ChannelID)
		if loadErr != nil {
			log.Printf("notifications: retry load channel %s failed: %v", rec.ChannelID, loadErr)
			continue
		}
		if !ok || !channel.Enabled {
			// Channel gone or disabled — exhaust retries immediately.
			d.exhaustNotificationRetry(ctx, rec, "channel unavailable or disabled", nil)
			continue
		}

		payload := d.payloadForRetry(rec)
		if d.maintenanceSuppressesGroupIDs(notificationPayloadGroupIDs(payload)) {
			// Leave the retry due without consuming an attempt. It will resume as
			// soon as the active maintenance window ends.
			continue
		}
		if isIncidentNotificationPayload(payload) && notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
			// Incident delivery is deliberately APNs-only. If an operator changes
			// the channel type while a retry is pending, fail closed instead of
			// replaying the incident to an unrelated integration.
			d.exhaustNotificationRetry(ctx, rec, "incident delivery requires an APNs channel", nil)
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, notificationDispatchTimeout)
		sendErr := d.sendNotification(sendCtx, channel, payload)
		cancel()

		newRetryCount := rec.RetryCount + 1
		if sendErr == nil {
			// Success — clear retry state and mark sent.
			clearUpdateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, newRetryCount, nil, notifications.RecordStatusSent, "", nil)
			if clearUpdateErr != nil {
				log.Printf("notifications: failed to mark retry success for record %s: %v", rec.ID, clearUpdateErr)
			}
			log.Printf("notifications: retry succeeded for record %s (attempt %d)", rec.ID, newRetryCount)
			continue
		}

		securityruntime.Logf("notifications: retry %d/%d failed for record %s", newRetryCount, rec.MaxRetries, rec.ID)

		failedPayload, _ := payloadWithAPNsRetryTargets(payload, sendErr)
		if newRetryCount >= rec.MaxRetries {
			d.exhaustNotificationRetry(ctx, rec, sanitizeNotificationDeliveryError(channel, sendErr), failedPayload)
			continue
		}

		// Schedule next retry.
		nextRetry := now.Add(notifications.RetryBackoff(newRetryCount))
		updateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, newRetryCount, &nextRetry, notifications.RecordStatusFailed, sanitizeNotificationDeliveryError(channel, sendErr), failedPayload)
		if updateErr != nil {
			log.Printf("notifications: failed to update retry state for record %s: %v", rec.ID, updateErr)
		}
	}
}

// exhaustNotificationRetry marks a record as permanently failed by setting
// retry_count = max_retries and clearing next_retry_at.
func (d *Deps) exhaustNotificationRetry(ctx context.Context, rec notifications.Record, reason string, payload map[string]any) {
	safeReason := notifications.SanitizeDeliveryErrorMessage(reason)
	updateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, rec.MaxRetries, nil, notifications.RecordStatusFailed, safeReason, payload)
	if updateErr != nil {
		log.Printf("notifications: failed to exhaust retry for record %s: %v", rec.ID, updateErr)
	}
	log.Printf("notifications: record %s permanently failed after %d retries: %s", rec.ID, rec.MaxRetries, safeReason)
}

func (d *Deps) payloadForRetry(rec notifications.Record) map[string]any {
	if len(rec.Payload) > 0 {
		payload := cloneAnyMap(rec.Payload)
		payload["retry"] = true
		if payloadString(payload, "alert_instance_id") == "" && strings.TrimSpace(rec.AlertInstanceID) != "" {
			payload["alert_instance_id"] = strings.TrimSpace(rec.AlertInstanceID)
		}
		return payload
	}

	if d.AlertInstanceStore != nil && strings.TrimSpace(rec.AlertInstanceID) != "" {
		inst, ok, err := d.AlertInstanceStore.GetAlertInstance(strings.TrimSpace(rec.AlertInstanceID))
		if err == nil && ok && d.AlertStore != nil {
			rule, rok, ruleErr := d.AlertStore.GetAlertRule(strings.TrimSpace(inst.RuleID))
			if ruleErr == nil && rok {
				payload := buildAlertNotificationPayload(rule, inst.ID, retryStateForInstance(inst.Status), d.collectRuleGroupIDs(rule))
				payload["retry"] = true
				return payload
			}
		}
	}

	payload := map[string]any{
		"event":             "notification.retry",
		"alert_instance_id": strings.TrimSpace(rec.AlertInstanceID),
		"alert_id":          strings.TrimSpace(rec.AlertInstanceID),
		"title":             "LabTether notification retry",
		"text":              fmt.Sprintf("Retrying notification delivery for alert instance %s", strings.TrimSpace(rec.AlertInstanceID)),
		"retry":             true,
	}
	return payload
}

func retryStateForInstance(status string) string {
	switch strings.TrimSpace(status) {
	case alerts.InstanceStatusResolved:
		return "resolved"
	default:
		return "firing"
	}
}

// runNotificationRetryLoop periodically checks for pending retries and
// re-dispatches them. It runs every 30 seconds.
func (d *Deps) RunNotificationRetryLoop(ctx context.Context) {
	const retryInterval = 30 * time.Second
	for {
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("notifications: retry loop stopped")
			return
		case <-timer.C:
			d.RetryPendingNotifications(ctx)
			d.ProcessDuePushDigests(ctx)
		}
	}
}
