package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
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

const apnsRetryTargetsPayloadKey = "_labtether_apns_retry_targets"

type apnsRetryTarget struct {
	deviceID         string
	tokenFingerprint string
	bundleID         string
	environment      string
}

func newAPNsRetryTarget(device persistence.PushDevice, bundleID, environment string) apnsRetryTarget {
	target := apnsRetryTarget{
		deviceID:    strings.TrimSpace(device.ID),
		bundleID:    strings.TrimSpace(bundleID),
		environment: strings.ToLower(strings.TrimSpace(environment)),
	}
	if target.deviceID == "" {
		digest := sha256.Sum256([]byte(strings.TrimSpace(device.PushToken)))
		target.tokenFingerprint = hex.EncodeToString(digest[:])
	}
	return target
}

func (t apnsRetryTarget) valid() bool {
	if t.bundleID == "" || (t.environment != "sandbox" && t.environment != "production") {
		return false
	}
	identities := 0
	if t.deviceID != "" {
		identities++
	}
	if t.tokenFingerprint != "" {
		if len(t.tokenFingerprint) != sha256.Size*2 {
			return false
		}
		if _, err := hex.DecodeString(t.tokenFingerprint); err != nil {
			return false
		}
		identities++
	}
	return identities == 1
}

func (t apnsRetryTarget) payloadValue() map[string]any {
	value := map[string]any{
		"bundle_id":   t.bundleID,
		"environment": t.environment,
	}
	if t.deviceID != "" {
		value["device_id"] = t.deviceID
	} else {
		value["token_fingerprint"] = t.tokenFingerprint
	}
	return value
}

type apnsDeliveryTarget struct {
	token       string
	retryTarget apnsRetryTarget
}

type apnsDeliveryGroup struct {
	bundleID    string
	environment string
	targets     []apnsDeliveryTarget
	seenTokens  map[string]struct{}
}

func (g *apnsDeliveryGroup) tokens() []string {
	if g == nil || len(g.targets) == 0 {
		return nil
	}
	tokens := make([]string, 0, len(g.targets))
	for _, target := range g.targets {
		tokens = append(tokens, target.token)
	}
	return tokens
}

type apnsFanoutError struct {
	totalTargets int
	totalGroups  int
	failedGroups int
	failures     []apnsRetryTarget
	details      []string
}

func (e *apnsFanoutError) Error() string {
	if e == nil {
		return "APNs fanout failed"
	}
	message := fmt.Sprintf(
		"APNs fanout failed for %d/%d targets across %d/%d groups",
		len(e.failures),
		e.totalTargets,
		e.failedGroups,
		e.totalGroups,
	)
	if len(e.details) > 0 {
		message += ": " + strings.Join(e.details, "; ")
	}
	return message
}

func payloadWithAPNsRetryTargets(payload map[string]any, err error) (map[string]any, bool) {
	var fanoutErr *apnsFanoutError
	if !errors.As(err, &fanoutErr) || len(fanoutErr.failures) == 0 {
		return nil, false
	}
	targets := make([]map[string]any, 0, len(fanoutErr.failures))
	seen := make(map[apnsRetryTarget]struct{}, len(fanoutErr.failures))
	for _, target := range fanoutErr.failures {
		if !target.valid() {
			continue
		}
		if _, duplicate := seen[target]; duplicate {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target.payloadValue())
	}
	if len(targets) == 0 {
		return nil, false
	}
	retryPayload := cloneAnyMap(payload)
	retryPayload[apnsRetryTargetsPayloadKey] = targets
	return retryPayload, true
}

