package alerting

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"sort"
	"strings"
	"time"
)

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
