package alerting

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/notifications"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxNotificationConfigBytes      = 64 << 10
	maxNotificationConfigDepth      = 8
	maxNotificationConfigItems      = 256
	maxNotificationConfigStringLen  = 16 << 10
	maxNotificationURLLength        = 4096
	maxNotificationRecipientCount   = 100
	maxNotificationRecipientListLen = 2048
)

type notificationChannelValidationError struct {
	err error
}

func (e *notificationChannelValidationError) Error() string {
	if e == nil || e.err == nil {
		return "invalid notification channel"
	}
	return e.err.Error()
}

func invalidNotificationChannel(err error) error {
	if err == nil {
		return nil
	}
	return &notificationChannelValidationError{err: err}
}

func ValidateCreateChannelRequest(req notifications.CreateChannelRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateMaxLen("name", req.Name, MaxAlertRuleNameLength); err != nil {
		return err
	}
	if notifications.NormalizeChannelType(req.Type) == "" {
		return errors.New("type must be webhook, email, slack, apns, ntfy, or gotify")
	}
	return ValidateNotificationChannelConfig(req.Type, req.Config)
}

func ValidateUpdateChannelRequest(req notifications.UpdateChannelRequest) error {
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return errors.New("name cannot be empty")
		}
		if err := validateMaxLen("name", *req.Name, MaxAlertRuleNameLength); err != nil {
			return err
		}
	}
	if req.Config != nil {
		return validateNotificationConfigBounds(*req.Config)
	}
	return nil
}

// ValidateNotificationChannelConfig rejects a channel that cannot be safely
// dispatched. The outbound runtime still revalidates DNS, network class and
// redirects at send time; this layer keeps malformed or oversized durable
// configuration from entering the database in the first place.
func ValidateNotificationChannelConfig(channelType string, config map[string]any) error {
	channelType = notifications.NormalizeChannelType(channelType)
	if channelType == "" {
		return errors.New("unsupported notification channel type")
	}
	if err := validateNotificationConfigBounds(config); err != nil {
		return err
	}

	switch channelType {
	case notifications.ChannelTypeWebhook:
		if err := validateNotificationEndpoint(config, "url", false); err != nil {
			return err
		}
		if err := validateWebhookHeaders(config["headers"]); err != nil {
			return err
		}
	case notifications.ChannelTypeSlack:
		endpointConfig := config
		if configString(config, "webhook_url") == "" && configString(config, "webhookUrl") != "" {
			endpointConfig = cloneAnyMap(config)
			endpointConfig["webhook_url"] = configString(config, "webhookUrl")
		}
		if err := validateNotificationEndpoint(endpointConfig, "webhook_url", false); err != nil {
			return err
		}
	case notifications.ChannelTypeEmail:
		if err := validateEmailNotificationConfig(config); err != nil {
			return err
		}
	case notifications.ChannelTypeAPNs:
		if err := validateAPNsNotificationConfig(config); err != nil {
			return err
		}
	case notifications.ChannelTypeNtfy:
		if err := validateNtfyNotificationConfig(config); err != nil {
			return err
		}
	case notifications.ChannelTypeGotify:
		if err := validateGotifyNotificationConfig(config); err != nil {
			return err
		}
	}
	return nil
}

func validateNotificationConfigBounds(config map[string]any) error {
	if config == nil {
		return errors.New("config is required")
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return errors.New("config must contain valid JSON values")
	}
	if len(encoded) > maxNotificationConfigBytes {
		return fmt.Errorf("config must be at most %d bytes", maxNotificationConfigBytes)
	}
	return validateNotificationConfigValue(config, 0)
}

