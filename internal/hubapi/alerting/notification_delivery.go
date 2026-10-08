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
	_ "time/tzdata"
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
	// Re-check at actual delivery time so an asynchronous send accepted just
	// before a maintenance window cannot leak through after suppression begins.
	if d.isAlertMaintenanceSuppressed(rule) {
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
				loadedChannel, loaded, loadErr := d.getNotificationChannelForRuntime(channelID)
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
				d.recordNotificationHistory(channelID, instanceID, routeID, notifications.RecordStatusFailed, "channel disabled", payload)
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
				rec := d.recordNotificationHistoryWithRetry(channel.ID, instanceID, routeID, notifications.RecordStatusFailed, safeError, failedPayload)
				securityruntime.Logf("notifications: channel %s send failed (retry %d/%d)", channel.ID, rec.RetryCount, rec.MaxRetries)
				continue
			}
			d.recordNotificationHistory(channel.ID, instanceID, routeID, notifications.RecordStatusSent, "", payload)
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
	case notifications.ChannelTypeAPNs:
		return d.sendAPNsNotification(ctx, adapter, channel, payload)
	default:
		return adapter.Send(ctx, channel.Config, payload)
	}
}
