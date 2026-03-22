package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func CollectorConfigString(config map[string]any, key string) string {
	value, ok := config[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return strings.TrimSpace(strconv.Itoa(typed))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func CollectorConfigBool(config map[string]any, key string) (bool, bool) {
	value, ok := config[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	default:
		return false, false
	}
}

func CollectorConfigDuration(config map[string]any, key string, fallback time.Duration) time.Duration {
	value, ok := config[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return fallback
		}
		if parsed, err := time.ParseDuration(trimmed); err == nil && parsed > 0 {
			return parsed
		}
		if seconds, err := strconv.Atoi(trimmed); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Second
		}
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Second
		}
	}
	return fallback
}
