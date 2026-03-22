package alerting

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
)

const (
	notificationRouteScanLimit   = 500
	notificationDispatchTimeout  = 15 * time.Second
	notificationDefaultRouteName = "default"
)

// dispatchAlertNotifications evaluates configured routes and sends notifications
// for the provided alert instance state transition.
func (d *Deps) DispatchAlertNotifications(rule alerts.Rule, instanceID, state string) {
	if d.NotificationStore == nil {
		return
	}
	if strings.TrimSpace(instanceID) == "" {
		return
	}
	limiter := d.NotificationSem
	if limiter == nil {
		d.dispatchAlertNotificationsSync(rule, instanceID, state)
		return
	}

	d.NotificationWG.Add(1)
	go func() {
		defer d.NotificationWG.Done()
		limiter <- struct{}{}
		defer func() {
			<-limiter
		}()
		d.dispatchAlertNotificationsSync(rule, instanceID, state)
	}()
}

func (d *Deps) dispatchAlertNotificationsAsync(rule alerts.Rule, instanceID, state string) {
	d.DispatchAlertNotifications(rule, instanceID, state)
}

func (d *Deps) dispatchAlertNotificationsSync(rule alerts.Rule, instanceID, state string) {
	if d.NotificationStore == nil {
		return
	}
	if strings.TrimSpace(instanceID) == "" {
		return
	}

	routes, err := d.NotificationStore.ListAlertRoutes(notificationRouteScanLimit)
	if err != nil {
		log.Printf("notifications: failed to list alert routes: %v", err)
		return
	}
	if len(routes) == 0 {
		return
	}

	predicateContext, _ := d.BuildAlertPredicateContext(rule, nil)

	payload := buildAlertNotificationPayload(rule, instanceID, state, predicateContext.GroupIDs)
	seenChannelsByRoute := make(map[string]map[string]struct{}, len(routes))
	channelCache := make(map[string]notifications.Channel, 16)
	missingChannels := make(map[string]struct{}, 8)

	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		if !d.RouteMatchesAlert(route, rule, state, predicateContext) {
			continue
		}
		if len(route.ChannelIDs) == 0 {
			continue
		}

		routeID := strings.TrimSpace(route.ID)
		if routeID == "" {
			routeID = notificationDefaultRouteName
		}
		if _, ok := seenChannelsByRoute[routeID]; !ok {
			seenChannelsByRoute[routeID] = make(map[string]struct{}, len(route.ChannelIDs))
		}

		for _, channelID := range route.ChannelIDs {
			channelID = strings.TrimSpace(channelID)
			if channelID == "" {
				continue
			}
			if _, dup := seenChannelsByRoute[routeID][channelID]; dup {
				continue
			}
			seenChannelsByRoute[routeID][channelID] = struct{}{}

			channel, ok := channelCache[channelID]
			if !ok {
				if _, missing := missingChannels[channelID]; missing {
					continue
				}
				loadedChannel, loaded, loadErr := d.NotificationStore.GetNotificationChannel(channelID)
				if loadErr != nil {
					log.Printf("notifications: failed to load channel %s: %v", channelID, loadErr)
					continue
				}
				if !loaded {
					missingChannels[channelID] = struct{}{}
					log.Printf("notifications: route %s references unknown channel %s", routeID, channelID)
					continue
				}
				channelCache[channelID] = loadedChannel
				channel = loadedChannel
			}
			if !channel.Enabled {
				d.recordNotificationHistory(channelID, instanceID, routeID, notifications.RecordStatusFailed, "channel disabled")
				continue
			}

			sendCtx, cancel := context.WithTimeout(context.Background(), notificationDispatchTimeout)
			sendErr := d.sendNotification(sendCtx, channel, payload)
			cancel()
			if sendErr != nil {
				rec := d.recordNotificationHistoryWithRetry(channel.ID, instanceID, routeID, notifications.RecordStatusFailed, sendErr.Error())
				log.Printf("notifications: channel %s send failed (retry %d/%d): %v", channel.ID, rec.RetryCount, rec.MaxRetries, sendErr)
				continue
			}
			d.recordNotificationHistory(channel.ID, instanceID, routeID, notifications.RecordStatusSent, "")
		}
	}
}

