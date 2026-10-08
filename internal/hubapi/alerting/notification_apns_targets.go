package alerting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"net/url"
	"strings"
)

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
