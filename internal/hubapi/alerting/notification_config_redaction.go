package alerting

import (
	"encoding/base64"
	"github.com/labtether/labtether/internal/notifications"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func notificationConfigKeyIsSecret(channelType string, path []string, rawKey string) bool {
	key := normalizeNotificationConfigKey(rawKey)
	if key == "" {
		return false
	}
	switch notifications.NormalizeChannelType(channelType) {
	case notifications.ChannelTypeWebhook:
		if len(path) == 0 && (key == "url" || key == "secret" || key == "headers") {
			return true
		}
	case notifications.ChannelTypeSlack:
		if len(path) == 0 && key == "webhook_url" {
			return true
		}
	case notifications.ChannelTypeEmail:
		if len(path) == 0 && key == "smtp_pass" {
			return true
		}
	case notifications.ChannelTypeAPNs:
		if len(path) == 0 && (key == "auth_key_path" || key == "device_tokens") {
			return true
		}
	case notifications.ChannelTypeNtfy:
		if len(path) == 0 && (key == "token" || key == "password") {
			return true
		}
	case notifications.ChannelTypeGotify:
		if len(path) == 0 && (key == "app_token" || key == "token") {
			return true
		}
	}

	switch key {
	case "pass", "passwd", "password", "secret", "token", "api_key", "apikey", "authorization", "auth_header", "private_key", "client_secret", "access_token", "refresh_token", "bearer_token":
		return true
	}
	for _, suffix := range []string{"_password", "_passwd", "_secret", "_token", "_api_key", "_private_key"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func normalizeNotificationConfigKey(value string) string {
	runes := []rune(strings.TrimSpace(value))
	var b strings.Builder
	lastWasSeparator := false
	for i, r := range runes {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			if unicode.IsUpper(r) && b.Len() > 0 && !lastWasSeparator {
				previous := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextIsLower) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			lastWasSeparator = false
		default:
			if b.Len() > 0 && !lastWasSeparator {
				b.WriteByte('_')
				lastWasSeparator = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func redactNotificationChannel(channel notifications.Channel) notifications.Channel {
	channel.Config = redactNotificationConfigValue(channel.Type, nil, channel.Config, false).(map[string]any)
	return channel
}

func redactNotificationConfigValue(channelType string, path []string, value any, inheritedSecret bool) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key) || notificationValueLooksEncrypted(child) {
				continue
			}
			out[key] = redactNotificationConfigValue(channelType, appendNotificationPath(path, key), child, false)
		}
		return out
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, child := range typed {
			converted[key] = child
		}
		return redactNotificationConfigValue(channelType, path, converted, inheritedSecret)
	case []any:
		out := make([]any, 0, len(typed))
		for i, child := range typed {
			if notificationValueLooksEncrypted(child) {
				continue
			}
			out = append(out, redactNotificationConfigValue(channelType, appendNotificationPath(path, strconv.Itoa(i)), child, inheritedSecret))
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, child := range typed {
			if !notificationValueLooksEncrypted(child) {
				out = append(out, child)
			}
		}
		return out
	default:
		return typed
	}
}

func notificationValueLooksEncrypted(value any) bool {
	text, ok := value.(string)
	return ok && strings.HasPrefix(strings.TrimSpace(text), "v2:")
}

func mergePreservedNotificationSecrets(channelType string, existing, incoming map[string]any) map[string]any {
	return mergeNotificationConfigMaps(channelType, nil, existing, incoming)
}

func mergeNotificationConfigMaps(channelType string, path []string, existing, incoming map[string]any) map[string]any {
	result := deepCloneNotificationMap(incoming)
	for key, existingValue := range existing {
		incomingValue, present := incoming[key]
		if notificationConfigKeyIsSecret(channelType, path, key) {
			if !present || notificationSecretPlaceholder(incomingValue) {
				result[key] = deepCloneNotificationValue(existingValue)
			}
			continue
		}
		existingMap, existingIsMap := notificationAnyMap(existingValue)
		incomingMap, incomingIsMap := notificationAnyMap(incomingValue)
		if present && existingIsMap && incomingIsMap {
			result[key] = mergeNotificationConfigMaps(channelType, appendNotificationPath(path, key), existingMap, incomingMap)
		}
	}
	return result
}

func notificationSecretPlaceholder(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(strings.ToUpper(text)) {
	case "", strings.ToUpper(notificationRedactedValue), "********", "••••••••":
		return true
	default:
		return false
	}
}

func notificationAnyMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = child
		}
		return out, true
	default:
		return nil, false
	}
}

func deepCloneNotificationMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = deepCloneNotificationValue(value)
	}
	return out
}

func deepCloneNotificationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCloneNotificationMap(typed)
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, child := range typed {
			out[key] = child
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = deepCloneNotificationValue(child)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func sanitizeNotificationDeliveryError(channel notifications.Channel, err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	secrets := collectNotificationSecretStrings(channel.Type, nil, channel.Config, false)
	if notifications.NormalizeChannelType(channel.Type) == notifications.ChannelTypeEmail {
		user, _ := channel.Config["smtp_user"].(string)
		password, _ := channel.Config["smtp_pass"].(string)
		if user != "" && password != "" {
			secrets = append(secrets, base64.StdEncoding.EncodeToString([]byte("\x00"+user+"\x00"+password)))
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	seen := make(map[string]struct{}, len(secrets)*7)
	for _, secret := range secrets {
		secretBytes := []byte(secret)
		for _, candidate := range []string{
			secret,
			url.QueryEscape(secret),
			url.PathEscape(secret),
			base64.StdEncoding.EncodeToString(secretBytes),
			base64.RawStdEncoding.EncodeToString(secretBytes),
			base64.URLEncoding.EncodeToString(secretBytes),
			base64.RawURLEncoding.EncodeToString(secretBytes),
		} {
			if candidate == "" {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			message = strings.ReplaceAll(message, candidate, "[redacted]")
		}
	}
	return notifications.SanitizeDeliveryErrorMessage(message)
}

func collectNotificationSecretStrings(channelType string, path []string, value any, inheritedSecret bool) []string {
	var out []string
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			secret := inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key)
			out = append(out, collectNotificationSecretStrings(channelType, appendNotificationPath(path, key), child, secret)...)
		}
	case map[string]string:
		for key, child := range typed {
			secret := inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key)
			out = append(out, collectNotificationSecretStrings(channelType, appendNotificationPath(path, key), child, secret)...)
		}
	case []any:
		for i, child := range typed {
			out = append(out, collectNotificationSecretStrings(channelType, appendNotificationPath(path, strconv.Itoa(i)), child, inheritedSecret)...)
		}
	case []string:
		if inheritedSecret {
			for _, child := range typed {
				if child != "" {
					out = append(out, child)
				}
			}
		}
	case string:
		if inheritedSecret && typed != "" {
			out = append(out, typed)
		}
	}
	return out
}
