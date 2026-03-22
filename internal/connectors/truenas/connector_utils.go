package truenas

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", value)
	}
}

// anyToBool converts an arbitrary value to a bool.
// TrueNAS wraps many property values in a map with "parsed"/"value"/"rawvalue"
// keys; this function handles that nesting automatically.
// The second return value is false when the conversion is not possible.
func anyToBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		switch trimmed {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case map[string]any:
		for _, key := range []string{"parsed", "value", "rawvalue"} {
			if nested, ok := typed[key]; ok {
				if parsed, ok := anyToBool(nested); ok {
					return parsed, true
				}
			}
		}
	}
	return false, false
}

// anyToFloat converts an arbitrary JSON-decoded value to float64.
// TrueNAS nested value maps ({"value":"...","rawvalue":"..."}) are unwrapped
// automatically. Returns 0 when the conversion is not possible.
func anyToFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return f
	case map[string]any:
		for _, key := range []string{"rawvalue", "parsed", "value"} {
			if nested, ok := typed[key]; ok && nested != nil {
				return anyToFloat(nested)
			}
		}
	}
	return 0
}

// formatFloat converts a float64 byte count to its integer string representation.
// Zero values are returned as "0".
func formatFloat(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatInt(int64(value), 10)
}

// normalizeID converts a raw name or path into a URL/key-safe identifier by
// lower-casing it and replacing separators with distinct substitutes to avoid
// collisions (e.g. "tank/data" vs "tank-data" stay distinct).
func normalizeID(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return "unknown"
	}
	// Use double-dash for path separators so "tank/data" → "tank--data"
	// which is distinct from "tank-data" → "tank-data".
	replacer := strings.NewReplacer("/", "--", "\\", "--", " ", "_", ":", "-")
	return replacer.Replace(trimmed)
}

// nestedValue traverses a map[string]any following the given chain of keys,
// unwrapping TrueNAS-style {"value":"...","rawvalue":"..."} wrappers at each
// level if the final value is itself a map.
func nestedValue(m map[string]any, keys ...string) any {
	var current any = m
	for _, key := range keys {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = cm[key]
	}
	// Unwrap TrueNAS nested value maps.
	if nested, ok := current.(map[string]any); ok {
		for _, key := range []string{"value", "rawvalue", "parsed"} {
			if v, exists := nested[key]; exists {
				return v
			}
		}
	}
	return current
}

// paramOrTarget returns the value of paramKey from req.Params, falling back
// to req.TargetID when the param is absent or empty.
func paramOrTarget(req connectorsdk.ActionRequest, paramKey string) string {
	if v := strings.TrimSpace(req.Params[paramKey]); v != "" {
		return v
	}
	return strings.TrimSpace(req.TargetID)
}

// parseFloat converts a string to float64, returning 0 on failure.
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// clampPercent restricts a percentage value to the [0, 100] range.
func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
