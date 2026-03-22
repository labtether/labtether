package collectors

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

var (
	proxmoxMACPattern  = regexp.MustCompile(`(?i)(?:[0-9a-f]{2}:){5}[0-9a-f]{2}`)
	proxmoxUUIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

func ProxmoxGuestIdentityMetadataFromConfig(config map[string]any) map[string]string {
	if len(config) == 0 {
		return nil
	}

	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	guestUUID := ""
	guestHostname := ""
	guestIPs := make([]string, 0, 4)
	guestMACs := make([]string, 0, 4)
	seenIP := make(map[string]struct{}, 4)
	seenMAC := make(map[string]struct{}, 4)

	addIP := func(value string) {
		normalized := proxmoxNormalizeIP(value)
		if normalized == "" {
			return
		}
		if _, exists := seenIP[normalized]; exists {
			return
		}
		seenIP[normalized] = struct{}{}
		guestIPs = append(guestIPs, normalized)
	}
	addMAC := func(value string) {
		normalized := proxmoxNormalizeMAC(value)
		if normalized == "" {
			return
		}
		if _, exists := seenMAC[normalized]; exists {
			return
		}
		seenMAC[normalized] = struct{}{}
		guestMACs = append(guestMACs, normalized)
	}

	for _, key := range keys {
		rawValue := strings.TrimSpace(fmt.Sprintf("%v", config[key]))
		if rawValue == "" || rawValue == "<nil>" {
			continue
		}
		lowerKey := strings.ToLower(strings.TrimSpace(key))

		if lowerKey == "hostname" && guestHostname == "" {
			guestHostname = strings.TrimSpace(rawValue)
		}
		if guestUUID == "" && (lowerKey == "smbios1" || lowerKey == "vmgenid" || strings.Contains(lowerKey, "uuid")) {
			guestUUID = proxmoxExtractUUID(rawValue)
		}

		for _, ip := range proxmoxExtractIPsFromConfigValue(lowerKey, rawValue) {
			addIP(ip)
		}
		for _, mac := range proxmoxExtractMACsFromConfigValue(lowerKey, rawValue) {
			addMAC(mac)
		}
	}

	metadata := map[string]string{}
	if guestUUID != "" {
		metadata["guest_uuid"] = guestUUID
	}
	if guestHostname != "" {
		metadata["guest_hostname"] = guestHostname
	}
	if len(guestIPs) > 0 {
		metadata["guest_ips"] = strings.Join(guestIPs, ",")
		metadata["guest_primary_ip"] = guestIPs[0]
	}
	if len(guestMACs) > 0 {
		metadata["guest_mac_addresses"] = strings.Join(guestMACs, ",")
		metadata["guest_primary_mac"] = guestMACs[0]
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func proxmoxExtractIPsFromConfigValue(key, value string) []string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if !(strings.HasPrefix(lowerKey, "ipconfig") || strings.HasPrefix(lowerKey, "net") || lowerKey == "ip" || lowerKey == "ip6") {
		return nil
	}

	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	tokens := strings.Split(value, ",")
	for _, token := range tokens {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) == 1 {
			if lowerKey == "ip" || lowerKey == "ip6" || strings.HasPrefix(lowerKey, "ipconfig") {
				if normalized := proxmoxNormalizeIP(pair[0]); normalized != "" {
					if _, exists := seen[normalized]; !exists {
						seen[normalized] = struct{}{}
						out = append(out, normalized)
					}
				}
			}
			continue
		}

		tokenKey := strings.ToLower(strings.TrimSpace(pair[0]))
		tokenValue := strings.TrimSpace(pair[1])
		if tokenKey != "ip" && tokenKey != "ip6" && tokenKey != "address" && tokenKey != "addr" {
			continue
		}
		if normalized := proxmoxNormalizeIP(tokenValue); normalized != "" {
			if _, exists := seen[normalized]; !exists {
				seen[normalized] = struct{}{}
				out = append(out, normalized)
			}
		}
	}
	return out
}

func proxmoxExtractMACsFromConfigValue(key, value string) []string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if !(strings.HasPrefix(lowerKey, "net") || strings.Contains(lowerKey, "mac") || strings.Contains(lowerKey, "hwaddr")) {
		return nil
	}

	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, match := range proxmoxMACPattern.FindAllString(value, -1) {
		normalized := proxmoxNormalizeMAC(match)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func proxmoxExtractUUID(value string) string {
	for _, token := range strings.Split(value, ",") {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) != 2 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(pair[0])) != "uuid" {
			continue
		}
		if normalized := strings.ToLower(strings.TrimSpace(pair[1])); proxmoxUUIDPattern.MatchString(normalized) {
			return proxmoxUUIDPattern.FindString(normalized)
		}
	}

	match := proxmoxUUIDPattern.FindString(strings.ToLower(strings.TrimSpace(value)))
	if match == "" {
		return ""
	}
	return match
}

func proxmoxNormalizeIP(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "dhcp", "auto", "none", "manual":
		return ""
	}
	if slash := strings.Index(trimmed, "/"); slash >= 0 {
		trimmed = strings.TrimSpace(trimmed[:slash])
	}
	if trimmed == "" {
		return ""
	}

	parsed := net.ParseIP(trimmed)
	if parsed == nil {
		return ""
	}
	if ipv4 := parsed.To4(); ipv4 != nil {
		normalized := ipv4.String()
		if !isUsableIdentityIP(normalized) {
			return ""
		}
		return normalized
	}
	return parsed.String()
}

func proxmoxNormalizeMAC(value string) string {
	match := proxmoxMACPattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return ""
	}
	return strings.ToUpper(match)
}
