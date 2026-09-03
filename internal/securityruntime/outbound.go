package securityruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
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
var privateHostnameSuffixes = []string{".local", ".lan", ".home", ".internal", ".home.arpa"}
var sharedAddressSpacePrefix = netip.MustParsePrefix("100.64.0.0/10")
var nonPublicSpecialPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	sharedAddressSpacePrefix,
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fec0::/10"),
}
var urlUserinfoInTextPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/?#\r\n]*@`)
var lookupIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}
var lookupIP = net.LookupIP
var lookupDialIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return lookupIPAddrs(ctx, host)
}

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
	return ip.IsPrivate()
}

func isPublicInternetIPAddress(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublicSpecialPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func isPermittedPrivateIPAddress(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.IsPrivate() || sharedAddressSpacePrefix.Contains(addr)
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
		resolvedOutboundAllowPrivateTCP(),
		parseBoolEnv(envOutboundAllowLoopback, false),
		parseBoolEnv(envOutboundAllowLinkLocal, false),
	)
}

func validateOutboundHostWithPolicy(host string, enforceAllowlist, allowPrivate, allowLoopback, allowLinkLocal bool) error {
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
		if !allowlisted {
			return fmt.Errorf("outbound host %q is not allowlisted", normalized)
		}
	}

	isLoopbackHost, isPrivateHost, isLinkLocalHost := hostRiskProfile(normalized)
	if isLoopbackHost {
		if !allowLoopback {
			return fmt.Errorf("outbound loopback host %q is not allowed", normalized)
		}
		if enforceAllowlist && !allowlisted {
			return fmt.Errorf("outbound host %q is not allowlisted", normalized)
		}
		return nil
	}
	if isLinkLocalHost {
		if !allowLinkLocal {
			return fmt.Errorf("outbound link-local host %q is not allowed", normalized)
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
		return validateResolvedOutboundHost(normalized, allowLoopback, allowPrivate, allowLinkLocal)
	}

	if err := validateResolvedOutboundHost(normalized, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
		return err
	}
	return nil
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

func resolvedOutboundAllowPrivateTCP() bool {
	return parseBoolEnv(envOutboundAllowPrivate, false)
}

func resolvedOutboundAllowPrivateWSS() bool {
	return resolvedOutboundAllowPrivate("wss") && resolvedOutboundAllowPrivateTCP()
}

func hostRiskProfile(host string) (isLoopback bool, isPrivate bool, isLinkLocal bool) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true, false, false
		}
		if ip.IsLinkLocalUnicast() {
			return false, false, true
		}
		return false, isPrivateIPAddress(ip), false
	}
	if strings.EqualFold(host, "localhost") {
		return true, false, false
	}
	resolvedLoopback, resolvedPrivate, resolvedLinkLocal, resolved := resolvedHostRisk(host)
	if resolved {
		return resolvedLoopback, resolvedPrivate, resolvedLinkLocal
	}
	return false, isLikelyPrivateHostname(host), false
}

func resolvedHostRisk(host string) (isLoopback bool, isPrivate bool, isLinkLocal bool, resolved bool) {
	host = normalizeHostname(host)
	if host == "" {
		return false, false, false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false, false, false, false
	}
	resolved = true
	for _, addr := range addrs {
		ip := addr.IP
		if ip == nil {
			continue
		}
		if ip.IsLoopback() {
			isLoopback = true
			continue
		}
		if ip.IsLinkLocalUnicast() {
			isLinkLocal = true
			continue
		}
		if isPrivateIPAddress(ip) {
			isPrivate = true
		}
	}
	return isLoopback, isPrivate, isLinkLocal, resolved
}

func validateResolvedOutboundHost(host string, allowLoopback, allowPrivate, allowLinkLocal bool) error {
	if ip := net.ParseIP(host); ip != nil {
		return validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal)
	}

	resolvedIPs, err := lookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve outbound host %q: %w", host, err)
	}
	if len(resolvedIPs) == 0 {
		return fmt.Errorf("outbound host %q did not resolve", host)
	}
	for _, ip := range resolvedIPs {
		if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return err
		}
	}
	return nil
}

