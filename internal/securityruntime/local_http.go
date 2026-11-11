package securityruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Plain http for one connection that its owner opted in to, on the local
// network only (Home Assistant's "Allow http on my network"). This does not
// use or change LABTETHER_ALLOW_INSECURE_TRANSPORT. The address must be on the
// local network when it is saved and on every connection: a name is resolved
// each time and every address it resolves to must be local, so DNS cannot
// point it at a public address later.

// ErrNotLocalNetworkAddress: the address is not on the local network.
var ErrNotLocalNetworkAddress = errors.New("plain http is allowed only to an address on your own network")

// tailnetPrefix is the CGNAT range Tailscale uses for its IPv4 addresses.
var tailnetPrefix = sharedAddressSpacePrefix

var localNetworkNameSuffixes = []string{".local", ".ts.net"}

// IsLocalNetworkAddr reports whether addr is a private, tailnet or link-local
// address. Loopback and every public address are not.
func IsLocalNetworkAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	// IsPrivate: the RFC 1918 ranges and IPv6 unique-local fc00::/7 (Tailscale's
	// IPv6 too); IsLinkLocalUnicast: 169.254.0.0/16 and fe80::/10.
	return addr.IsPrivate() || addr.IsLinkLocalUnicast() || tailnetPrefix.Contains(addr)
}

// IsLocalNetworkHost reports whether host may be offered plain http: a local
// network address, or a name ending in .local or .ts.net. A name is still
// checked by what it resolves to on every connection.
func IsLocalNetworkHost(host string) bool {
	host = normalizeHostname(host)
	if addr, err := netip.ParseAddr(host); err == nil {
		return IsLocalNetworkAddr(addr)
	}
	for _, suffix := range localNetworkNameSuffixes {
		if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}

// ValidateLocalHTTPURL accepts an absolute http:// URL without credentials
// whose host is on the local network (see IsLocalNetworkHost). It also keeps
// the outbound allowlist when LABTETHER_OUTBOUND_ALLOWLIST_MODE is on.
func ValidateLocalHTTPURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid url")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("url must not contain embedded credentials")
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return nil, fmt.Errorf("url scheme %q is not plain http", parsed.Scheme)
	}
	if !IsLocalNetworkHost(parsed.Hostname()) {
		return nil, ErrNotLocalNetworkAddress
	}
	if parseBoolEnv(envOutboundAllowlistMode, false) {
		if err := validateOutboundHostWithPolicy(parsed.Hostname(), true, true, false, true); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

// NewLocalHTTPClient returns a client for the one origin of rawURL. Each
// connection resolves the host again, refuses it unless every address is on
// the local network, and connects to a checked address; redirects may not
// leave the origin.
func NewLocalHTTPClient(rawURL string, timeout time.Duration) (*http.Client, error) {
	parsed, err := ValidateLocalHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	origin := localHTTPOrigin(parsed)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           localNetworkDialer(dialer, parsed.Hostname(), origin),
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL == nil || localHTTPOrigin(req.URL) != origin {
				return fmt.Errorf("redirect away from %s is not allowed", origin)
			}
			return nil
		},
	}, nil
}

func localHTTPOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
	}
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(normalizeHostname(u.Hostname()), port)
}

func localNetworkDialer(dialer *net.Dialer, wantHost, origin string) func(context.Context, string, string) (net.Conn, error) {
	wantHost = normalizeHostname(wantHost)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || normalizeHostname(host) != wantHost || "http://"+net.JoinHostPort(wantHost, port) != origin {
			return nil, fmt.Errorf("connection to %s is not allowed by this client", address)
		}
		addrs, err := resolveLocalNetworkHost(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, addr := range addrs {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
}

// resolveLocalNetworkHost returns the host's addresses, or an error when any
// of them is not on the local network.
func resolveLocalNetworkHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(normalizeHostname(host)); err == nil {
		if !IsLocalNetworkAddr(addr) {
			return nil, ErrNotLocalNetworkAddress
		}
		return []netip.Addr{addr.Unmap()}, nil
	}
	resolved, err := lookupIPAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("%s did not resolve", host)
	}
	addrs := make([]netip.Addr, 0, len(resolved))
	for _, ipAddr := range resolved {
		addr, ok := netip.AddrFromSlice(ipAddr.IP)
		if !ok || !IsLocalNetworkAddr(addr) {
			return nil, fmt.Errorf("%s resolves to an address outside your network: %w", host, ErrNotLocalNetworkAddress)
		}
		addrs = append(addrs, addr.Unmap())
	}
	return addrs, nil
}
