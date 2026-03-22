package protocols

import (
	"fmt"
	"net"
	"strings"
)

// ValidateManualDeviceHost validates a host for manual device use.
// Allows RFC-1918 private ranges (homelab LAN). Blocks loopback,
// link-local, and cloud metadata endpoints.
func ValidateManualDeviceHost(host string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("loopback host %q is not allowed", host)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil // hostname — DNS resolves at connect time
	}
	if ip.IsLoopback() {
		return fmt.Errorf("loopback address %q is not allowed", host)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("link-local address %q is not allowed", host)
	}
	return nil
}