func validateResolvedOutboundIP(host string, ip net.IP, allowLoopback, allowPrivate, allowLinkLocal bool) error {
	if ip == nil {
		return fmt.Errorf("outbound host %q resolved to an invalid address", host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("outbound host %q resolves to disallowed unspecified address %s", host, ip.String())
	}
	if ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return fmt.Errorf("outbound host %q resolves to disallowed multicast or broadcast address %s", host, ip.String())
	}
	if ip.IsLoopback() && !allowLoopback {
		return fmt.Errorf("outbound host %q resolves to disallowed loopback address %s", host, ip.String())
	}
	if ip.IsLinkLocalUnicast() && !allowLinkLocal {
		return fmt.Errorf("outbound host %q resolves to disallowed link-local address %s", host, ip.String())
	}
	if isPrivateIPAddress(ip) && !allowPrivate {
		return fmt.Errorf("outbound host %q resolves to disallowed private address %s", host, ip.String())
	}
	return nil
}

// URLContainsUserinfo reports whether rawURL has an authority containing an
// embedded username or password. It deliberately recognizes malformed URL
// escapes too, so invalid input cannot bypass secret filtering.
func URLContainsUserinfo(rawURL string) bool {
	_, _, ok := urlUserinfoBounds(rawURL)
	return ok
}

// RedactURLUserinfo removes an embedded username and password from a URL
// authority. The boolean result is false when the value has no userinfo, in
// which case the original value is returned unchanged.
func RedactURLUserinfo(rawURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawURL)
	authorityStart, userinfoEnd, ok := urlUserinfoBounds(trimmed)
	if !ok {
		return rawURL, false
	}
	return trimmed[:authorityStart] + trimmed[userinfoEnd:], true
}

// RedactURLUserinfoInText removes URL usernames and passwords from diagnostic
// text without exposing or interpreting the credential bytes.
func RedactURLUserinfoInText(value string) (string, bool) {
	redacted := urlUserinfoInTextPattern.ReplaceAllString(value, "${1}")
	return redacted, redacted != value
}

// RedactURLUserinfoValues returns a cloned string map with URL userinfo removed
// from every value. Callers can safely redact a response without mutating the
// stored metadata that supplied it.
func RedactURLUserinfoValues(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if safeValue, changed := RedactURLUserinfo(value); changed {
			redacted[key] = safeValue
		} else {
			redacted[key] = value
		}
	}
	return redacted
}

func urlUserinfoBounds(rawURL string) (authorityStart int, userinfoEnd int, ok bool) {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "//") {
		authorityStart = 2
	} else {
		schemeEnd := strings.Index(trimmed, "://")
		if schemeEnd <= 0 {
			return 0, 0, false
		}
		for index, char := range trimmed[:schemeEnd] {
			if index == 0 {
				if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
					return 0, 0, false
				}
				continue
			}
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '+' && char != '-' && char != '.' {
				return 0, 0, false
			}
		}
		authorityStart = schemeEnd + 3
	}

	authorityEnd := len(trimmed)
	if suffix := strings.IndexAny(trimmed[authorityStart:], "/?#"); suffix >= 0 {
		authorityEnd = authorityStart + suffix
	}
	userinfoAt := strings.LastIndex(trimmed[authorityStart:authorityEnd], "@")
	if userinfoAt < 0 {
		return 0, 0, false
	}
	return authorityStart, authorityStart + userinfoAt + 1, true
}

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
	client = secureOutboundHTTPClient(client, strings.ToLower(strings.TrimSpace(req.URL.Scheme)))
	// #nosec G704 -- request URL host/scheme validated by ValidateOutboundURL allowlist policy.
	return client.Do(req)
}

// OutboundAddressPolicy adds a caller-specific ceiling to the process-wide
// outbound policy. A false value always denies that address class, even if a
// broader runtime setting allows it.
type OutboundAddressPolicy struct {
	AllowPrivate   bool
	AllowLoopback  bool
	AllowLinkLocal bool
}

