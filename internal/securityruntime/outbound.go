package securityruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	envOutboundAllowedSchemes = "LABTETHER_OUTBOUND_ALLOWED_SCHEMES"
	envAllowInsecureTransport = "LABTETHER_ALLOW_INSECURE_TRANSPORT"
)

var defaultAllowedOutboundSchemes = []string{"https", "wss"}
var privateHostnameSuffixes = []string{".local", ".lan", ".home", ".internal", ".home.arpa"}
var lookupIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}
var lookupIP = net.LookupIP

func normalizeHostname(value string) string {
	host := strings.TrimSpace(strings.ToLower(value))
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSuffix(host, ".")
	return host
}

func parseHostPattern(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		if parsed, err := url.Parse(trimmed); err == nil {
			trimmed = parsed.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	if strings.Contains(trimmed, "/") {
		if _, _, err := net.ParseCIDR(trimmed); err == nil {
			return trimmed
		}
	}
	return normalizeHostname(trimmed)
}

func parseAllowedHostPatterns() []string {
	patterns := parseCSVEnv(envOutboundAllowedHosts, nil)
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		normalized := parseHostPattern(pattern)
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func isPrivateIPAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isLikelyPrivateHostname(host string) bool {
	if host == "" {
		return false
	}
	if !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range privateHostnameSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func hostMatchesPattern(host, pattern string) bool {
	host = normalizeHostname(host)
	if host == "" || pattern == "" {
		return false
	}

	if strings.Contains(pattern, "/") {
		if ip := net.ParseIP(host); ip != nil {
			if _, cidr, err := net.ParseCIDR(pattern); err == nil {
				return cidr.Contains(ip)
			}
		}
		return false
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		if suffix == "" {
			return false
		}
		return strings.HasSuffix(host, "."+suffix)
	}

	return strings.EqualFold(host, pattern)
}

func validateOutboundHost(host string) error {
	return validateOutboundHostWithPolicy(
		host,
		parseBoolEnv(envOutboundAllowlistMode, false),
		parseBoolEnv(envOutboundAllowPrivate, false),
		parseBoolEnv(envOutboundAllowLoopback, false),
	)
}

func validateOutboundHostWithPolicy(host string, enforceAllowlist, allowPrivate, allowLoopback bool) error {
	normalized := normalizeHostname(host)
	if normalized == "" {
		return fmt.Errorf("host is required")
	}

	allowlisted := false

	if enforceAllowlist {
		for _, pattern := range parseAllowedHostPatterns() {
			if hostMatchesPattern(normalized, pattern) {
				allowlisted = true
				break
			}
		}
	}

	isLoopbackHost, isPrivateHost := hostRiskProfile(normalized)
	if isLoopbackHost {
		if !allowLoopback {
			return fmt.Errorf("outbound loopback host %q is not allowed", normalized)
		}
		if enforceAllowlist && !allowlisted {
			return fmt.Errorf("outbound host %q is not allowlisted", normalized)
		}
		return nil
	}
	if isPrivateHost {
		if !allowPrivate {
			return fmt.Errorf("outbound private host %q is not allowed", normalized)
		}
		if enforceAllowlist && !allowlisted {
			return fmt.Errorf("outbound host %q is not allowlisted", normalized)
		}
		return nil
	}

	if !enforceAllowlist {
		return validateResolvedOutboundHost(normalized, allowLoopback, allowPrivate)
	}

	if err := validateResolvedOutboundHost(normalized, allowLoopback, allowPrivate); err != nil {
		return err
	}
	if allowlisted {
		return nil
	}
	return fmt.Errorf("outbound host %q is not allowlisted", normalized)
}

func defaultAllowPrivateForScheme(scheme string) bool {
	return strings.EqualFold(strings.TrimSpace(scheme), "https") ||
		strings.EqualFold(strings.TrimSpace(scheme), "wss")
}

func resolvedOutboundAllowPrivate(scheme string) bool {
	if value, present := parseBoolEnvWithPresence(envOutboundAllowPrivate, false); present {
		return value
	}
	return defaultAllowPrivateForScheme(scheme)
}

func hostRiskProfile(host string) (isLoopback bool, isPrivate bool) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true, true
		}
		return false, isPrivateIPAddress(ip)
	}
	if strings.EqualFold(host, "localhost") {
		return true, true
	}
	resolvedLoopback, resolvedPrivate, resolved := resolvedHostRisk(host)
	if resolved {
		return resolvedLoopback, resolvedPrivate
	}
	return false, isLikelyPrivateHostname(host)
}

