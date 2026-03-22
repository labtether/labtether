package collectors

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assets"
)

var identityIPv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

func CollectCollectorIdentity(asset assets.Asset) CollectorIdentity {
	identity := CollectorIdentity{
		IPs: make(map[string]struct{}, 4),
		Hostnames: make(map[string]struct{}, 4),
	}

	addIdentityHostname(identity.Hostnames, asset.Name)
	for key, value := range asset.Metadata {
		collectIdentityFromMetadata(identity, key, value)
	}
	return identity
}

func collectIdentityFromMetadata(identity CollectorIdentity, rawKey, rawValue string) {
	key := strings.ToLower(strings.TrimSpace(rawKey))
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return
	}

	for _, ip := range extractIdentityIPv4(value) {
		if isUsableIdentityIP(ip) {
			identity.IPs[ip] = struct{}{}
		}
	}

	if strings.Contains(key, "url") || strings.Contains(key, "endpoint") || strings.Contains(key, "base_url") || strings.Contains(key, "address") || strings.Contains(key, "host") {
		host, ip := CollectorEndpointIdentity(value)
		if host != "" {
			addIdentityHostname(identity.Hostnames, host)
		}
		if ip != "" && isUsableIdentityIP(ip) {
			identity.IPs[ip] = struct{}{}
		}
	}

	if strings.Contains(key, "hostname") || key == "host" || strings.HasSuffix(key, "_host") || strings.Contains(key, "dns") || strings.Contains(key, "name") {
		addIdentityHostname(identity.Hostnames, value)
	}
}

func addIdentityHostname(target map[string]struct{}, raw string) {
	normalized := normalizeIdentityHostname(raw)
	if normalized == "" || normalized == "localhost" {
		return
	}
	target[normalized] = struct{}{}
}

func normalizeIdentityHostname(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}

	if host, _ := CollectorEndpointIdentity(value); host != "" {
		value = host
	}
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}
	if strings.Contains(value, ":") {
		if parts := strings.Split(value, ":"); len(parts) == 2 {
			value = parts[0]
		}
	}
	value = strings.TrimRight(value, ".")
	if value == "" {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '-' {
			continue
		}
		return ""
	}
	return value
}

func extractIdentityIPv4(text string) []string {
	matches := identityIPv4Pattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	out := make([]string, 0, len(matches))
	for _, match := range matches {
		parts := strings.Split(match, ".")
		if len(parts) != 4 {
			continue
		}
		valid := true
		for _, part := range parts {
			num, err := strconv.Atoi(part)
			if err != nil || num < 0 || num > 255 {
				valid = false
				break
			}
		}
		if valid {
			out = append(out, match)
		}
	}
	return out
}

func isUsableIdentityIP(ip string) bool {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return false
	}
	if trimmed == "0.0.0.0" {
		return false
	}
	if strings.HasPrefix(trimmed, "127.") {
		return false
	}
	if strings.HasPrefix(trimmed, "169.254.") {
		return false
	}
	return true
}