func (d *Deps) WaitForNotificationDispatches() {
	if d == nil || d.NotificationWG == nil {
		return
	}
	d.NotificationWG.Wait()
}

func (d *Deps) RouteMatchesAlert(route notifications.Route, rule alerts.Rule, state string, predicateContext AlertPredicateContext) bool {
	severityFilter := strings.TrimSpace(route.SeverityFilter)
	if severityFilter != "" && !strings.EqualFold(severityFilter, rule.Severity) {
		return false
	}

	groupFilter := strings.TrimSpace(route.GroupFilter)
	if groupFilter != "" {
		if _, ok := predicateContext.GroupIDs[groupFilter]; !ok {
			return false
		}
	}

	for rawKey, rawValue := range route.Matchers {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		expectedValues := selectorStringValues(rawValue)
		if key == "" || len(expectedValues) == 0 {
			continue
		}
		if _, deprecated := DeprecatedCanonicalPredicateKeys[key]; deprecated {
			return false
		}

		switch key {
		case "severity", "alert_severity":
			if !valueMatchesExpected(rule.Severity, expectedValues) {
				return false
			}
		case "state", "alert_state":
			if !valueMatchesExpected(state, expectedValues) {
				return false
			}
		case "rule_id", "alert_rule_id":
			if !valueMatchesExpected(rule.ID, expectedValues) {
				return false
			}
		case "group_id":
			if !setContainsAny(predicateContext.GroupIDs, normalizeSelectorValues(expectedValues, normalizeSelectorToken)) {
				return false
			}
		case "resource_kind":
			if !setContainsAny(predicateContext.ResourceKinds, normalizeSelectorValues(expectedValues, normalizeKindToken)) {
				return false
			}
		case "resource_class":
			if !setContainsAny(predicateContext.ResourceClasses, normalizeSelectorValues(expectedValues, normalizeSelectorToken)) {
				return false
			}
		case "capability":
			if !setContainsAny(predicateContext.Capabilities, normalizeSelectorValues(expectedValues, normalizeSelectorToken)) {
				return false
			}
		case "capabilities_any":
			if !setContainsAny(predicateContext.Capabilities, normalizeSelectorValues(expectedValues, normalizeSelectorToken)) {
				return false
			}
		case "capabilities_all":
			if !setContainsAll(predicateContext.Capabilities, normalizeSelectorValues(expectedValues, normalizeSelectorToken)) {
				return false
			}
		default:
			labelValue := rule.Labels[key]
			if labelValue == "" {
				labelValue = rule.Labels[strings.TrimSpace(rawKey)]
			}
			if !valueMatchesExpected(labelValue, expectedValues) {
				return false
			}
		}
	}

	return true
}

func (d *Deps) collectRuleGroupIDs(rule alerts.Rule) map[string]struct{} {
	out := make(map[string]struct{}, len(rule.Targets))
	for _, target := range rule.Targets {
		groupID := strings.TrimSpace(target.GroupID)
		if groupID != "" {
			out[groupID] = struct{}{}
		}
	}

	targetAssets, err := d.ResolveRuleTargetAssets(rule, nil, true)
	if err != nil {
		return out
	}
	for _, targetAsset := range targetAssets {
		if groupID := strings.TrimSpace(targetAsset.GroupID); groupID != "" {
			out[groupID] = struct{}{}
		}
	}

	return out
}

func (d *Deps) sendNotification(ctx context.Context, channel notifications.Channel, payload map[string]any) error {
	channelType := notifications.NormalizeChannelType(channel.Type)
	if channelType == "" {
		return fmt.Errorf("unsupported channel type")
	}

	var adapter notifications.Adapter
	if d.NotificationAdapters != nil {
		adapter = d.NotificationAdapters[channelType]
	}
	if adapter == nil {
		return fmt.Errorf("notification adapter unavailable for channel type %s", channelType)
	}

	switch channelType {
	case notifications.ChannelTypeEmail:
		emailPayload, err := buildEmailNotificationPayload(channel.Config, payload)
		if err != nil {
			return err
		}
		return adapter.Send(ctx, channel.Config, emailPayload)
	case notifications.ChannelTypeSlack:
		return adapter.Send(ctx, channel.Config, buildSlackNotificationPayload(payload))
	default:
		return adapter.Send(ctx, channel.Config, payload)
	}
}

