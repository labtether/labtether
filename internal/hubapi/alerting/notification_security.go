package alerting

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/notifications"
)

var errNotificationSecretsUnavailable = errors.New("notification secret encryption is unavailable")

const notificationRedactedValue = "[REDACTED]"

// notificationConfigCASStore is implemented by the Postgres store. It keeps
// lazy legacy migration from overwriting a concurrent channel edit.
type notificationConfigCASStore interface {
	CompareAndSwapNotificationChannelConfig(id string, expected, replacement map[string]any) (bool, error)
}

func (d *Deps) createSecureNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	channelID := idgen.New("nch")
	channelType := notifications.NormalizeChannelType(req.Type)
	if err := ValidateNotificationChannelConfig(channelType, req.Config); err != nil {
		return notifications.Channel{}, invalidNotificationChannel(err)
	}
	storedConfig, err := d.encryptNotificationConfig(channelID, channelType, req.Config)
	if err != nil {
		return notifications.Channel{}, err
	}
	req.ID = channelID
	req.Type = channelType
	req.Config = storedConfig
	created, err := d.NotificationStore.CreateNotificationChannel(req)
	if err != nil {
		return notifications.Channel{}, err
	}
	return redactNotificationChannel(created), nil
}

func (d *Deps) updateSecureNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	raw, ok, err := d.NotificationStore.GetNotificationChannel(id)
	if err != nil {
		return notifications.Channel{}, err
	}
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}

	needsMigration := notificationConfigNeedsMigration(raw.Type, nil, raw.Config, false)
	if req.Config != nil || needsMigration {
		plainConfig, legacyPlaintext, decryptErr := d.decryptNotificationConfig(raw.ID, raw.Type, raw.Config)
		if decryptErr != nil {
			return notifications.Channel{}, decryptErr
		}
		if req.Config != nil {
			merged := mergePreservedNotificationSecrets(raw.Type, plainConfig, *req.Config)
			if validationErr := ValidateNotificationChannelConfig(raw.Type, merged); validationErr != nil {
				return notifications.Channel{}, invalidNotificationChannel(validationErr)
			}
			storedConfig, encryptErr := d.encryptNotificationConfig(raw.ID, raw.Type, merged)
			if encryptErr != nil {
				return notifications.Channel{}, encryptErr
			}
			req.Config = &storedConfig
		} else if legacyPlaintext {
			// A non-config edit is also an opportunity to finish migrating a legacy
			// plaintext row without an extra write before the requested update.
			storedConfig, encryptErr := d.encryptNotificationConfig(raw.ID, raw.Type, plainConfig)
			if encryptErr != nil {
				return notifications.Channel{}, encryptErr
			}
			req.Config = &storedConfig
		}
	}

	updated, err := d.NotificationStore.UpdateNotificationChannel(id, req)
	if err != nil {
		return notifications.Channel{}, err
	}
	return redactNotificationChannel(updated), nil
}

func (d *Deps) listNotificationChannelsForAPI(limit int) ([]notifications.Channel, error) {
	channels, err := d.NotificationStore.ListNotificationChannels(limit)
	if err != nil {
		return nil, err
	}
	result := make([]notifications.Channel, 0, len(channels))
	for _, channel := range channels {
		if d.NotificationSecrets != nil && notificationConfigNeedsMigration(channel.Type, nil, channel.Config, false) {
			migrated, err := d.notificationChannelForRuntime(channel)
			if err != nil {
				return nil, err
			}
			channel = migrated
		}
		result = append(result, redactNotificationChannel(channel))
	}
	return result, nil
}

func (d *Deps) getNotificationChannelForAPI(id string) (notifications.Channel, bool, error) {
	channel, ok, err := d.NotificationStore.GetNotificationChannel(id)
	if err != nil || !ok {
		return notifications.Channel{}, ok, err
	}
	if d.NotificationSecrets != nil && notificationConfigNeedsMigration(channel.Type, nil, channel.Config, false) {
		migrated, err := d.notificationChannelForRuntime(channel)
		if err != nil {
			return notifications.Channel{}, false, err
		}
		channel = migrated
	}
	return redactNotificationChannel(channel), true, nil
}

