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
	return ip.IsPrivate()
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

func secureOutboundHTTPClient(base *http.Client, scheme string) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: defaultOutboundTimeout}
	}
	client := *base
	if client.Timeout <= 0 {
		client.Timeout = defaultOutboundTimeout
	}
	client.Transport = secureOutboundRoundTripper(base.Transport, scheme)
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

func secureOutboundRoundTripper(base http.RoundTripper, scheme string) http.RoundTripper {
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
	allowPrivate := resolvedOutboundAllowPrivate(scheme)
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialOutboundValidated(ctx, network, address, allowLoopback, allowPrivate, allowLinkLocal)
	}
	return transport
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

func dialOutboundValidated(ctx context.Context, network, address string, allowLoopback, allowPrivate, allowLinkLocal bool) (net.Conn, error) {
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
		if err := validateResolvedOutboundIP(host, ip, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	addrs, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host %q did not resolve", host)
	}
	for _, addr := range addrs {
		if err := validateResolvedOutboundIP(host, addr.IP, allowLoopback, allowPrivate, allowLinkLocal); err != nil {
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

	allowPrivate := parseBoolEnv(envOutboundAllowPrivate, false)
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
	allowPrivate := parseBoolEnv(envOutboundAllowPrivate, false)
	allowLoopback := parseBoolEnv(envOutboundAllowLoopback, false)
	allowLinkLocal := parseBoolEnv(envOutboundAllowLinkLocal, false)
	return map[string]string{
		"allowlist_mode":           strconv.FormatBool(allowlistMode),
		"allow_private":            strconv.FormatBool(allowPrivate),
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
