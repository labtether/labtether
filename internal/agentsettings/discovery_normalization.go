// Package agentsettings contains hub-local copies of agent setting definitions,
// validation logic, and key constants. This allows the hub to manage agent
// settings without depending on the extracted agent packages.
package agentsettings

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// NormalizeDiscoveryPortListValue validates and normalizes a comma-separated port list.
func NormalizeDiscoveryPortListValue(key, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\t':
			return true
		default:
			return false
		}
	})

	if len(fields) == 0 {
		return "", nil
	}

	seen := make(map[int]struct{}, len(fields))
	ports := make([]int, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token == "" {
			continue
		}
		port, err := strconv.Atoi(token)
		if err != nil || port <= 0 || port > 65535 {
			return "", fmt.Errorf("%s must contain only TCP ports between 1 and 65535", key)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}

	sort.Ints(ports)
	items := make([]string, 0, len(ports))
	for _, port := range ports {
		items = append(items, strconv.Itoa(port))
	}
	return strings.Join(items, ","), nil
}

// NormalizeDiscoveryCIDRListValue validates and normalizes a comma-separated CIDR list.
func NormalizeDiscoveryCIDRListValue(key, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\t':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 {
		return "", nil
	}

	seen := make(map[string]struct{}, len(fields))
	cidrs := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if token == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(token)
		if err != nil || !prefix.IsValid() {
			return "", fmt.Errorf("%s must contain valid CIDR values", key)
		}

		addr := prefix.Addr()
		if !addr.Is4() && !addr.Is6() {
			return "", fmt.Errorf("%s only supports IPv4 or IPv6 CIDR values", key)
		}
		if !isPrivateOrLocalCIDR(prefix) {
			return "", fmt.Errorf("%s only allows private/local CIDR ranges", key)
		}

		normalized := prefix.Masked().String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		cidrs = append(cidrs, normalized)
	}

	sort.Strings(cidrs)
	return strings.Join(cidrs, ","), nil
}

func isPrivateOrLocalCIDR(prefix netip.Prefix) bool {
	addr := prefix.Addr()
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() {
		return true
	}
	if addr.Is4() {
		ip := net.ParseIP(addr.String())
		return ip != nil && ip.IsLinkLocalUnicast()
	}
	return addr.Is6() && addr.IsLinkLocalUnicast()
}