func (d *Deps) recordNotificationHistory(channelID, instanceID, routeID, status, errorMessage string) {
	if d.NotificationStore == nil {
		return
	}
	_, err := d.NotificationStore.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID:       strings.TrimSpace(channelID),
		AlertInstanceID: strings.TrimSpace(instanceID),
		RouteID:         strings.TrimSpace(routeID),
		Status:          strings.TrimSpace(status),
		Error:           strings.TrimSpace(errorMessage),
	})
	if err != nil {
		log.Printf("notifications: failed to persist notification history (channel=%s route=%s): %v", channelID, routeID, err)
	}
}

// recordNotificationHistoryWithRetry creates a failed notification record and
// schedules it for retry by setting next_retry_at via RetryBackoff(0).
// It returns the created record so the caller can log retry metadata.
func (d *Deps) recordNotificationHistoryWithRetry(channelID, instanceID, routeID, status, errorMessage string) notifications.Record {
	if d.NotificationStore == nil {
		return notifications.Record{}
	}
	rec, err := d.NotificationStore.CreateNotificationRecord(notifications.CreateRecordRequest{
		ChannelID:       strings.TrimSpace(channelID),
		AlertInstanceID: strings.TrimSpace(instanceID),
		RouteID:         strings.TrimSpace(routeID),
		Status:          strings.TrimSpace(status),
		Error:           strings.TrimSpace(errorMessage),
	})
	if err != nil {
		log.Printf("notifications: failed to persist notification history (channel=%s route=%s): %v", channelID, routeID, err)
		return notifications.Record{}
	}
	// Schedule first retry.
	nextRetry := time.Now().UTC().Add(notifications.RetryBackoff(0))
	updateErr := d.NotificationStore.UpdateRetryState(context.Background(), rec.ID, 0, &nextRetry, notifications.RecordStatusFailed)
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
		channel, ok, loadErr := d.NotificationStore.GetNotificationChannel(rec.ChannelID)
		if loadErr != nil {
			log.Printf("notifications: retry load channel %s failed: %v", rec.ChannelID, loadErr)
			continue
		}
		if !ok || !channel.Enabled {
			// Channel gone or disabled — exhaust retries immediately.
			d.exhaustNotificationRetry(ctx, rec, "channel unavailable or disabled")
			continue
		}

		payload := map[string]any{
			"alert_instance_id": rec.AlertInstanceID,
			"retry":             true,
		}
		sendCtx, cancel := context.WithTimeout(ctx, notificationDispatchTimeout)
		sendErr := d.sendNotification(sendCtx, channel, payload)
		cancel()

		newRetryCount := rec.RetryCount + 1
		if sendErr == nil {
			// Success — clear retry state and mark sent.
			clearUpdateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, newRetryCount, nil, notifications.RecordStatusSent)
			if clearUpdateErr != nil {
				log.Printf("notifications: failed to mark retry success for record %s: %v", rec.ID, clearUpdateErr)
			}
			log.Printf("notifications: retry succeeded for record %s (attempt %d)", rec.ID, newRetryCount)
			continue
		}

		log.Printf("notifications: retry %d/%d failed for record %s: %v", newRetryCount, rec.MaxRetries, rec.ID, sendErr)

		if newRetryCount >= rec.MaxRetries {
			d.exhaustNotificationRetry(ctx, rec, sendErr.Error())
			continue
		}

		// Schedule next retry.
		nextRetry := now.Add(notifications.RetryBackoff(newRetryCount))
		updateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, newRetryCount, &nextRetry, notifications.RecordStatusFailed)
		if updateErr != nil {
			log.Printf("notifications: failed to update retry state for record %s: %v", rec.ID, updateErr)
		}
	}
}