func apnsRetryTargetSetFromPayload(payload map[string]any) (map[apnsRetryTarget]struct{}, bool, error) {
	rawTargets, restricted := payload[apnsRetryTargetsPayloadKey]
	if !restricted {
		return nil, false, nil
	}
	rawValues := make([]any, 0)
	switch typed := rawTargets.(type) {
	case []any:
		rawValues = append(rawValues, typed...)
	case []map[string]any:
		for _, rawTarget := range typed {
			rawValues = append(rawValues, rawTarget)
		}
	default:
		return nil, true, fmt.Errorf("target list has an invalid type")
	}
	if len(rawValues) == 0 {
		return nil, true, fmt.Errorf("target list is empty")
	}
	targets := make(map[apnsRetryTarget]struct{})
	for _, rawTarget := range rawValues {
		targetMap, ok := notificationAnyMap(rawTarget)
		if !ok {
			return nil, true, fmt.Errorf("target entry has an invalid type")
		}
		target := apnsRetryTarget{
			deviceID:         strings.TrimSpace(notificationAnyToString(targetMap["device_id"])),
			tokenFingerprint: strings.ToLower(strings.TrimSpace(notificationAnyToString(targetMap["token_fingerprint"]))),
			bundleID:         strings.TrimSpace(notificationAnyToString(targetMap["bundle_id"])),
			environment:      strings.ToLower(strings.TrimSpace(notificationAnyToString(targetMap["environment"]))),
		}
		if !target.valid() {
			return nil, true, fmt.Errorf("target entry is invalid")
		}
		targets[target] = struct{}{}
	}
	return targets, true, nil
}

func failedAPNsTargetsForGroup(group *apnsDeliveryGroup, err error) []apnsRetryTarget {
	if group == nil || len(group.targets) == 0 {
		return nil
	}
	indices, indexed := notifications.APNsFailedDeliveryIndices(err)
	if !indexed || len(indices) == 0 {
		return allAPNsTargetsForGroup(group)
	}
	seen := make(map[int]struct{}, len(indices))
	failed := make([]apnsRetryTarget, 0, len(indices))
	for _, index := range indices {
		if index < 0 || index >= len(group.targets) {
			// An invalid positional outcome cannot safely identify a subset.
			// Retry the group rather than risk silently dropping a failure.
			return allAPNsTargetsForGroup(group)
		}
		if _, duplicate := seen[index]; duplicate {
			continue
		}
		seen[index] = struct{}{}
		failed = append(failed, group.targets[index].retryTarget)
	}
	return failed
}

func allAPNsTargetsForGroup(group *apnsDeliveryGroup) []apnsRetryTarget {
	targets := make([]apnsRetryTarget, 0, len(group.targets))
	for _, target := range group.targets {
		targets = append(targets, target.retryTarget)
	}
	return targets
}

func sanitizedAPNsGroupError(err error, group *apnsDeliveryGroup) string {
	if err == nil {
		return "unknown APNs delivery error"
	}
	message := err.Error()
	for _, target := range group.targets {
		for _, secret := range []string{target.token, url.PathEscape(target.token)} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[redacted]")
			}
		}
	}
	return notifications.SanitizeDeliveryErrorMessage(message)
}

