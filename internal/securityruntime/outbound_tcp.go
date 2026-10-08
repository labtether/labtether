package securityruntime

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

var lookupDialIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return lookupIPAddrs(ctx, host)
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
