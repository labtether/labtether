package alerting

import (
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/securityruntime"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
)

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