// NewOriginRestrictedOutboundHTTPClient returns an HTTP client that may only
// contact the exact origins supplied in allowedURLs. Every request and
// redirect is checked against the current outbound policy, while the transport
// re-resolves and validates every address again immediately before connecting.
//
// Origins compare scheme, normalized hostname, and effective port. Paths,
// queries, and fragments in allowedURLs do not widen the boundary.
func NewOriginRestrictedOutboundHTTPClient(base *http.Client, addressPolicy OutboundAddressPolicy, allowedURLs ...string) (*http.Client, error) {
	if base != nil && base.Transport != nil {
		if _, ok := base.Transport.(*http.Transport); !ok {
			return nil, fmt.Errorf("origin-restricted client requires a standard HTTP transport")
		}
	}
	allowedOrigins, scheme, err := validatedOutboundOrigins(addressPolicy, allowedURLs)
	if err != nil {
		return nil, err
	}

	client := secureOutboundHTTPClientWithAddressPolicy(base, scheme, &addressPolicy)
	client.Transport = originRestrictedRoundTripper{
		base:           client.Transport,
		allowedOrigins: allowedOrigins,
		addressPolicy:  addressPolicy,
	}
	priorRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return fmt.Errorf("redirect target is required")
		}
		if err := validateOutboundURLAgainstOrigins(req.URL.String(), allowedOrigins, addressPolicy); err != nil {
			return err
		}
		if len(via) > 0 && via[0] != nil && via[0].URL != nil {
			fromOrigin, err := canonicalOutboundOrigin(via[0].URL)
			if err != nil {
				return err
			}
			toOrigin, err := canonicalOutboundOrigin(req.URL)
			if err != nil {
				return err
			}
			if fromOrigin != toOrigin {
				return fmt.Errorf("cross-origin redirect from %q to %q is not allowed", fromOrigin, toOrigin)
			}
		}
		return priorRedirect(req, via)
	}
	return client, nil
}

// ValidateOutboundURLAgainstOrigins applies the process outbound policy and
// requires rawURL to match one of the exact origins represented by allowedURLs.
func ValidateOutboundURLAgainstOrigins(rawURL string, addressPolicy OutboundAddressPolicy, allowedURLs ...string) error {
	allowedOrigins, _, err := validatedOutboundOrigins(addressPolicy, allowedURLs)
	if err != nil {
		return err
	}
	return validateOutboundURLAgainstOrigins(rawURL, allowedOrigins, addressPolicy)
}

func validatedOutboundOrigins(addressPolicy OutboundAddressPolicy, allowedURLs []string) (map[string]struct{}, string, error) {
	if len(allowedURLs) == 0 {
		return nil, "", fmt.Errorf("at least one allowed outbound origin is required")
	}
	allowedOrigins := make(map[string]struct{}, len(allowedURLs))
	var requiredScheme string
	for _, rawURL := range allowedURLs {
		parsed, err := validateOutboundURLWithAddressPolicy(rawURL, addressPolicy)
		if err != nil {
			return nil, "", err
		}
		origin, err := canonicalOutboundOrigin(parsed)
		if err != nil {
			return nil, "", err
		}
		scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
		if requiredScheme == "" {
			requiredScheme = scheme
		} else if scheme != requiredScheme {
			return nil, "", fmt.Errorf("allowed outbound origins must use the same scheme")
		}
		allowedOrigins[origin] = struct{}{}
	}
	return allowedOrigins, requiredScheme, nil
}

func validateOutboundURLAgainstOrigins(rawURL string, allowedOrigins map[string]struct{}, addressPolicy OutboundAddressPolicy) error {
	parsed, err := validateOutboundURLWithAddressPolicy(rawURL, addressPolicy)
	if err != nil {
		return err
	}
	origin, err := canonicalOutboundOrigin(parsed)
	if err != nil {
		return err
	}
	if _, ok := allowedOrigins[origin]; !ok {
		return fmt.Errorf("outbound origin %q is not allowed", origin)
	}
	return nil
}