func resolvedHostRisk(host string) (isLoopback bool, isPrivate bool, resolved bool) {
	host = normalizeHostname(host)
	if host == "" {
		return false, false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false, false, false
	}
	resolved = true
	for _, addr := range addrs {
		ip := addr.IP
		if ip == nil {
			continue
		}
		if ip.IsLoopback() {
			isLoopback = true
			isPrivate = true
			continue
		}
		if isPrivateIPAddress(ip) {
			isPrivate = true
		}
	}
	return isLoopback, isPrivate, resolved
}

func validateResolvedOutboundHost(host string, allowLoopback, allowPrivate bool) error {
	if ip := net.ParseIP(host); ip != nil {
		return validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate)
	}

	resolvedIPs, err := lookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range resolvedIPs {
		if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate); err != nil {
			return err
		}
	}
	return nil
}

func validateResolvedOutboundIP(host string, ip net.IP, allowLoopback, allowPrivate bool) error {
	if ip == nil {
		return nil
	}
	if ip.IsLoopback() && !allowLoopback {
		return fmt.Errorf("outbound host %q resolves to disallowed loopback address %s", host, ip.String())
	}
	if isPrivateIPAddress(ip) && !allowPrivate {
		return fmt.Errorf("outbound host %q resolves to disallowed private address %s", host, ip.String())
	}
	return nil
}

func ValidateOutboundURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("url must be absolute")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("url host is required")
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
	if err := validateOutboundHostWithPolicy(parsed.Hostname(), enforceAllowlist, allowPrivate, allowLoopback); err != nil {
		return nil, err
	}

	return parsed, nil
}

func NewOutboundRequestWithContext(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	parsed, err := ValidateOutboundURL(rawURL)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- URL host/scheme validated by ValidateOutboundURL allowlist policy.
	return http.NewRequestWithContext(ctx, method, parsed.String(), body)
}

func DoOutboundRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("request is required")
	}
	if _, err := ValidateOutboundURL(req.URL.String()); err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	// #nosec G704 -- request URL host/scheme validated by ValidateOutboundURL allowlist policy.
	return client.Do(req)
}

func ValidateOutboundDialTarget(host string, port int) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("host is required")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	return validateOutboundHost(host)
}

func ValidateOutboundHostPort(host, portRaw string, fallbackPort int) (string, int, error) {
	normalizedHost := strings.TrimSpace(host)
	if normalizedHost == "" {
		return "", 0, fmt.Errorf("host is required")
	}
	port := fallbackPort
	if trimmedPort := strings.TrimSpace(portRaw); trimmedPort != "" {
		parsedPort, err := strconv.Atoi(trimmedPort)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port %q", trimmedPort)
		}
		port = parsedPort
	}
	if err := ValidateOutboundDialTarget(normalizedHost, port); err != nil {
		return "", 0, err
	}
	return normalizedHost, port, nil
}

func DialOutboundTCPTimeout(host string, port int, timeout time.Duration) (net.Conn, error) {
	if err := ValidateOutboundDialTarget(host, port); err != nil {
		return nil, err
	}
	address := net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port))
	// #nosec G704 -- outbound host validated by ValidateOutboundDialTarget allowlist policy.
	return net.DialTimeout("tcp", address, timeout)
}

func OutboundPolicySummary() map[string]string {
	allowlistMode := parseBoolEnv(envOutboundAllowlistMode, false)
	allowPrivate := parseBoolEnv(envOutboundAllowPrivate, false)
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	return map[string]string{
		"allowlist_mode":           strconv.FormatBool(allowlistMode),
		"allow_private":            strconv.FormatBool(allowPrivate),
		"allow_loopback":           strconv.FormatBool(allowLoopback),
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
