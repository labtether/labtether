package securityruntime

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	envOutboundAllowlistMode  = "LABTETHER_OUTBOUND_ALLOWLIST_MODE"
	envOutboundAllowedHosts   = "LABTETHER_OUTBOUND_ALLOWED_HOSTS"
	envOutboundAllowPrivate   = "LABTETHER_OUTBOUND_ALLOW_PRIVATE"
	envOutboundAllowLoopback  = "LABTETHER_OUTBOUND_ALLOW_LOOPBACK"
	envOutboundAllowLinkLocal = "LABTETHER_OUTBOUND_ALLOW_LINK_LOCAL"
	envOutboundAllowedSchemes = "LABTETHER_OUTBOUND_ALLOWED_SCHEMES"
	envAllowInsecureTransport = "LABTETHER_ALLOW_INSECURE_TRANSPORT"
	defaultOutboundTimeout    = 30 * time.Second
)

// InsecureTransportAllowed reports whether the process-wide, explicitly named
// insecure transport escape hatch is enabled. Protocol-specific callers must
// still require their own local acknowledgement before using this value.
func InsecureTransportAllowed() bool {
	return parseBoolEnv(envAllowInsecureTransport, false)
}

var defaultAllowedOutboundSchemes = []string{"https", "wss"}

func ValidateOutboundURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid url")
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("url must be absolute")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("url host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("url must not contain embedded credentials")
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if (scheme == "http" || scheme == "ws") && !parseBoolEnv(envAllowInsecureTransport, false) {
		return nil, fmt.Errorf("insecure url scheme %q requires %s=true", scheme, envAllowInsecureTransport)
	}
	allowedSchemes := toSet(effectiveAllowedOutboundSchemes(), strings.ToLower)
	if _, ok := allowedSchemes[scheme]; !ok {
		return nil, fmt.Errorf("url scheme %q is not allowed", scheme)
	}

	enforceAllowlist := parseBoolEnv(envOutboundAllowlistMode, false)
	allowPrivate := resolvedOutboundAllowPrivate(scheme)
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	if err := validateOutboundHostWithPolicy(parsed.Hostname(), enforceAllowlist, allowPrivate, allowLoopback, allowLinkLocal); err != nil {
		return nil, err
	}

	return parsed, nil
}

func OutboundPolicySummary() map[string]string {
	allowlistMode := parseBoolEnv(envOutboundAllowlistMode, false)
	allowPrivateHTTPS := resolvedOutboundAllowPrivate("https")
	allowPrivateWSS := resolvedOutboundAllowPrivateWSS()
	allowPrivateTCP := resolvedOutboundAllowPrivateTCP()
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	// allow_private remains a backward-compatible alias for the generic HTTPS
	// URL policy. Callers needing socket behavior use the transport fields.
	return map[string]string{
		"allowlist_mode":           strconv.FormatBool(allowlistMode),
		"allow_private":            strconv.FormatBool(allowPrivateHTTPS),
		"allow_private_https":      strconv.FormatBool(allowPrivateHTTPS),
		"allow_private_wss":        strconv.FormatBool(allowPrivateWSS),
		"allow_private_tcp":        strconv.FormatBool(allowPrivateTCP),
		"allow_loopback":           strconv.FormatBool(allowLoopback),
		"allow_link_local":         strconv.FormatBool(allowLinkLocal),
		"allow_insecure_transport": strconv.FormatBool(parseBoolEnv(envAllowInsecureTransport, false)),
		"allowed_hosts":            strings.Join(parseAllowedHostPatterns(), ","),
		"schemes":                  strings.Join(effectiveAllowedOutboundSchemes(), ","),
	}
}

func effectiveAllowedOutboundSchemes() []string {
	schemes := parseCSVEnv(envOutboundAllowedSchemes, defaultAllowedOutboundSchemes)
	if parseBoolEnv(envAllowInsecureTransport, false) {
		if !containsStringFold(schemes, "http") {
			schemes = append(schemes, "http")
		}
		if !containsStringFold(schemes, "ws") {
			schemes = append(schemes, "ws")
		}
	}
	return schemes
}

func containsStringFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), candidate) {
			return true
		}
	}
	return false
}
