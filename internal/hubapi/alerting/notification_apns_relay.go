package alerting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

// apnsChannelRouting describes which registrations an APNs channel may send
// to. A relay channel reaches only relay registrations of the official app; a
// direct channel reaches only raw tokens for its configured bundle ids.
// Sending across transports would make APNs or the relay reject a valid
// registration and the cleanup handler would then delete it.
type apnsChannelRouting struct {
	transport       string
	defaultBundleID string
	allowedBundles  map[string]struct{}
}

// apnsChannelUsesRelay reads the explicit transport. Channels are stored with
// it (normalizeAPNsChannelConfig runs on create and update), so dispatch never
// infers relay mode from missing key fields in a legacy or partial config.
func apnsChannelUsesRelay(config map[string]any) bool {
	return strings.EqualFold(configString(config, "transport"), notifications.APNsTransportRelay)
}

func apnsRoutingForChannel(config map[string]any) apnsChannelRouting {
	if apnsChannelUsesRelay(config) {
		return apnsChannelRouting{
			transport:       notifications.APNsTransportRelay,
			defaultBundleID: notifications.OfficialAppBundleID,
			allowedBundles:  map[string]struct{}{notifications.OfficialAppBundleID: {}},
		}
	}
	defaultBundleID := configString(config, "bundle_id")
	return apnsChannelRouting{
		transport:       notifications.APNsTransportDirect,
		defaultBundleID: defaultBundleID,
		allowedBundles:  allowedAPNsBundleIDs(config, defaultBundleID),
	}
}

// deliveryConfig clones a channel config for one send and pins the transport,
// so the adapter and this routing decision can never disagree.
func (r apnsChannelRouting) deliveryConfig(config map[string]any) map[string]any {
	cloned := cloneAnyMap(config)
	cloned["transport"] = r.transport
	return cloned
}

func (r apnsChannelRouting) accepts(device persistence.PushDevice) bool {
	return notifications.NormalizeAPNsTransport(device.PushTransport) == r.transport
}

// bundleFor returns the device's APNs topic, or "" when this channel may not
// deliver to it.
func (r apnsChannelRouting) bundleFor(device persistence.PushDevice) string {
	bundleID := firstNonBlank(device.BundleID, r.defaultBundleID)
	if _, allowed := r.allowedBundles[bundleID]; bundleID == "" || !allowed {
		return ""
	}
	return bundleID
}

var apnsOperatorKeyFields = []string{"auth_key_path", "key_id", "team_id"}

// normalizeAPNsChannelConfig makes relay channels explicit before storage.
// An update that switches a channel to transport "relay" drops the operator
// key fields that secret-preserving merges would otherwise carry over.
func normalizeAPNsChannelConfig(channelType string, config map[string]any) map[string]any {
	if notifications.NormalizeChannelType(channelType) != notifications.ChannelTypeAPNs || config == nil {
		return config
	}
	transport := strings.ToLower(configString(config, "transport"))
	if transport != notifications.APNsTransportRelay && (transport != "" || !notifications.APNsUsesRelay(config)) {
		return config
	}
	normalized := cloneAnyMap(config)
	if transport == notifications.APNsTransportRelay {
		for _, key := range []string{"auth_key_path", "key_id", "team_id", "allowed_bundle_ids"} {
			delete(normalized, key)
		}
	}
	normalized["transport"] = notifications.APNsTransportRelay
	if configString(normalized, "bundle_id") == "" {
		normalized["bundle_id"] = notifications.OfficialAppBundleID
	}
	return normalized
}

// validateAPNsRelayConfig accepts only settings that mean something for the
// relay. The relay serves the official app; operators with their own app
// build use transport "direct" with their own key.
func validateAPNsRelayConfig(config map[string]any) error {
	for _, key := range apnsOperatorKeyFields {
		if configString(config, key) != "" {
			return fmt.Errorf("%s is not used with the LabTether push relay; set transport to direct to use your own APNs key", key)
		}
	}
	if _, present := config["allowed_bundle_ids"]; present {
		return errors.New("allowed_bundle_ids is not used with the LabTether push relay")
	}
	if bundleID := configString(config, "bundle_id"); bundleID != "" && bundleID != notifications.OfficialAppBundleID {
		return fmt.Errorf("the LabTether push relay only serves %s", notifications.OfficialAppBundleID)
	}
	if production, present := config["production"]; present {
		if _, ok := production.(bool); !ok {
			return errors.New("production must be a boolean")
		}
	}
	return nil
}

func validateAPNsTransport(config map[string]any) error {
	switch strings.ToLower(configString(config, "transport")) {
	case "", notifications.APNsTransportDirect, notifications.APNsTransportRelay:
		return nil
	default:
		return errors.New("transport must be direct or relay")
	}
}
