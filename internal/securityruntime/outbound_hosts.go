package securityruntime

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

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
