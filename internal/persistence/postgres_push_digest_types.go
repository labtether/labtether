package persistence

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// MaxPushDigestEventsPerDevice bounds durable work even if an alert storm
	// targets a device that remains offline for the full digest TTL.
	MaxPushDigestEventsPerDevice = 128
	MaxPushDigestGroupIDs        = 64
	MaxPushDigestIdentityBytes   = 16 * 1024
	MaxPushDigestLabels          = 64
	MaxPushDigestEnqueueBatch    = 5_000
	MaxPushDigestClaimBatch      = 100
	MaxPushDigestCleanupBatch    = 500
	// PushDigestEventTTL must remain comfortably longer than the maximum
	// 24-hour digest window. Matching the window exactly makes a 24-hour digest
	// expire in ClaimDuePushDigests before it can ever become deliverable, and
	// leaves near-maximum windows no time for the 30-second worker cadence or a
	// bounded retry.
	PushDigestEventTTL = 48 * time.Hour
)

// PushDigestEnqueue is the privacy-minimised alert snapshot retained for a
// server-side iOS digest. It deliberately excludes alert titles, descriptions,
// target names, and APNs tokens.
type PushDigestEnqueue struct {
	DeviceID                 string
	ChannelID                string
	DedupeKey                string
	RuleID                   string
	Labels                   map[string]string
	Severity                 string
	NodeOffline              bool
	ServiceDown              bool
	GroupIDs                 []string
	MaintenanceScopeComplete bool
	WindowSeconds            int
	CreatedAt                time.Time
}

type PushDigestEnqueueResult struct {
	Inserted         int
	Duplicates       int
	Dropped          int
	DroppedDeviceIDs []string
}

// PushDigestEvent is a bounded event snapshot returned only to the delivery
// worker. IDs are opaque and are used to delete exactly the claimed snapshot.
type PushDigestEvent struct {
	ID                       string
	RuleID                   string
	Labels                   map[string]string
	Severity                 string
	NodeOffline              bool
	ServiceDown              bool
	GroupIDs                 []string
	MaintenanceScopeComplete bool
	CreatedAt                time.Time
	RetryCount               int
	DueAt                    time.Time
	ExpiresAt                time.Time
}

// PushDigestClaim fences one device's due digest across multiple hub
// processes. PushDevice contains the current registration/preferences and must
// be revalidated before APNs delivery.
type PushDigestClaim struct {
	LeaseExpiresAt     time.Time
	Device             PushDevice
	ChannelID          string
	WindowSeconds      int
	RetryCount         int
	DeliveryGeneration int64
	ExpiresAt          time.Time
	Events             []PushDigestEvent
}

// PushDigestSettlement partitions the exact held claim; RetryAt contains only
// entries for which this attempt reached the delivery boundary and failed.
type PushDigestSettlement struct {
	Sent, Deferred, Discarded []string
	RetryAt                   map[string]time.Time
	DeferredUntil             time.Time
}

// ValidPushDigestIdentity preserves empty labels while rejecting missing or
// oversized identity. The snapshot remains server-side, never in APNs text.
func ValidPushDigestIdentity(ruleID string, labels map[string]string) bool {
	if strings.TrimSpace(ruleID) == "" || len(ruleID) > 255 || labels == nil || len(labels) > MaxPushDigestLabels {
		return false
	}
	for key, value := range labels {
		if len(key) == 0 || len(key) > 255 || len(value) > 4096 {
			return false
		}
	}
	encoded, err := json.Marshal(labels)
	return err == nil && len(encoded) <= MaxPushDigestIdentityBytes
}

func normalizePushDigestEnqueue(event PushDigestEnqueue) (PushDigestEnqueue, error) {
	event.DeviceID = strings.TrimSpace(event.DeviceID)
	event.ChannelID = strings.TrimSpace(event.ChannelID)
	event.DedupeKey = strings.ToLower(strings.TrimSpace(event.DedupeKey))
	event.Severity = strings.ToLower(strings.TrimSpace(event.Severity))
	if !ValidPushDigestIdentity(event.RuleID, event.Labels) {
		return PushDigestEnqueue{}, fmt.Errorf("push digest alert identity missing or oversized")
	}
	labels := make(map[string]string, len(event.Labels))
	for k, v := range event.Labels {
		labels[k] = v
	}
	event.Labels = labels
	if event.DeviceID == "" || event.ChannelID == "" {
		return PushDigestEnqueue{}, fmt.Errorf("push digest device and channel are required")
	}
	if len(event.DedupeKey) != 64 || !isLowerHex(event.DedupeKey) {
		return PushDigestEnqueue{}, fmt.Errorf("push digest dedupe key must be a 64-character lowercase hex digest")
	}
	if event.Severity != "info" && event.Severity != "warning" {
		return PushDigestEnqueue{}, fmt.Errorf("push digest severity must be info or warning")
	}
	if event.WindowSeconds < 30 {
		event.WindowSeconds = 30
	}
	if event.WindowSeconds > 86_400 {
		event.WindowSeconds = 86_400
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}

	groups := make(map[string]struct{}, len(event.GroupIDs))
	for _, rawGroupID := range event.GroupIDs {
		groupID := strings.TrimSpace(rawGroupID)
		if groupID == "" || len(groupID) > 255 {
			continue
		}
		groups[groupID] = struct{}{}
	}
	event.GroupIDs = make([]string, 0, len(groups))
	for groupID := range groups {
		event.GroupIDs = append(event.GroupIDs, groupID)
	}
	sort.Strings(event.GroupIDs)
	if len(event.GroupIDs) > MaxPushDigestGroupIDs {
		event.GroupIDs = nil
		event.MaintenanceScopeComplete = false
	}
	return event, nil
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return value != ""
}
