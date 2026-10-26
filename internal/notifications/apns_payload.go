package notifications

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Alert payload and header helpers shared by direct APNs delivery and the
// LabTether push relay. Values are bounded before they leave the Hub.

// buildAPNsPayload constructs the APNs JSON payload from the notification payload.
func buildAPNsPayload(payload map[string]any) ([]byte, error) {
	title := boundedUTF8(payloadString(payload, "title"), 160)
	text := boundedUTF8(payloadString(payload, "text"), 1_024)
	if title == "" {
		title = "LabTether Alert"
	}

	aps := map[string]any{
		"alert": map[string]any{
			"title": title,
			"body":  text,
		},
		"sound": "default",
	}

	alertID := firstNonBlank(
		payloadString(payload, "alert_id"),
		payloadString(payload, "alert_instance_id"),
	)
	apns := map[string]any{"aps": aps, "title": title}
	if alertID != "" {
		apns["alert_id"] = boundedUTF8(alertID, 256)
	}

	category := payloadString(payload, "apns_category")
	if category != "" {
		aps["category"] = boundedUTF8(category, 128)
	}

	for key, maxBytes := range map[string]int{
		"incident_id": 256,
		"deep_link":   512,
		"runbook_id":  256,
		"severity":    64,
		"status":      64,
		"summary":     1_024,
		"event":       128,
		"rule_id":     256,
	} {
		if value := payloadString(payload, key); value != "" {
			apns[key] = boundedUTF8(value, maxBytes)
		}
	}
	if digestCount := payloadPositiveInt(payload, "digest_count"); digestCount > 0 {
		// Keep the count as a number so the iOS client can render a localised
		// digest summary without parsing user-visible text.
		apns["digest_count"] = digestCount
	}

	encoded, err := json.Marshal(apns)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 4*1024 {
		return nil, fmt.Errorf("payload exceeds APNs 4 KB limit")
	}
	return encoded, nil
}

func payloadPositiveInt(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	var value int64
	switch typed := payload[key].(type) {
	case int:
		value = int64(typed)
	case int32:
		value = int64(typed)
	case int64:
		value = typed
	case float64:
		if typed <= 0 || typed > 10_000 {
			if typed > 10_000 {
				return 10_000
			}
			return 0
		}
		value = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		value = parsed
	default:
		return 0
	}
	if value <= 0 {
		return 0
	}
	if value > 10_000 {
		return 10_000
	}
	return value
}

func payloadUnixTime(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	var value int64
	switch typed := payload[key].(type) {
	case int:
		value = int64(typed)
	case int64:
		value = typed
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		value = parsed
	default:
		return 0
	}
	// 9999-12-31T23:59:59Z is a conservative upper bound for a Unix
	// timestamp accepted as an APNs expiry.
	if value <= 0 || value > 253_402_300_799 {
		return 0
	}
	return value
}

func boundedHeaderValue(value string, maxBytes int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	return boundedUTF8(value, maxBytes)
}

func boundedUTF8(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// extractDeviceTokens reads device tokens from the config map.
// Supports both []string and []any (as JSON-decoded arrays typically arrive as []any).
func extractDeviceTokens(config map[string]any) []string {
	if tokens, ok := config["device_tokens"].([]string); ok {
		return tokens
	}
	raw, ok := config["device_tokens"].([]any)
	if !ok {
		return nil
	}
	tokens := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			tokens = append(tokens, s)
		}
	}
	return tokens
}

// truncateToken returns the first 8 characters of a device token for safe logging.
func truncateToken(token string) string {
	if len(token) > 8 {
		return token[:8]
	}
	return token
}

// apnsAlertPriority returns the caller's APNs priority (5 or 10), default 10.
func apnsAlertPriority(payload map[string]any) int64 {
	if configured := payloadPositiveInt(payload, "apns_priority"); configured == 5 || configured == 10 {
		return configured
	}
	return 10
}

// apnsAlertCollapseID prefers a caller-supplied privacy-safe collapse id (for
// example a digest), then the alert or incident identifier.
func apnsAlertCollapseID(payload map[string]any) string {
	return firstNonBlank(
		payloadString(payload, "collapse_id"),
		payloadString(payload, "alert_id"),
		payloadString(payload, "incident_id"),
	)
}

func apnsPriorityHeader(priority int64) string {
	return strconv.FormatInt(priority, 10)
}