// exhaustNotificationRetry marks a record as permanently failed by setting
// retry_count = max_retries and clearing next_retry_at.
func (d *Deps) exhaustNotificationRetry(ctx context.Context, rec notifications.Record, reason string) {
	updateErr := d.NotificationStore.UpdateRetryState(ctx, rec.ID, rec.MaxRetries, nil, notifications.RecordStatusFailed)
	if updateErr != nil {
		log.Printf("notifications: failed to exhaust retry for record %s: %v", rec.ID, updateErr)
	}
	log.Printf("notifications: record %s permanently failed after %d retries: %s", rec.ID, rec.MaxRetries, reason)
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
		}
	}
}

func buildAlertNotificationPayload(rule alerts.Rule, instanceID, state string, groupIDs map[string]struct{}) map[string]any {
	now := time.Now().UTC()
	alertID := strings.TrimSpace(instanceID)
	targets := make([]map[string]string, 0, len(rule.Targets))
	for _, target := range rule.Targets {
		targets = append(targets, map[string]string{
			"id":       strings.TrimSpace(target.ID),
			"asset_id": strings.TrimSpace(target.AssetID),
			"group_id": strings.TrimSpace(target.GroupID),
		})
	}

	groupList := make([]string, 0, len(groupIDs))
	for groupID := range groupIDs {
		groupList = append(groupList, groupID)
	}
	sort.Strings(groupList)

	title := fmt.Sprintf("[%s] %s (%s)", strings.ToUpper(strings.TrimSpace(rule.Severity)), strings.TrimSpace(rule.Name), strings.TrimSpace(state))
	description := strings.TrimSpace(rule.Description)
	if description == "" {
		description = "No description provided."
	}

	return map[string]any{
		"event":             "alert." + strings.TrimSpace(state),
		"state":             strings.TrimSpace(state),
		"alert_instance_id": alertID,
		"alert_id":          alertID,
		"rule_id":           strings.TrimSpace(rule.ID),
		"rule_name":         strings.TrimSpace(rule.Name),
		"severity":          strings.TrimSpace(rule.Severity),
		"description":       description,
		"title":             title,
		"text":              fmt.Sprintf("%s\nRule: %s (%s)\nState: %s\nInstance: %s\nOccurred: %s", description, rule.Name, rule.ID, state, instanceID, now.Format(time.RFC3339)),
		"occurred_at":       now.Format(time.RFC3339),
		"target_scope":      strings.TrimSpace(rule.TargetScope),
		"group_ids":         groupList,
		"targets":           targets,
		"labels":            cloneAlertLabels(rule.Labels),
		"deep_link":         fmt.Sprintf("labtether://alerts/%s", alertID),
		"apns_category":     "LT_ALERT_ACTIONS",
	}
}

func buildEmailNotificationPayload(config map[string]any, payload map[string]any) (map[string]any, error) {
	to := firstNonBlank(
		configString(config, "to"),
		configString(config, "recipients"),
		configString(config, "email_to"),
	)
	if to == "" {
		return nil, fmt.Errorf("email channel config missing recipient address")
	}

	severity := strings.ToUpper(payloadString(payload, "severity"))
	ruleName := payloadString(payload, "rule_name")
	state := payloadString(payload, "state")
	subject := fmt.Sprintf("[%s] %s (%s)", severity, ruleName, state)
	if prefix := configString(config, "subject_prefix"); prefix != "" {
		subject = strings.TrimSpace(prefix + " " + subject)
	}

	body := payloadString(payload, "text")
	if body == "" {
		body = fmt.Sprintf("Alert %s for rule %s (%s)", state, ruleName, severity)
	}

	return map[string]any{
		"to":      to,
		"subject": subject,
		"body":    body,
	}, nil
}

func buildSlackNotificationPayload(payload map[string]any) map[string]any {
	return map[string]any{
		"title": payloadString(payload, "title"),
		"text":  payloadString(payload, "text"),
	}
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(notificationAnyToString(payload[key]))
}

func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	return strings.TrimSpace(notificationAnyToString(config[key]))
}

func notificationAnyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%f", typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func cloneAlertLabels(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// normalizeSelectorToken, normalizeSelectorValues, setContainsAny,
// setContainsAll, and valueMatchesExpected are defined in alert_predicates.go
// and shared across the package.