func validateOutboundURLWithAddressPolicy(rawURL string, addressPolicy OutboundAddressPolicy) (*url.URL, error) {
	parsed, err := ValidateOutboundURL(rawURL)
	if err != nil {
		return nil, err
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	allowPrivate := resolvedOutboundAllowPrivate(scheme) && addressPolicy.AllowPrivate
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false) && addressPolicy.AllowLoopback
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false) && addressPolicy.AllowLinkLocal
	if err := validateRestrictedOutboundHost(parsed.Hostname(), allowLoopback, allowPrivate, allowLinkLocal); err != nil {
		return nil, err
	}
	return parsed, nil
}

func canonicalOutboundOrigin(parsed *url.URL) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("url is required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("url userinfo is not allowed")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := normalizeHostname(parsed.Hostname())
	if scheme == "" || host == "" {
		return "", fmt.Errorf("url scheme and host are required")
	}
	port := parsed.Port()
	if port == "" {
		switch scheme {
		case "https", "wss":
			port = "443"
		case "http", "ws":
			port = "80"
		default:
			return "", fmt.Errorf("url scheme %q has no default port", scheme)
		}
	}
	if parsedPort, err := strconv.Atoi(port); err != nil || parsedPort <= 0 || parsedPort > 65535 {
		return "", fmt.Errorf("invalid url port %q", port)
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}

func secureOutboundHTTPClient(base *http.Client, scheme string) *http.Client {
	return secureOutboundHTTPClientWithAddressPolicy(base, scheme, nil)
}

func secureOutboundHTTPClientWithAddressPolicy(base *http.Client, scheme string, addressPolicy *OutboundAddressPolicy) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: defaultOutboundTimeout}
	}
	client := *base
	if client.Timeout <= 0 {
		client.Timeout = defaultOutboundTimeout
	}
	client.Transport = secureOutboundRoundTripper(base.Transport, scheme, addressPolicy)
	priorRedirect := base.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return fmt.Errorf("redirect target is required")
		}
		if _, err := ValidateOutboundURL(req.URL.String()); err != nil {
			return err
		}
		if priorRedirect != nil {
			return priorRedirect(req, via)
		}
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &client
}

func secureOutboundRoundTripper(base http.RoundTripper, scheme string, addressPolicy *OutboundAddressPolicy) http.RoundTripper {
	var transport *http.Transport
	if t, ok := base.(*http.Transport); ok && t != nil {
		transport = t.Clone()
	} else if base == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		return outboundValidatingRoundTripper{base: base}
	}
	transport.Proxy = nil
	transport.DialTLSContext = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		allowPrivate := resolvedOutboundAllowPrivate(scheme)
		allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
		allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
		if addressPolicy != nil {
			allowPrivate = allowPrivate && addressPolicy.AllowPrivate
			allowLoopback = allowLoopback && addressPolicy.AllowLoopback
			allowLinkLocal = allowLinkLocal && addressPolicy.AllowLinkLocal
		}
		return dialOutboundValidated(ctx, network, address, allowLoopback, allowPrivate, allowLinkLocal, addressPolicy != nil)
	}
	return outboundValidatingRoundTripper{base: transport}
}

type outboundValidatingRoundTripper struct {
	base http.RoundTripper
}

func (rt outboundValidatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("request is required")
	}
	if _, err := ValidateOutboundURL(req.URL.String()); err != nil {
		return nil, err
	}
	return rt.base.RoundTrip(req)
}

type originRestrictedRoundTripper struct {
	base           http.RoundTripper
	allowedOrigins map[string]struct{}
	addressPolicy  OutboundAddressPolicy
}

func (rt originRestrictedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("request is required")
	}
	if err := validateOutboundURLAgainstOrigins(req.URL.String(), rt.allowedOrigins, rt.addressPolicy); err != nil {
		return nil, err
	}
	return rt.base.RoundTrip(req)
}

