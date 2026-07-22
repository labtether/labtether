package alerting

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/securityruntime"
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

func validateNotificationEndpoint(config map[string]any, key string, baseOnly bool) error {
	raw, ok := config[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s is required", key)
	}
	raw = strings.TrimSpace(raw)
	if len(raw) > maxNotificationURLLength || containsNotificationControl(raw) {
		return fmt.Errorf("%s must be a printable URL of at most %d bytes", key, maxNotificationURLLength)
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", key)
	}
	if !strings.EqualFold(parsed.Scheme, "https") && !strings.EqualFold(parsed.Scheme, "http") {
		return fmt.Errorf("%s must use HTTP or HTTPS", key)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not contain embedded credentials", key)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain a fragment", key)
	}
	if baseOnly && parsed.RawQuery != "" {
		return fmt.Errorf("%s base URL must not contain a query", key)
	}
	return nil
}

func validateWebhookHeaders(raw any) error {
	if raw == nil {
		return nil
	}
	headers, ok := notificationAnyMap(raw)
	if !ok {
		return errors.New("headers must be an object of string values")
	}
	if len(headers) > 64 {
		return errors.New("headers must contain at most 64 fields")
	}
	for key, value := range headers {
		text, ok := value.(string)
		if !ok || !validNotificationHeaderName(key) || len(text) > 4096 || containsNotificationControl(text) {
			return errors.New("headers must contain valid bounded HTTP header names and string values")
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "host", "content-length", "transfer-encoding", "connection", "upgrade", "trailer":
			return fmt.Errorf("header %q cannot be overridden", key)
		}
	}
	return nil
}

func validateEmailNotificationConfig(config map[string]any) error {
	host := configString(config, "smtp_host")
	if host == "" || len(host) > 253 || containsNotificationControl(host) || strings.ContainsAny(host, "/@") {
		return errors.New("smtp_host must be a valid host name or IP address")
	}
	port, err := notificationConfigInteger(config["smtp_port"], 587)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("smtp_port must be an integer between 1 and 65535")
	}
	from := configString(config, "from")
	if parsed, parseErr := mail.ParseAddress(from); parseErr != nil || parsed == nil || strings.TrimSpace(parsed.Address) == "" {
		return errors.New("from must be a valid email address")
	}
	recipients := firstNonBlank(configString(config, "to"), configString(config, "recipients"), configString(config, "email_to"))
	if recipients == "" || len(recipients) > maxNotificationRecipientListLen {
		return errors.New("to must contain one or more bounded recipient addresses")
	}
	parsedRecipients, parseErr := mail.ParseAddressList(recipients)
	if parseErr != nil || len(parsedRecipients) == 0 || len(parsedRecipients) > maxNotificationRecipientCount {
		return fmt.Errorf("to must contain between 1 and %d valid recipient addresses", maxNotificationRecipientCount)
	}

	mode := strings.ToLower(configString(config, "smtp_tls_mode"))
	if mode == "" {
		if port == 465 {
			mode = "implicit"
		} else {
			mode = "starttls"
		}
	}
	switch mode {
	case "starttls":
	case "implicit", "implicit_tls", "tls", "smtps":
		mode = "implicit"
	case "insecure", "none", "plaintext":
		mode = "insecure"
	default:
		return errors.New("smtp_tls_mode must be starttls, implicit, or insecure")
	}
	user := configString(config, "smtp_user")
	password := configString(config, "smtp_pass")
	if (user == "") != (password == "") {
		return errors.New("SMTP authentication requires both smtp_user and smtp_pass")
	}
	if mode == "insecure" {
		if !configBool(config, "allow_insecure_smtp") || !securityruntime.InsecureTransportAllowed() {
			return errors.New("insecure SMTP requires both channel and process acknowledgement")
		}
		if user != "" || password != "" {
			return errors.New("insecure SMTP cannot carry credentials")
		}
	}
	return nil
}

func validateAPNsNotificationConfig(config map[string]any) error {
	for _, key := range []string{"auth_key_path", "key_id", "team_id", "bundle_id"} {
		if configString(config, key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	authKeyPath := configString(config, "auth_key_path")
	if len(authKeyPath) > 4096 || !filepath.IsAbs(authKeyPath) || filepath.Ext(authKeyPath) != ".p8" || containsNotificationControl(authKeyPath) {
		return errors.New("auth_key_path must be an absolute .p8 file path")
	}
	for _, key := range []string{"key_id", "team_id"} {
		if !boundedASCIIIdentifier(configString(config, key), 3, 64) {
			return fmt.Errorf("%s must be a bounded alphanumeric identifier", key)
		}
	}
	if !validNotificationBundleID(configString(config, "bundle_id")) {
		return errors.New("bundle_id must be a valid application bundle identifier")
	}
	if production, present := config["production"]; present {
		if _, ok := production.(bool); !ok {
			return errors.New("production must be a boolean")
		}
	}
	if allowed, present := config["allowed_bundle_ids"]; present {
		values, ok := notificationStringSlice(allowed)
		if !ok || len(values) > 32 {
			return errors.New("allowed_bundle_ids must be an array of at most 32 bundle identifiers")
		}
		for _, value := range values {
			if !validNotificationBundleID(value) {
				return errors.New("allowed_bundle_ids contains an invalid application bundle identifier")
			}
		}
	}
	return nil
}

func validateNtfyNotificationConfig(config map[string]any) error {
	if err := validateNotificationEndpoint(config, "server_url", true); err != nil {
		return err
	}
	topic := configString(config, "topic")
	if topic == "" || len(topic) > 64 || containsNotificationControl(topic) || strings.ContainsAny(topic, "/?#") {
		return errors.New("topic must be a single printable path segment of at most 64 bytes")
	}
	token := configString(config, "token")
	user := configString(config, "username")
	password := configString(config, "password")
	if token != "" && (user != "" || password != "") {
		return errors.New("ntfy token and basic authentication cannot be combined")
	}
	if (user == "") != (password == "") {
		return errors.New("ntfy basic authentication requires both username and password")
	}
	if err := validateNotificationPriority(config["priority"]); err != nil {
		return err
	}
	if click := configString(config, "click"); click != "" {
		if err := validateNotificationEndpoint(map[string]any{"click": click}, "click", false); err != nil {
			return err
		}
	}
	for _, key := range []string{"tags", "username"} {
		if value := configString(config, key); len(value) > 256 || containsNotificationControl(value) {
			return fmt.Errorf("%s must be printable and at most 256 bytes", key)
		}
	}
	return nil
}

func validateGotifyNotificationConfig(config map[string]any) error {
	if err := validateNotificationEndpoint(config, "server_url", true); err != nil {
		return err
	}
	if firstNonBlank(configString(config, "app_token"), configString(config, "token")) == "" {
		return errors.New("app_token is required")
	}
	return validateNotificationPriority(config["priority"])
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
