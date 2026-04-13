package protocols

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

var lookupManualDeviceIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}
var dialManualDeviceTCPContext = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	var dialer net.Dialer
	if timeout > 0 {
		dialer.Timeout = timeout
	}
	return dialer.DialContext(ctx, network, address)
}

// ValidateManualDeviceHost validates a host for manual device use.
// It permits LAN/private addresses but rejects loopback and other
// non-routable/surprising addresses that would break the direct-proxy trust
// boundary.
func ValidateManualDeviceHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if len(host) > 253 {
		return fmt.Errorf("host too long (max 253 characters)")
	}
	if strings.ContainsAny(host, " \t\n\r") {
		return fmt.Errorf("host contains whitespace")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("loopback host %q is not allowed", host)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil // hostname — DNS resolves at connect time
	}
	return validateManualDeviceIP(ip, host)
}

// DialManualDeviceTCPTimeout resolves and dials a manual-device endpoint while
// enforcing manual-device host policy on both literal IPs and DNS results.
func DialManualDeviceTCPTimeout(ctx context.Context, host string, port int, timeout time.Duration) (net.Conn, error) {
	host = strings.TrimSpace(host)
	if err := ValidateManualDeviceHost(host); err != nil {
		return nil, err
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}

	if ip := net.ParseIP(host); ip != nil {
		return dialManualDeviceAddress(ctx, ip.String(), port, timeout)
	}

	addrs, err := lookupManualDeviceIPAddrs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve host %q: no IP addresses found", host)
	}

	lastErr := error(nil)
	for _, addr := range addrs {
		if err := validateManualDeviceIP(addr.IP, addr.IP.String()); err != nil {
			return nil, fmt.Errorf("host %q resolved to disallowed address %q: %w", host, addr.IP.String(), err)
		}
		conn, dialErr := dialManualDeviceAddress(ctx, addr.IP.String(), port, timeout)
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no connection attempts made")
	}
	return nil, fmt.Errorf("connect host %q: %w", host, lastErr)
}

func validateManualDeviceIP(ip net.IP, host string) error {
	if ip == nil {
		return fmt.Errorf("invalid IP address %q", host)
	}
	if ip.IsLoopback() {
		return fmt.Errorf("loopback address %q is not allowed", host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("unspecified address %q is not allowed", host)
	}
	if ip.Equal(net.IPv4bcast) {
		return fmt.Errorf("broadcast address %q is not allowed", host)
	}
	if ip.IsMulticast() {
		return fmt.Errorf("multicast address %q is not allowed", host)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local address %q is not allowed", host)
	}
	return nil
}

func dialManualDeviceAddress(ctx context.Context, host string, port int, timeout time.Duration) (net.Conn, error) {
	return dialManualDeviceTCPContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
}
