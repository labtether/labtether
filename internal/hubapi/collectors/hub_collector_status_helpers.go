package collectors

import (
	"log"
	"strings"
	"time"
)

// NormalizeTrueNASStatus derives asset status from TrueNAS metadata.
func NormalizeTrueNASStatus(metadata map[string]string) string {
	status := strings.ToLower(strings.TrimSpace(metadata["status"]))
	state := strings.ToLower(strings.TrimSpace(metadata["state"]))
	combined := status + " " + state

	switch {
	case strings.Contains(combined, "faulted"), strings.Contains(combined, "unavail"),
		strings.Contains(combined, "removed"):
		return "offline"
	case strings.Contains(combined, "offline"), strings.Contains(combined, "stopped"),
		strings.Contains(combined, "degraded"):
		return "offline"
	case strings.Contains(combined, "online"), strings.Contains(combined, "running"),
		strings.Contains(combined, "healthy"), strings.Contains(combined, "active"):
		return "online"
	default:
		return "online" // TrueNAS assets discovered via API are reachable
	}
}

// NormalizePortainerStatus derives normalized heartbeat status from Portainer metadata.
func NormalizePortainerStatus(metadata map[string]string) string {
	if len(metadata) == 0 {
		return "stale"
	}

	status := strings.ToLower(strings.TrimSpace(metadata["status"]))
	state := strings.ToLower(strings.TrimSpace(metadata["state"]))
	combined := strings.TrimSpace(status + " " + state)

	switch {
	case hasNormalizedToken(combined, "up"), hasNormalizedToken(combined, "running"),
		hasNormalizedToken(combined, "active"), hasNormalizedToken(combined, "started"):
		return "online"
	case hasNormalizedToken(combined, "down"), hasNormalizedToken(combined, "stopped"),
		hasNormalizedToken(combined, "inactive"), hasNormalizedToken(combined, "exited"),
		hasNormalizedToken(combined, "dead"), hasNormalizedToken(combined, "paused"):
		return "offline"
	default:
		return "stale"
	}
}

// NormalizeDockerStatus derives normalized heartbeat status from Docker metadata.
func NormalizeDockerStatus(metadata map[string]string) string {
	if len(metadata) == 0 {
		return "online"
	}

	status := strings.ToLower(strings.TrimSpace(metadata["status"]))
	state := strings.ToLower(strings.TrimSpace(metadata["state"]))
	combined := strings.TrimSpace(status + " " + state)

	switch {
	case hasNormalizedToken(combined, "running"), hasNormalizedToken(combined, "up"),
		hasNormalizedToken(combined, "active"), hasNormalizedToken(combined, "healthy"),
		hasNormalizedToken(combined, "started"):
		return "online"
	case hasNormalizedToken(combined, "stopped"), hasNormalizedToken(combined, "exited"),
		hasNormalizedToken(combined, "dead"), hasNormalizedToken(combined, "paused"):
		return "offline"
	default:
		return "online"
	}
}

func hasNormalizedToken(text, token string) bool {
	if text == "" || token == "" {
		return false
	}
	lower := strings.ToLower(text)
	target := strings.ToLower(token)
	idx := 0
	for {
		pos := strings.Index(lower[idx:], target)
		if pos < 0 {
			return false
		}
		pos += idx // absolute position in lower
		before := pos - 1
		after := pos + len(target)
		leftOK := before < 0 || !isAlphaNum(lower[before])
		rightOK := after >= len(lower) || !isAlphaNum(lower[after])
		if leftOK && rightOK {
			return true
		}
		idx = pos + 1
	}
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

func (d *Deps) UpdateCollectorStatus(collectorID, status, errMsg string) {
	if d.HubCollectorStore == nil {
		return
	}
	if err := d.HubCollectorStore.UpdateHubCollectorStatus(collectorID, status, errMsg, time.Now().UTC()); err != nil {
		log.Printf("hub collector: failed to update status for %s: %v", collectorID, err) // #nosec G706 -- Collector IDs are bounded persisted identifiers and the error is local runtime state.
	}
}