func validateNotificationConfigValue(value any, depth int) error {
	if depth > maxNotificationConfigDepth {
		return fmt.Errorf("config nesting must be at most %d levels", maxNotificationConfigDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) > maxNotificationConfigItems {
			return fmt.Errorf("config objects must contain at most %d fields", maxNotificationConfigItems)
		}
		for key, child := range typed {
			if !boundedPrintableNotificationValue(key, 128) {
				return errors.New("config field names must be printable and at most 128 characters")
			}
			if err := validateNotificationConfigValue(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if len(typed) > maxNotificationConfigItems {
			return fmt.Errorf("config arrays must contain at most %d values", maxNotificationConfigItems)
		}
		for _, child := range typed {
			if err := validateNotificationConfigValue(child, depth+1); err != nil {
				return err
			}
		}
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, child := range typed {
			converted[key] = child
		}
		return validateNotificationConfigValue(converted, depth)
	case []string:
		if len(typed) > maxNotificationConfigItems {
			return fmt.Errorf("config arrays must contain at most %d values", maxNotificationConfigItems)
		}
		for _, child := range typed {
			if err := validateNotificationConfigValue(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > maxNotificationConfigStringLen {
			return fmt.Errorf("config string values must be at most %d bytes", maxNotificationConfigStringLen)
		}
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	default:
		return errors.New("config must contain only JSON-compatible values")
	}
	return nil
}

func validateNotificationPriority(raw any) error {
	if raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		return nil
	}
	priority, err := notificationConfigInteger(raw, 0)
	if err != nil || priority < 1 || priority > 5 {
		return errors.New("priority must be an integer between 1 and 5")
	}
	return nil
}

func notificationConfigInteger(raw any, fallback int) (int, error) {
	if raw == nil {
		return fallback, nil
	}
	switch typed := raw.(type) {
	case int:
		return typed, nil
	case int64:
		if int64(int(typed)) != typed {
			return 0, errors.New("integer out of range")
		}
		return int(typed), nil
	case float64:
		if typed != float64(int(typed)) {
			return 0, errors.New("value is not an integer")
		}
		return int(typed), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(typed))
	default:
		return 0, errors.New("value is not an integer")
	}
}

func configBool(config map[string]any, key string) bool {
	switch typed := config[key].(type) {
	case bool:
		return typed
	case string:
		value, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && value
	default:
		return false
	}
}

func notificationStringSlice(raw any) ([]string, bool) {
	switch typed := raw.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			out = append(out, strings.TrimSpace(text))
		}
		return out, true
	default:
		return nil, false
	}
}

func boundedPrintableNotificationValue(value string, maxRunes int) bool {
	if strings.TrimSpace(value) == "" || len([]rune(value)) > maxRunes {
		return false
	}
	return !containsNotificationControl(value)
}

func containsNotificationControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validNotificationHeaderName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func boundedASCIIIdentifier(value string, minLen, maxLen int) bool {
	value = strings.TrimSpace(value)
	if len(value) < minLen || len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func validNotificationBundleID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 255 || containsNotificationControl(value) {
		return false
	}
	segments := strings.Split(value, ".")
	if len(segments) < 2 {
		return false
	}
	for _, segment := range segments {
		if segment == "" {
			return false
		}
		for _, r := range segment {
			if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-') {
				return false
			}
		}
	}
	return true
}

func ValidateCreateRouteRequest(req notifications.CreateRouteRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateMaxLen("name", req.Name, MaxAlertRuleNameLength); err != nil {
		return err
	}
	if req.GroupWaitSeconds < 0 {
		return errors.New("group_wait_seconds must be >= 0")
	}
	if req.GroupIntervalSeconds < 0 {
		return errors.New("group_interval_seconds must be >= 0")
	}
	if req.RepeatIntervalSeconds < 0 {
		return errors.New("repeat_interval_seconds must be >= 0")
	}
	if err := ValidateNoDeprecatedCanonicalPredicateKeys(req.Matchers, "matchers"); err != nil {
		return err
	}
	if err := validateUnsupportedRouteDispatchSettings(req.GroupBy, req.GroupWaitSeconds, req.GroupIntervalSeconds, req.RepeatIntervalSeconds); err != nil {
		return err
	}
	return nil
}

func ValidateUpdateRouteRequest(req notifications.UpdateRouteRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return errors.New("name cannot be empty")
	}
	if req.GroupWaitSeconds != nil && *req.GroupWaitSeconds < 0 {
		return errors.New("group_wait_seconds must be >= 0")
	}
	if req.GroupIntervalSeconds != nil && *req.GroupIntervalSeconds < 0 {
		return errors.New("group_interval_seconds must be >= 0")
	}
	if req.RepeatIntervalSeconds != nil && *req.RepeatIntervalSeconds < 0 {
		return errors.New("repeat_interval_seconds must be >= 0")
	}
	if req.Matchers != nil {
		if err := ValidateNoDeprecatedCanonicalPredicateKeys(*req.Matchers, "matchers"); err != nil {
			return err
		}
	}
	groupBy := []string(nil)
	if req.GroupBy != nil {
		groupBy = *req.GroupBy
	}
	groupWait := 0
	if req.GroupWaitSeconds != nil {
		groupWait = *req.GroupWaitSeconds
	}
	groupInterval := 0
	if req.GroupIntervalSeconds != nil {
		groupInterval = *req.GroupIntervalSeconds
	}
	repeatInterval := 0
	if req.RepeatIntervalSeconds != nil {
		repeatInterval = *req.RepeatIntervalSeconds
	}
	if err := validateUnsupportedRouteDispatchSettings(groupBy, groupWait, groupInterval, repeatInterval); err != nil {
		return err
	}
	return nil
}

func validateUnsupportedRouteDispatchSettings(groupBy []string, groupWaitSeconds, groupIntervalSeconds, repeatIntervalSeconds int) error {
	if len(groupBy) > 0 || groupWaitSeconds > 0 || groupIntervalSeconds > 0 || repeatIntervalSeconds > 0 {
		return errors.New("grouping and repeat interval settings are not supported yet")
	}
	return nil
}
