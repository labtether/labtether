package proxmox

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func vmidString(value float64) string {
	parsed, ok := finiteIntegralInt64(value)
	if !ok || parsed <= 0 {
		return ""
	}
	return strconv.FormatInt(parsed, 10)
}

func normalizeID(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	trimmed = strings.ReplaceAll(trimmed, "/", "-")
	return trimmed
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return finiteNumberString(typed)
	case int:
		return strconv.Itoa(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func finiteNumberString(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	if parsed, ok := finiteIntegralInt64(value); ok {
		return strconv.FormatInt(parsed, 10)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func finiteIntegralInt64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < minInt64AsFloat || value >= maxInt64ExclusiveAsFloat {
		return 0, false
	}
	return int64(value), true
}

const (
	minInt64AsFloat          = -9223372036854775808.0
	maxInt64ExclusiveAsFloat = 9223372036854775808.0
)