func (d *Deps) getNotificationChannelForRuntime(id string) (notifications.Channel, bool, error) {
	channel, ok, err := d.NotificationStore.GetNotificationChannel(id)
	if err != nil || !ok {
		return notifications.Channel{}, ok, err
	}
	plain, err := d.notificationChannelForRuntime(channel)
	if err != nil {
		return notifications.Channel{}, false, err
	}
	return plain, true, nil
}

func (d *Deps) notificationChannelForRuntime(raw notifications.Channel) (notifications.Channel, error) {
	plainConfig, legacyPlaintext, err := d.decryptNotificationConfig(raw.ID, raw.Type, raw.Config)
	if err != nil {
		return notifications.Channel{}, err
	}
	if legacyPlaintext {
		replacement, encryptErr := d.encryptNotificationConfig(raw.ID, raw.Type, plainConfig)
		if encryptErr != nil {
			return notifications.Channel{}, encryptErr
		}
		if migrator, ok := d.NotificationStore.(notificationConfigCASStore); ok {
			swapped, migrateErr := migrator.CompareAndSwapNotificationChannelConfig(raw.ID, raw.Config, replacement)
			if migrateErr != nil {
				return notifications.Channel{}, fmt.Errorf("migrate notification channel config: %w", migrateErr)
			}
			if !swapped {
				// A concurrent edit won the race. Never dispatch the stale config that
				// lost the compare-and-swap; reload and use the row that is current now.
				latest, ok, loadErr := d.NotificationStore.GetNotificationChannel(raw.ID)
				if loadErr != nil {
					return notifications.Channel{}, fmt.Errorf("reload notification channel after migration race: %w", loadErr)
				}
				if !ok {
					return notifications.Channel{}, notifications.ErrChannelNotFound
				}
				latestConfig, _, decryptErr := d.decryptNotificationConfig(latest.ID, latest.Type, latest.Config)
				if decryptErr != nil {
					return notifications.Channel{}, decryptErr
				}
				latest.Config = latestConfig
				return latest, nil
			}
		} else {
			// Non-Postgres stores are used by tests and reduced runtimes. Production
			// uses the compare-and-swap path above.
			if _, migrateErr := d.NotificationStore.UpdateNotificationChannel(raw.ID, notifications.UpdateChannelRequest{Config: &replacement}); migrateErr != nil {
				return notifications.Channel{}, fmt.Errorf("migrate notification channel config: %w", migrateErr)
			}
		}
	}
	raw.Config = plainConfig
	return raw, nil
}

func notificationConfigNeedsMigration(channelType string, path []string, value any, inheritedSecret bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			secret := inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key)
			if notificationConfigNeedsMigration(channelType, appendNotificationPath(path, key), child, secret) {
				return true
			}
		}
	case map[string]string:
		for key, child := range typed {
			secret := inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key)
			if notificationConfigNeedsMigration(channelType, appendNotificationPath(path, key), child, secret) {
				return true
			}
		}
	case []any:
		for i, child := range typed {
			if notificationConfigNeedsMigration(channelType, appendNotificationPath(path, strconv.Itoa(i)), child, inheritedSecret) {
				return true
			}
		}
	case []string:
		for i, child := range typed {
			if notificationConfigNeedsMigration(channelType, appendNotificationPath(path, strconv.Itoa(i)), child, inheritedSecret) {
				return true
			}
		}
	case string:
		return inheritedSecret && typed != "" && !strings.HasPrefix(strings.TrimSpace(typed), "v2:")
	case nil:
		return false
	default:
		// Force the migration/decryption path to validate malformed values in a
		// secret container rather than silently treating them as safe.
		return inheritedSecret
	}
	return false
}