func dialOutboundValidated(ctx context.Context, network, address string, allowLoopback, allowPrivate, allowLinkLocal, requirePublic bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	host = normalizeHostname(host)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	dialer := &net.Dialer{Timeout: defaultOutboundTimeout, KeepAlive: 30 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if err := validateRestrictedResolvedIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal, requirePublic); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	addrs, err := lookupDialIPAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host %q did not resolve", host)
	}
	for _, addr := range addrs {
		if err := validateRestrictedResolvedIP(host, addr.IP, allowLoopback, allowPrivate, allowLinkLocal, requirePublic); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("host %q did not resolve to a dialable address", host)
}

func validateRestrictedOutboundHost(host string, allowLoopback, allowPrivate, allowLinkLocal bool) error {
	normalized := normalizeHostname(host)
	if normalized == "" {
		return fmt.Errorf("host is required")
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return validateRestrictedResolvedIP(normalized, ip, allowLoopback, allowPrivate, allowLinkLocal, true)
	}
	resolvedIPs, err := lookupIP(normalized)
	if err != nil {
		return fmt.Errorf("resolve outbound host %q: %w", normalized, err)
	}
	if len(resolvedIPs) == 0 {
		return fmt.Errorf("outbound host %q did not resolve", normalized)
	}
	for _, ip := range resolvedIPs {
		if err := validateRestrictedResolvedIP(normalized, ip, allowLoopback, allowPrivate, allowLinkLocal, true); err != nil {
			return err
		}
	}
	return nil
}

func validateRestrictedResolvedIP(host string, ip net.IP, allowLoopback, allowPrivate, allowLinkLocal, requirePublic bool) error {
	if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
		return err
	}
	if !requirePublic || isPublicInternetIPAddress(ip) || (ip.IsLoopback() && allowLoopback) || (ip.IsLinkLocalUnicast() && allowLinkLocal) {
		return nil
	}
	if allowPrivate && isPermittedPrivateIPAddress(ip) {
		return nil
	}
	return fmt.Errorf("outbound host %q resolves to disallowed non-public address %s", host, ip.String())
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

// CanonicalizeOutboundHost accepts exactly one host value. It deliberately
// rejects URLs, userinfo, paths, queries, fragments, embedded ports, control
// characters, and ambiguous bracket forms before any DNS lookup occurs.
// IPv6 literals may be supplied bare or inside one matching bracket pair.
func CanonicalizeOutboundHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if len(host) > 253 {
		return "", fmt.Errorf("host too long (max 253 characters)")
	}
	for _, char := range host {
		if char <= 0x20 || char == 0x7f || char > 0x7f {
			return "", fmt.Errorf("host contains unsupported characters")
		}
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/\\@?#") {
		return "", fmt.Errorf("host must not contain a URL, userinfo, path, query, or fragment")
	}

	if strings.HasPrefix(host, "[") || strings.HasSuffix(host, "]") {
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") || len(host) < 3 {
			return "", fmt.Errorf("invalid bracketed host")
		}
		host = host[1 : len(host)-1]
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() != nil {
			return "", fmt.Errorf("brackets are only valid around an IPv6 literal")
		}
		return strings.ToLower(ip.String()), nil
	}
	if strings.ContainsAny(host, "[]") {
		return "", fmt.Errorf("invalid bracketed host")
	}

	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if ip := net.ParseIP(host); ip != nil {
		return strings.ToLower(ip.String()), nil
	}
	if strings.Contains(host, ":") {
		return "", fmt.Errorf("host must not include a port or malformed IPv6 literal")
	}

	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname label")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return "", fmt.Errorf("invalid hostname character")
			}
		}
	}
	return host, nil
}