// sendAPNsNotification loads registered iOS devices, applies the preferences
// that can be evaluated deterministically at dispatch time, then partitions
// immediate delivery by APNs topic/environment and durably queues eligible
// non-urgent alerts into per-device server-side digests.
func (d *Deps) sendAPNsNotification(
	ctx context.Context,
	adapter notifications.Adapter,
	channel notifications.Channel,
	payload map[string]any,
) error {
	baseConfig := channel.Config
	if d.PushDeviceStore == nil {
		// Preserve support for direct adapter tests and explicitly supplied token
		// lists in callers that do not have the hub persistence layer available.
		return adapter.Send(ctx, baseConfig, payload)
	}

	devices, err := d.PushDeviceStore.GetAllPushTokens(ctx)
	if err != nil {
		return fmt.Errorf("load registered APNs devices: %w", err)
	}

	defaultBundleID := configString(baseConfig, "bundle_id")
	allowedBundleIDs := allowedAPNsBundleIDs(baseConfig, defaultBundleID)
	defaultEnvironment := "sandbox"
	if production, _ := baseConfig["production"].(bool); production {
		defaultEnvironment = "production"
	}

	retryTargets, retryRestricted, retryRestrictionErr := apnsRetryTargetSetFromPayload(payload)
	if retryRestrictionErr != nil {
		// Never expand corrupt target metadata back into a fleet-wide fanout.
		// Returning an error also preserves failure accounting until the bounded
		// retry budget is exhausted or an operator repairs the record.
		return fmt.Errorf("invalid APNs retry target restriction: %w", retryRestrictionErr)
	}
	groupsByKey := make(map[string]*apnsDeliveryGroup)
	dispatchTime := time.Now()
	digestStore := d.durablePushDigestStore()
	queueDigest := shouldQueuePushDigest(channel.ID, digestStore, payload)
	digestEnqueues := make([]persistence.PushDigestEnqueue, 0)
	digestRetryTargets := make(map[string]apnsRetryTarget)
	for _, device := range devices {
		if !isApplePushPlatform(device.Platform) || strings.TrimSpace(device.PushToken) == "" {
			continue
		}
		// Quiet hours defer a durable digest; they must not discard the event
		// before it reaches the queue. All other current preferences are still
		// enforced at enqueue time and are rechecked again before APNs delivery.
		if queueDigest {
			if !pushDeviceAllowsDigestPayloadAt(device, payload, dispatchTime) {
				continue
			}
		} else if !pushDeviceAllowsPayloadAt(device, payload, dispatchTime) {
			continue
		}

		bundleID := firstNonBlank(device.BundleID, defaultBundleID)
		if _, allowed := allowedBundleIDs[bundleID]; bundleID == "" || !allowed {
			continue
		}
		environment := strings.ToLower(strings.TrimSpace(device.Environment))
		if environment != "sandbox" && environment != "production" {
			environment = defaultEnvironment
		}

		token := strings.TrimSpace(device.PushToken)
		retryTarget := newAPNsRetryTarget(device, bundleID, environment)
		if retryRestricted {
			if _, selected := retryTargets[retryTarget]; !selected {
				continue
			}
		}
		if queueDigest {
			if enqueue, ok := buildPushDigestEnqueue(device, channel.ID, payload, dispatchTime); ok {
				digestEnqueues = append(digestEnqueues, enqueue)
				digestRetryTargets[enqueue.DeviceID] = retryTarget
				continue
			}
		}
		key := environment + "\x00" + bundleID
		group := groupsByKey[key]
		if group == nil {
			group = &apnsDeliveryGroup{
				bundleID:    bundleID,
				environment: environment,
				seenTokens:  make(map[string]struct{}),
			}
			groupsByKey[key] = group
		}
		if _, duplicate := group.seenTokens[token]; duplicate {
			continue
		}
		group.seenTokens[token] = struct{}{}
		group.targets = append(group.targets, apnsDeliveryTarget{token: token, retryTarget: retryTarget})
	}

	groups := make([]*apnsDeliveryGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		sort.Slice(group.targets, func(i, j int) bool {
			return group.targets[i].token < group.targets[j].token
		})
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].environment == groups[j].environment {
			return groups[i].bundleID < groups[j].bundleID
		}
		return groups[i].environment < groups[j].environment
	})
	if notificationPayloadBool(payload, "notification_test") && len(groups) == 0 && len(digestEnqueues) == 0 {
		return errors.New("no eligible registered APNs devices")
	}

	fanoutErr := &apnsFanoutError{totalGroups: len(groups)}
	if len(digestEnqueues) > 0 {
		fanoutErr.totalGroups++
		fanoutErr.totalTargets += len(digestEnqueues)
		result, enqueueErr := digestStore.EnqueuePushDigestEvents(ctx, digestEnqueues)
		if enqueueErr != nil {
			fanoutErr.failedGroups++
			for _, enqueue := range digestEnqueues {
				if target, ok := digestRetryTargets[enqueue.DeviceID]; ok {
					fanoutErr.failures = append(fanoutErr.failures, target)
				}
			}
			fanoutErr.details = append(fanoutErr.details, "durable digest queue: enqueue failed")
		} else if len(result.DroppedDeviceIDs) > 0 {
			fanoutErr.failedGroups++
			for _, deviceID := range result.DroppedDeviceIDs {
				if target, ok := digestRetryTargets[deviceID]; ok {
					fanoutErr.failures = append(fanoutErr.failures, target)
				}
			}
			fanoutErr.details = append(fanoutErr.details, fmt.Sprintf(
				"durable digest queue: %d device queues reached the bounded event cap",
				len(result.DroppedDeviceIDs),
			))
		}
	}
	for _, group := range groups {
		fanoutErr.totalTargets += len(group.targets)
		config := cloneAnyMap(baseConfig)
		config["bundle_id"] = group.bundleID
		config["production"] = group.environment == "production"
		config["device_tokens"] = group.tokens()
		notifications.SetAPNsInvalidDeviceTokenHandler(
			config,
			notifications.APNsInvalidDeviceTokenHandler(func(token string) error {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return d.PushDeviceStore.DeletePushDeviceByToken(
					cleanupCtx,
					token,
					group.bundleID,
					group.environment,
				)
			}),
		)
		if err := adapter.Send(ctx, config, payload); err != nil {
			fanoutErr.failedGroups++
			fanoutErr.failures = append(fanoutErr.failures, failedAPNsTargetsForGroup(group, err)...)
			fanoutErr.details = append(
				fanoutErr.details,
				fmt.Sprintf("%s/%s: %s", group.bundleID, group.environment, sanitizedAPNsGroupError(err, group)),
			)
		}
	}
	if len(fanoutErr.failures) > 0 {
		return fanoutErr
	}
	return nil
}

