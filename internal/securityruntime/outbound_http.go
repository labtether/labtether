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