// ValidateOutboundEndpoint canonicalizes a separately supplied host and port,
// then applies the process outbound policy (allowlist and public/private,
// loopback, and link-local controls). The returned host is safe to retain as
// the authoritative server-side session target.
func ValidateOutboundEndpoint(rawHost string, port int) (string, int, error) {
	host, err := CanonicalizeOutboundHost(rawHost)
	if err != nil {
		return "", 0, err
	}
	if err := ValidateOutboundDialTarget(host, port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// ResolveOutboundTCPHost resolves and validates an endpoint immediately before
// handing it to an out-of-process TCP client such as guacd. It returns a
// literal IP so that the downstream process cannot perform a second DNS lookup
// and bypass the hub's outbound policy.
func ResolveOutboundTCPHost(ctx context.Context, rawHost string, port int) (string, error) {
	host, err := CanonicalizeOutboundHost(rawHost)
	if err != nil {
		return "", err
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid port %d", port)
	}

	enforceAllowlist := parseBoolEnv(envOutboundAllowlistMode, false)
	if enforceAllowlist {
		allowed := false
		for _, pattern := range parseAllowedHostPatterns() {
			if hostMatchesPattern(host, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("outbound host %q is not allowlisted", host)
		}
	}

	allowPrivate := resolvedOutboundAllowPrivateTCP()
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	if strings.EqualFold(host, "localhost") && !allowLoopback {
		return "", fmt.Errorf("outbound loopback host %q is not allowed", host)
	}
	if isLikelyPrivateHostname(host) && net.ParseIP(host) == nil && !allowPrivate {
		return "", fmt.Errorf("outbound private host %q is not allowed", host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return "", err
		}
		return ip.String(), nil
	}
	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return "", fmt.Errorf("resolve outbound host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("outbound host %q did not resolve", host)
	}
	for _, addr := range addrs {
		if err := validateResolvedOutboundIP(host, addr.IP, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return "", err
		}
	}
	for _, addr := range addrs {
		if addr.IP != nil {
			return addr.IP.String(), nil
		}
	}
	return "", fmt.Errorf("outbound host %q did not resolve to a usable address", host)
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return DialOutboundTCPContext(ctx, host, port, timeout)
}

// OutboundTCPDialContext returns a net/http- and websocket-compatible dial
// function that enforces the outbound policy at the actual TCP connection
// boundary. In particular, it resolves a hostname once, validates every
// returned address, and only then dials a validated literal IP.
func OutboundTCPDialContext(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		switch strings.ToLower(strings.TrimSpace(network)) {
		case "tcp", "tcp4", "tcp6":
		default:
			return nil, fmt.Errorf("outbound network %q is not allowed", network)
		}

		host, portRaw, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid outbound address %q: %w", address, err)
		}
		port, err := strconv.Atoi(portRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid outbound port %q: %w", portRaw, err)
		}
		return DialOutboundTCPContext(ctx, host, port, timeout)
	}
}

// DialOutboundTCPContext resolves a hostname once, validates every returned
// address against outbound policy, and dials a validated literal IP. This
// removes the validation/dial DNS rebinding window.
func DialOutboundTCPContext(ctx context.Context, host string, port int, timeout time.Duration) (net.Conn, error) {
	host = normalizeHostname(host)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}

	enforceAllowlist := parseBoolEnv(envOutboundAllowlistMode, false)
	if enforceAllowlist {
		allowed := false
		for _, pattern := range parseAllowedHostPatterns() {
			if hostMatchesPattern(host, pattern) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("outbound host %q is not allowlisted", host)
		}
	}

	allowPrivate := parseBoolEnv(envOutboundAllowPrivate, false)
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	if strings.EqualFold(host, "localhost") && !allowLoopback {
		return nil, fmt.Errorf("outbound loopback host %q is not allowed", host)
	}
	if isLikelyPrivateHostname(host) && net.ParseIP(host) == nil && !allowPrivate {
		return nil, fmt.Errorf("outbound private host %q is not allowed", host)
	}

	if timeout <= 0 {
		timeout = defaultOutboundTimeout
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	}

	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve outbound host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("outbound host %q did not resolve", host)
	}
	for _, addr := range addrs {
		if err := validateResolvedOutboundIP(host, addr.IP, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, addr := range addrs {
		if addr.IP == nil {
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.IP.String(), strconv.Itoa(port)))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("outbound host %q did not resolve to a dialable address", host)
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
