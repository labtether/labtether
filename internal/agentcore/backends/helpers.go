package backends

import (
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	// MaxCommandOutputBytes is the maximum bytes of command output to preserve.
	MaxCommandOutputBytes = 64 * 1024
	// PackageActionCommandTimeout is the timeout for package action commands.
	PackageActionCommandTimeout = 10 * time.Minute
)

// TruncateCommandOutput returns the trimmed string representation of payload,
// truncated to maxBytes if necessary.
func TruncateCommandOutput(payload []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 8 * 1024
	}
	if len(payload) <= maxBytes {
		return strings.TrimSpace(string(payload))
	}
	return strings.TrimSpace(string(payload[:maxBytes])) + "\n...output truncated"
}

// EntryMatchesQuery checks if a log entry matches a journal query's filters.
// Used by both the darwin unified-log backend and the Windows Event Log backend.
func EntryMatchesQuery(entry agentmgr.LogStreamData, req agentmgr.JournalQueryData) bool {
	if !levelMatchesPriority(entry.Level, req.Priority) {
		return false
	}

	if unit := normalizeUnitFilter(req.Unit); unit != "" {
		source := strings.ToLower(strings.TrimSpace(entry.Source))
		if source == "" || (source != unit && !strings.Contains(source, unit)) {
			return false
		}
	}

	if search := strings.ToLower(strings.TrimSpace(req.Search)); search != "" {
		haystack := strings.ToLower(entry.Message + " " + entry.Source)
		if !strings.Contains(haystack, search) {
			return false
		}
	}

	return true
}

func normalizeUnitFilter(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimSuffix(value, ".service")
	return value
}

func levelMatchesPriority(level string, priority string) bool {
	normalizedPriority := strings.ToLower(strings.TrimSpace(priority))
	if normalizedPriority == "" || normalizedPriority == "all" {
		return true
	}

	maxRank, ok := priorityMaxRank(normalizedPriority)
	if !ok {
		return true
	}
	return logLevelRank(level) <= maxRank
}

func priorityMaxRank(priority string) (int, bool) {
	switch priority {
	case "emerg", "alert", "crit", "err", "error":
		return 3, true
	case "warning", "warn":
		return 4, true
	case "notice":
		return 5, true
	case "info":
		return 6, true
	case "debug":
		return 7, true
	default:
		return 0, false
	}
}

func logLevelRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error", "err", "critical", "crit", "alert", "emerg":
		return 3
	case "warning", "warn":
		return 4
	case "notice":
		return 5
	case "debug":
		return 7
	default:
		return 6
	}
}