func allowedAPNsBundleIDs(config map[string]any, defaultBundleID string) map[string]struct{} {
	allowed := make(map[string]struct{})
	if defaultBundleID = strings.TrimSpace(defaultBundleID); defaultBundleID != "" {
		allowed[defaultBundleID] = struct{}{}
	}
	var configured []string
	switch values := config["allowed_bundle_ids"].(type) {
	case []string:
		configured = values
	case []any:
		configured = make([]string, 0, len(values))
		for _, value := range values {
			if stringValue, ok := value.(string); ok {
				configured = append(configured, stringValue)
			}
		}
	}
	for _, bundleID := range configured {
		if bundleID = strings.TrimSpace(bundleID); bundleID != "" {
			allowed[bundleID] = struct{}{}
		}
	}
	return allowed
}

func isApplePushPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "ios", "ipados":
		return true
	default:
		return false
	}
}

func pushDeviceAllowsPayload(device persistence.PushDevice, payload map[string]any) bool {
	return pushDeviceAllowsPayloadAt(device, payload, time.Now())
}

func pushDeviceAllowsPayloadAt(device persistence.PushDevice, payload map[string]any, now time.Time) bool {
	return pushDeviceAllowsPayloadAtWithQuietHours(device, payload, now, true)
}

func pushDeviceAllowsDigestPayloadAt(device persistence.PushDevice, payload map[string]any, now time.Time) bool {
	return pushDeviceAllowsPayloadAtWithQuietHours(device, payload, now, false)
}

func pushDeviceAllowsPayloadAtWithQuietHours(device persistence.PushDevice, payload map[string]any, now time.Time, enforceQuietHours bool) bool {
	severityRank := pushSeverityRank(payloadString(payload, "severity"))
	minimumRank := pushSeverityRank(device.MinimumSeverity)
	if minimumRank == 0 {
		minimumRank = pushSeverityRank("warning")
	}
	if severityRank < minimumRank {
		return false
	}
	if enforceQuietHours && device.QuietHoursEnabled &&
		severityRank < pushSeverityRank("critical") &&
		pushDeviceIsInQuietHours(device, now) {
		return false
	}

	event := strings.ToLower(payloadString(payload, "event"))
	isIncident := strings.HasPrefix(event, "incident.") || payloadString(payload, "incident_id") != ""
	isAlert := strings.HasPrefix(event, "alert.") || payloadString(payload, "alert_id") != "" || payloadString(payload, "alert_instance_id") != ""
	category := strings.ToLower(strings.TrimSpace(device.PushCategory))
	if category == "" {
		category = "critical_only"
	}
	switch category {
	case "critical_only":
		if isIncident || severityRank < pushSeverityRank("high") {
			return false
		}
	case "all_alerts":
		if isIncident || !isAlert {
			return false
		}
	case "alerts_and_incidents":
		if !isAlert && !isIncident {
			return false
		}
	default:
		return false
	}

	if isNodeOfflinePush(payload) {
		return device.NotifyNodeOffline
	}
	if isServiceDownPush(payload) {
		return device.NotifyServiceDown
	}
	if isAlert && severityRank >= pushSeverityRank("high") {
		return device.NotifyCriticalAlerts
	}
	return true
}

