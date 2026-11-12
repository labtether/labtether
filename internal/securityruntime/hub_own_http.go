package securityruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// Plain http for the hub's check of its own address. With
// LABTETHER_TLS_MODE=disabled a reverse proxy ends TLS and the hub listens on
// plain http, so its own address is http://. This does not use or change
// LABTETHER_ALLOW_INSECURE_TRANSPORT.
//
// The caller passes only an address the hub was deployed with (an environment
// variable or the built-in default), never one saved in Settings. The address
// must be on this host or a private network on every connection: a name is
// resolved each time and every address it resolves to is checked, so DNS
// cannot point it at a public address later.

// ErrNotHubOwnAddress: the address is not on this host or a private network.
var ErrNotHubOwnAddress = errors.New("plain http to the hub's own address is allowed only on this host or a private network")

// GetHubOwnHTTP sends one GET to the hub's own plain http address. The request
// can reach only the origin of rawURL, carries no credentials, uses no proxy
// and does not follow a redirect to another origin.
func GetHubOwnHTTP(ctx context.Context, rawURL string, timeout time.Duration) (*http.Response, error) {
	parsed, err := validateHubOwnHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- host checked by validateHubOwnHTTPURL; the client re-checks every resolved address on connect.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- hub-own client: one origin, every resolved address checked on connect.
	return newHubOwnHTTPClient(parsed, timeout).Do(req)
}

// validateHubOwnHTTPURL accepts an absolute http:// URL without credentials.
// An address is checked now; a name is checked by what it resolves to on
// every connection. It keeps the outbound allowlist when
// LABTETHER_OUTBOUND_ALLOWLIST_MODE is on.
func validateHubOwnHTTPURL(rawURL string) (*url.URL, error) {
	parsed, err := parsePlainHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	host := normalizeHostname(parsed.Hostname())
	if addr, parseErr := netip.ParseAddr(host); parseErr == nil {
		if err := checkHubOwnAddr(host, addr); err != nil {
			return nil, err
		}
	}
	if parseBoolEnv(envOutboundAllowlistMode, false) {
		// The operator's allowlist, with their private setting; never link-local.
		if err := validateOutboundHostWithPolicy(host, true, localHTTPPrivateAllowed(), true, false); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

// newHubOwnHTTPClient returns a client for the one origin of parsed. It is
// made for one request, so it keeps no idle connection.
func newHubOwnHTTPClient(parsed *url.URL, timeout time.Duration) *http.Client {
	client := newOneOriginHTTPClient(parsed, timeout, resolveHubOwnHost)
	if transport, ok := client.Transport.(*http.Transport); ok {
		transport.DisableKeepAlives = true
	}
	return client
}

// resolveHubOwnHost returns the host's addresses, or an error when any of
// them is not on this host or a private network.
func resolveHubOwnHost(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, _, err := resolveHostAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if err := checkHubOwnAddr(host, addr); err != nil {
			return nil, err
		}
	}
	return addrs, nil
}

// checkHubOwnAddr allows loopback, and a private or tailnet address unless
// the operator set LABTETHER_OUTBOUND_ALLOW_PRIVATE=false. Link-local, cloud
// metadata and every public address are refused.
func checkHubOwnAddr(host string, addr netip.Addr) error {
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return nil
	}
	if !IsLocalNetworkAddr(addr) {
		return fmt.Errorf("%s: %w", normalizeHostname(host), ErrNotHubOwnAddress)
	}
	// The operator's setting is read again on every connection.
	if !localHTTPPrivateAllowed() {
		return fmt.Errorf("outbound private host %q is not allowed", normalizeHostname(host))
	}
	return nil
}
