package alerting

import (
	"strings"

	"github.com/labtether/labtether/internal/alerts"
)

// Silence matcher keys with a meaning of their own; every other key names a
// rule label.
const (
	silenceKeyRuleID  = "rule_id"
	silenceKeyAssetID = "asset_id"
)

// matchesSilence reports whether a silence covers a whole rule: each matcher
// equals the rule's ID (rule_id) or one of its labels. A silence that names a
// device (asset_id) covers the whole rule only when the rule's own asset_id
// label is that device; otherwise it covers that device's part of an alert
// (deviceSilenced).
func matchesSilence(rule alerts.Rule, silence alerts.AlertSilence) bool {
	return silenceMatches(rule, silence, "")
}

func silenceMatches(rule alerts.Rule, silence alerts.AlertSilence, device string) bool {
	if len(silence.Matchers) == 0 {
		return false
	}
	for key, want := range silence.Matchers {
		switch key {
		case silenceKeyRuleID:
			if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.ID) != strings.TrimSpace(want) {
				return false
			}
		case silenceKeyAssetID:
			if label, ok := rule.Labels[silenceKeyAssetID]; ok && label == want {
				continue
			}
			if device == "" || device != strings.TrimSpace(want) {
				return false
			}
		default:
			if value, ok := rule.Labels[key]; !ok || value != want {
				return false
			}
		}
	}
	return true
}

// deviceSilenced reports whether a silence that names this device covers the
// rule's alert on it.
func deviceSilenced(rule alerts.Rule, silences []alerts.AlertSilence, device string) bool {
	for _, silence := range silences {
		if _, named := silence.Matchers[silenceKeyAssetID]; named && silenceMatches(rule, silence, device) {
			return true
		}
	}
	return false
}

// withoutSilencedDevices drops the devices a silence covers from a firing
// scope. allSilenced is true when the scope named devices and every one is
// silenced: the alert then waits like a silenced rule. The rule keeps firing
// for its other devices.
func withoutSilencedDevices(rule alerts.Rule, scope alerts.FiringScope, silences []alerts.AlertSilence) (alerts.FiringScope, bool) {
	if len(scope.AssetIDs) == 0 {
		return scope, false
	}
	kept := make([]string, 0, len(scope.AssetIDs))
	for _, id := range scope.AssetIDs {
		if !deviceSilenced(rule, silences, id) {
			kept = append(kept, id)
		}
	}
	scope.AssetIDs = kept
	return scope, len(kept) == 0
}

// devicesSilenced reports whether an alert's devices are all silenced, so
// its messages stop. An alert with no device is silenced only by its rule.
func devicesSilenced(rule alerts.Rule, silences []alerts.AlertSilence, devices []string) bool {
	if len(devices) == 0 {
		return false
	}
	for _, id := range devices {
		if !deviceSilenced(rule, silences, id) {
			return false
		}
	}
	return true
}
