package assets

import (
	"strconv"
	"strings"
	"time"
)

// MetadataKeyCollectorInterval carries the sync interval, in seconds, of the
// connection (Docker, Proxmox, PBS, TrueNAS, Portainer, Home Assistant) that
// stores a device. The connection refreshes the device once per interval.
const MetadataKeyCollectorInterval = "collector_interval_seconds"

// Agent check-in limits: an agent device is online under 65 s and stale
// (not responding) under 5 minutes.
const (
	agentOnlineSeconds  = 65.0
	agentOfflineSeconds = 300.0
)

// CollectorIntervalSeconds returns the sync interval stamped on a device by
// its connection, or 0 when the device has none (agent devices).
func CollectorIntervalSeconds(metadata map[string]string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(metadata[MetadataKeyCollectorInterval]))
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

// Freshness returns "online", "stale" or "offline" from a device's last
// check-in. Agent devices use the agent limits. A connection refreshes its
// devices once per its interval, so such a device stays online under
// 1.5 x interval + 15 s and goes offline after 5 x interval, never tighter
// than the agent limits. The console's assetFreshnessLabel
// (web/console/app/console/formatters.ts) uses the same limits.
func Freshness(lastSeenAt, now time.Time, metadata map[string]string) string {
	if lastSeenAt.IsZero() {
		return "offline"
	}
	age := now.Sub(lastSeenAt.UTC())
	if age < 0 {
		return "offline"
	}
	onlineSeconds, offlineSeconds := agentOnlineSeconds, agentOfflineSeconds
	if interval := float64(CollectorIntervalSeconds(metadata)); interval > 0 {
		onlineSeconds = max(onlineSeconds, 1.5*interval+15)
		offlineSeconds = max(offlineSeconds, 5*interval)
	}
	switch ageSeconds := age.Seconds(); {
	case ageSeconds < onlineSeconds:
		return "online"
	case ageSeconds < offlineSeconds:
		return "stale"
	default:
		return "offline"
	}
}
