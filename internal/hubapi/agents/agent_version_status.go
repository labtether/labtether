package agents

import (
	"strconv"
	"strings"
)

// DetermineAgentVersionStatus says whether an agent needs the Hub's agent
// release. Release versions compare by number, so v2.0.0 is newer than
// v1.7.1 and v1.10.0 is newer than v1.9.0, with or without the leading "v".
// An agent at or above the Hub's release is up to date: the Hub must not offer
// an older release to an agent that was updated first. Other versions, such as
// development builds and release candidates, compare as text.
func DetermineAgentVersionStatus(currentVersion, latestVersion string) string {
	current := strings.TrimSpace(currentVersion)
	latest := strings.TrimSpace(latestVersion)
	if current == "" || latest == "" {
		return "unknown"
	}
	if currentParts, ok := parseAgentReleaseVersion(current); ok {
		if latestParts, ok := parseAgentReleaseVersion(latest); ok {
			if compareAgentReleaseVersions(currentParts, latestParts) >= 0 {
				return "up_to_date"
			}
			return "update_available"
		}
	}
	if strings.EqualFold(current, latest) {
		return "up_to_date"
	}
	return "update_available"
}

// parseAgentReleaseVersion reads "v2.0.0", "2.0.0" or "v2026.1" as numbers.
// It refuses anything with a suffix such as "-rc1" or "-dirty".
func parseAgentReleaseVersion(raw string) ([3]uint64, bool) {
	var parsed [3]uint64
	value := raw
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > len(parsed) {
		return parsed, false
	}
	for index, part := range parts {
		if part == "" || strings.TrimLeft(part, "0123456789") != "" {
			return parsed, false
		}
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return parsed, false
		}
		parsed[index] = number
	}
	return parsed, true
}

func compareAgentReleaseVersions(left, right [3]uint64) int {
	for index := range left {
		switch {
		case left[index] < right[index]:
			return -1
		case left[index] > right[index]:
			return 1
		}
	}
	return 0
}
