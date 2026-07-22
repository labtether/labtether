package remotewrite

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/runtimesettings"
)

// ConfigFromRuntimeValues converts the trusted effective runtime-settings map
// into one validated worker configuration.
func ConfigFromRuntimeValues(values map[string]string) (Config, error) {
	enabled, err := strconv.ParseBool(values[runtimesettings.KeyPrometheusRemoteWriteEnabled])
	if err != nil {
		return Config{}, fmt.Errorf("remote write enabled setting is invalid")
	}
	interval, err := time.ParseDuration(values[runtimesettings.KeyPrometheusRemoteWriteInterval])
	if err != nil {
		return Config{}, fmt.Errorf("remote write interval setting is invalid")
	}
	return NormalizeConfig(Config{
		Enabled:  enabled,
		URL:      values[runtimesettings.KeyPrometheusRemoteWriteURL],
		Username: values[runtimesettings.KeyPrometheusRemoteWriteUsername],
		Password: values[runtimesettings.KeyPrometheusRemoteWritePassword],
		Interval: interval,
	})
}

const (
	DefaultInterval = 30 * time.Second
	MinInterval     = 10 * time.Second
	MaxInterval     = time.Hour

	MaxURLBytes      = 4096
	MaxUsernameBytes = 512
	MaxPasswordBytes = 4096
)

// Config is the complete, immutable configuration for one remote_write worker.
// Password is intentionally kept only in memory; callers must never log Config.
type Config struct {
	Enabled  bool
	URL      string
	Username string
	Password string // #nosec G117 -- Runtime credential, never a hardcoded secret.
	Interval time.Duration
}

// NormalizeConfig validates and canonicalizes a runtime configuration. It does
// not perform DNS or outbound-policy checks; those are re-evaluated for every
// request so a live security policy change cannot be bypassed by stale state.
func NormalizeConfig(config Config) (Config, error) {
	config.URL = strings.TrimSpace(config.URL)
	config.Username = strings.TrimSpace(config.Username)
	if config.Interval == 0 {
		config.Interval = DefaultInterval
	}
	if config.Interval < MinInterval || config.Interval > MaxInterval {
		return Config{}, fmt.Errorf("remote write interval must be between %s and %s", MinInterval, MaxInterval)
	}
	if err := validateCredentialField("username", config.Username, MaxUsernameBytes, true); err != nil {
		return Config{}, err
	}
	if err := validateCredentialField("password", config.Password, MaxPasswordBytes, false); err != nil {
		return Config{}, err
	}
	if config.Enabled && config.Password != "" && config.Username == "" {
		return Config{}, fmt.Errorf("remote write username is required when a password is configured")
	}
	if config.URL == "" {
		if config.Enabled {
			return Config{}, fmt.Errorf("remote write URL is required when enabled")
		}
		return config, nil
	}
	if len(config.URL) > MaxURLBytes || !utf8.ValidString(config.URL) || strings.ContainsRune(config.URL, '\x00') {
		return Config{}, fmt.Errorf("remote write URL exceeds the allowed UTF-8 byte limit")
	}
	parsed, err := url.Parse(config.URL)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return Config{}, fmt.Errorf("remote write URL must be an absolute HTTP(S) URL")
	}
	parsed.Scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return Config{}, fmt.Errorf("remote write URL must use HTTP or HTTPS")
	}
	if parsed.User != nil {
		return Config{}, fmt.Errorf("remote write URL must not contain user information")
	}
	if port := parsed.Port(); port != "" {
		parsedPort, portErr := strconv.Atoi(port)
		if portErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return Config{}, fmt.Errorf("remote write URL port is invalid")
		}
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return Config{}, fmt.Errorf("remote write URL must not contain a query string")
	}
	if parsed.Fragment != "" {
		return Config{}, fmt.Errorf("remote write URL must not contain a fragment")
	}
	if strings.ContainsAny(config.URL, "\r\n\t") {
		return Config{}, fmt.Errorf("remote write URL contains control characters")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	config.URL = parsed.String()
	return config, nil
}

func validateCredentialField(name, value string, maxBytes int, rejectColon bool) error {
	if len(value) > maxBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("remote write %s exceeds the allowed UTF-8 byte limit", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("remote write %s contains control characters", name)
	}
	if rejectColon && strings.ContainsRune(value, ':') {
		return fmt.Errorf("remote write username must not contain a colon")
	}
	return nil
}

// EndpointFingerprint is the non-secret durable identity used to decide
// whether a replacement destination can safely reuse the prior replay cursor.
// The Basic Auth username is included because many hosted receivers use it as
// a tenant identifier. Password rotations at the same tenant retain progress.
func (c Config) EndpointFingerprint() (string, error) {
	normalized, err := NormalizeConfig(c)
	if err != nil {
		return "", err
	}
	if normalized.URL == "" {
		return "", fmt.Errorf("remote write URL is required")
	}
	identity := make([]byte, 8+len(normalized.URL)+len(normalized.Username))
	binary.BigEndian.PutUint32(identity[:4], uint32(len(normalized.URL))) // #nosec G115 -- NormalizeConfig caps the URL at 4096 bytes.
	copy(identity[4:], normalized.URL)
	offset := 4 + len(normalized.URL)
	binary.BigEndian.PutUint32(identity[offset:offset+4], uint32(len(normalized.Username))) // #nosec G115 -- NormalizeConfig caps the username at 512 bytes.
	copy(identity[offset+4:], normalized.Username)
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:]), nil
}
