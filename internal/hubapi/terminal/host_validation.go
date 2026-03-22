package terminal

import (
	"fmt"
	"net"
	"strings"
)

// ValidateQuickConnectHost validates a hostname or IP for Quick Connect.
// Rejects empty, too-long, and SSRF-sensitive addresses (link-local, broadcast, all-zeros).
func ValidateQuickConnectHost(host string) error {
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
		return fmt.Errorf("host %q is not allowed (loopback)", host)
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() {
			return fmt.Errorf("host %q is not allowed (loopback address)", host)
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("host %q is not allowed (unspecified address)", host)
		}
		if ip.Equal(net.IPv4bcast) {
			return fmt.Errorf("host %q is not allowed (broadcast address)", host)
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("host %q is not allowed (link-local address)", host)
		}
	}

	return nil
}