func pushDeviceIsInQuietHours(device persistence.PushDevice, now time.Time) bool {
	location, err := time.LoadLocation(strings.TrimSpace(device.TimeZone))
	if err != nil {
		// Legacy or corrupt registrations fail open: dropping a critical fleet
		// signal without a trustworthy local clock would be worse than delivery.
		return false
	}
	local := now.In(location)
	currentMinutes := local.Hour()*60 + local.Minute()
	start := device.QuietHoursStartMinutes
	end := device.QuietHoursEndMinutes
	if start < 0 || start > 1439 || end < 0 || end > 1439 || start == end {
		return false
	}
	if start < end {
		return currentMinutes >= start && currentMinutes < end
	}
	return currentMinutes >= start || currentMinutes < end
}

func pushSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return 4
	case "high", "major":
		return 3
	case "warning", "medium":
		return 2
	case "info", "low":
		return 1
	default:
		return 0
	}
}

func isNodeOfflinePush(payload map[string]any) bool {
	if strings.EqualFold(payloadString(payload, "rule_kind"), alerts.RuleKindHeartbeatStale) {
		return true
	}
	signal := strings.ToLower(payloadMapString(payload, "labels", "signal"))
	return signal == "offline" || signal == "node_offline" || signal == "agent_offline"
}

func isServiceDownPush(payload map[string]any) bool {
	signal := strings.ToLower(payloadMapString(payload, "labels", "signal"))
	return strings.Contains(signal, "service_down") || strings.Contains(signal, "down_transition")
}

func payloadMapString(payload map[string]any, mapKey, valueKey string) string {
	if payload == nil {
		return ""
	}
	switch values := payload[mapKey].(type) {
	case map[string]string:
		return strings.TrimSpace(values[valueKey])
	case map[string]any:
		return strings.TrimSpace(notificationAnyToString(values[valueKey]))
	default:
		return ""
	}
}

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

	payload := map[string]any{
		"event":             "alert." + strings.TrimSpace(state),
		"state":             strings.TrimSpace(state),
		"alert_instance_id": alertID,
		"alert_id":          alertID,
		"rule_id":           strings.TrimSpace(rule.ID),
		"rule_name":         strings.TrimSpace(rule.Name),
		"rule_kind":         strings.TrimSpace(rule.Kind),
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
	}
	if strings.EqualFold(strings.TrimSpace(state), "firing") {
		payload["apns_category"] = "LT_ALERT_ACTIONS"
	}
	return payload
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
	ruleName := firstNonBlank(
		payloadString(payload, "rule_name"),
		payloadString(payload, "title"),
		"LabTether notification",
	)
	state := payloadString(payload, "state")
	subject := ruleName
	if severity != "" {
		subject = fmt.Sprintf("[%s] %s", severity, subject)
	}
	if state != "" {
		subject = fmt.Sprintf("%s (%s)", subject, state)
	}
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

func notificationPayloadBool(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	switch typed := payload[key].(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
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
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return ""
		}
		if typed >= minInt64AsFloat && typed < maxInt64ExclusiveAsFloat && math.Trunc(typed) == typed {
			return fmt.Sprintf("%d", int64(typed))
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

const (
	minInt64AsFloat          = -9223372036854775808.0
	maxInt64ExclusiveAsFloat = 9223372036854775808.0
)

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