func (d *Deps) encryptNotificationConfig(channelID, channelType string, config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}
	value, _, err := d.transformNotificationConfigValue(channelID, channelType, nil, config, false, false)
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("notification config must be an object")
	}
	return result, nil
}

func (d *Deps) decryptNotificationConfig(channelID, channelType string, config map[string]any) (map[string]any, bool, error) {
	if config == nil {
		return nil, false, nil
	}
	value, migrated, err := d.transformNotificationConfigValue(channelID, channelType, nil, config, false, true)
	if err != nil {
		return nil, false, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, false, errors.New("notification config must be an object")
	}
	return result, migrated, nil
}

func (d *Deps) transformNotificationConfigValue(channelID, channelType string, path []string, value any, inheritedSecret, decrypt bool) (any, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		migrated := false
		for key, child := range typed {
			childPath := appendNotificationPath(path, key)
			secret := inheritedSecret || notificationConfigKeyIsSecret(channelType, path, key)
			transformed, childMigrated, err := d.transformNotificationConfigValue(channelID, channelType, childPath, child, secret, decrypt)
			if err != nil {
				return nil, false, err
			}
			out[key] = transformed
			migrated = migrated || childMigrated
		}
		return out, migrated, nil
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, child := range typed {
			converted[key] = child
		}
		return d.transformNotificationConfigValue(channelID, channelType, path, converted, inheritedSecret, decrypt)
	case []any:
		out := make([]any, len(typed))
		migrated := false
		for i, child := range typed {
			childPath := appendNotificationPath(path, strconv.Itoa(i))
			transformed, childMigrated, err := d.transformNotificationConfigValue(channelID, channelType, childPath, child, inheritedSecret, decrypt)
			if err != nil {
				return nil, false, err
			}
			out[i] = transformed
			migrated = migrated || childMigrated
		}
		return out, migrated, nil
	case []string:
		out := make([]string, len(typed))
		migrated := false
		for i, child := range typed {
			childPath := appendNotificationPath(path, strconv.Itoa(i))
			transformed, childMigrated, err := d.transformNotificationConfigValue(channelID, channelType, childPath, child, inheritedSecret, decrypt)
			if err != nil {
				return nil, false, err
			}
			out[i] = transformed.(string)
			migrated = migrated || childMigrated
		}
		return out, migrated, nil
	case string:
		if !inheritedSecret || typed == "" {
			return typed, false, nil
		}
		aad := notificationChannelSecretAAD(channelID, path)
		if decrypt {
			if !strings.HasPrefix(strings.TrimSpace(typed), "v2:") {
				if d.NotificationSecrets == nil {
					return nil, false, errNotificationSecretsUnavailable
				}
				return typed, true, nil
			}
			if d.NotificationSecrets == nil {
				return nil, false, errNotificationSecretsUnavailable
			}
			plain, err := d.NotificationSecrets.DecryptString(typed, aad)
			if err != nil {
				return nil, false, fmt.Errorf("decrypt notification channel secret: %w", err)
			}
			return plain, false, nil
		}
		if d.NotificationSecrets == nil {
			return nil, false, errNotificationSecretsUnavailable
		}
		ciphertext, err := d.NotificationSecrets.EncryptString(typed, aad)
		if err != nil {
			return nil, false, fmt.Errorf("encrypt notification channel secret: %w", err)
		}
		return ciphertext, false, nil
	case nil:
		return nil, false, nil
	default:
		if inheritedSecret {
			return nil, false, errors.New("notification secret fields must contain strings, arrays, or objects")
		}
		return typed, false, nil
	}
}

func notificationChannelSecretAAD(channelID string, path []string) string {
	encodedPath, _ := json.Marshal(path)
	return "notification-channel:" + strings.TrimSpace(channelID) + ":" + string(encodedPath)
}

func appendNotificationPath(path []string, segment string) []string {
	result := make([]string, len(path)+1)
	copy(result, path)
	result[len(path)] = segment
	return result
}

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
