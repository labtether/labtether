package shared

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CollectorAnyString converts an arbitrary value to a trimmed string.
func CollectorAnyString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return strings.TrimSpace(strconv.Itoa(typed))
	case int64:
		return strings.TrimSpace(strconv.FormatInt(typed, 10))
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

// CollectorAnyTime converts an arbitrary value to a time.Time.
func CollectorAnyTime(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case float64:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC()
		}
	case int64:
		if typed > 0 {
			return time.Unix(typed, 0).UTC()
		}
	case int:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC()
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			break
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.UTC()
		}
		if unix, err := strconv.ParseFloat(trimmed, 64); err == nil && unix > 0 {
			return time.Unix(int64(unix), 0).UTC()
		}
	}
	return time.Now().UTC()
}

// NormalizeAssetKey normalizes a string for use as an asset key.
func NormalizeAssetKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(value)
}

// NormalizeCollectorLogLevel maps a freeform level string to info/warn/error.
func NormalizeCollectorLogLevel(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "crit"), strings.Contains(lower, "err"), strings.Contains(lower, "fail"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	default:
		return "info"
	}
}

// StableConnectorLogID builds a deterministic log event ID from a prefix and key.
func StableConnectorLogID(prefix, key string) string {
	trimmedPrefix := strings.TrimSpace(prefix)
	trimmedKey := strings.TrimSpace(key)
	if trimmedPrefix == "" || trimmedKey == "" {
		return ""
	}

	encoded := base64.RawURLEncoding.EncodeToString([]byte(trimmedKey))
	if len(encoded) > 120 {
		encoded = encoded[:120]
	}
	return trimmedPrefix + "_" + encoded
}

// ParsePositiveInt parses a string as a positive integer.
func ParsePositiveInt(raw string) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// ParseAnyInt64 attempts to parse an arbitrary value as int64.
func ParseAnyInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		return n, err == nil
	}
	return 0, false
}

// FormatStorageInsightsWindow formats a duration as a human-readable window string.
func FormatStorageInsightsWindow(window time.Duration) string {
	if window <= 0 {
		return "24h"
	}
	return window.String()
}

// TrueNASAlertMessage extracts a user-friendly message from a TrueNAS alert.
func TrueNASAlertMessage(alert map[string]any) string {
	if formatted := strings.TrimSpace(CollectorAnyString(alert["formatted"])); formatted != "" {
		return formatted
	}
	if text := strings.TrimSpace(CollectorAnyString(alert["text"])); text != "" {
		return text
	}

	klass := strings.TrimSpace(CollectorAnyString(alert["klass"]))
	if klass == "" {
		return "truenas alert"
	}
	return "truenas alert: " + klass
}

// AnyToFloat64 converts an arbitrary value to float64.
func AnyToFloat64(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

// ParseAnyBoolLoose parses an arbitrary value as a boolean, accepting many formats.
func ParseAnyBoolLoose(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case float64:
		return typed != 0, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
		lower := strings.ToLower(strings.TrimSpace(typed))
		switch lower {
		case "yes", "y", "on", "enabled":
			return true, true
		case "no", "n", "off", "disabled":
			return false, true
		}
	}
	return false, false
}

// ParseAnyTimestamp parses an arbitrary value as a timestamp.
func ParseAnyTimestamp(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC(), true
	case int64:
		if typed > 0 {
			return time.Unix(typed, 0).UTC(), true
		}
	case int:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC(), true
		}
	case float64:
		if typed > 0 {
			return time.Unix(int64(typed), 0).UTC(), true
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return parsed.UTC(), true
		}
		if unix, err := strconv.ParseFloat(trimmed, 64); err == nil && unix > 0 {
			return time.Unix(int64(unix), 0).UTC(), true
		}
	}
	return time.Time{}, false
}
