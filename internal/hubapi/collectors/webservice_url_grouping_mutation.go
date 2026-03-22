package collectors

import (
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

func groupDiscoveredWebServiceURLs(base, incoming agentmgr.DiscoveredWebService) agentmgr.DiscoveredWebService {
	if base.Metadata == nil {
		base.Metadata = make(map[string]string)
	}
	if incoming.Metadata == nil {
		incoming.Metadata = map[string]string{}
	}

	base.URL, base.Metadata["alt_urls"] = groupPrimaryURLAndAliases(base.URL, base.Metadata["alt_urls"], incoming.URL)
	base.Metadata["alt_urls"] = groupAliasCSV(base.Metadata["alt_urls"], incoming.Metadata["alt_urls"])

	if strings.TrimSpace(base.ContainerID) == "" {
		base.ContainerID = strings.TrimSpace(incoming.ContainerID)
	}
	if strings.TrimSpace(base.ServiceUnit) == "" {
		base.ServiceUnit = strings.TrimSpace(incoming.ServiceUnit)
	}
	if strings.TrimSpace(base.IconKey) == "" {
		base.IconKey = strings.TrimSpace(incoming.IconKey)
	}
	if strings.TrimSpace(base.ServiceKey) == "" {
		base.ServiceKey = strings.TrimSpace(incoming.ServiceKey)
	}
	if strings.TrimSpace(base.Name) == "" {
		base.Name = strings.TrimSpace(incoming.Name)
	}
	if strings.TrimSpace(base.Category) == "" {
		base.Category = strings.TrimSpace(incoming.Category)
	}
	if strings.TrimSpace(base.Source) == "" {
		base.Source = strings.TrimSpace(incoming.Source)
	}

	groupMetadataField(base.Metadata, incoming.Metadata, "backend_url")
	groupMetadataField(base.Metadata, incoming.Metadata, "raw_url")
	groupMetadataField(base.Metadata, incoming.Metadata, "router_name")
	groupMetadataField(base.Metadata, incoming.Metadata, "proxy_provider")
	groupMetadataField(base.Metadata, incoming.Metadata, "public_port")
	groupMetadataField(base.Metadata, incoming.Metadata, "private_port")

	base.Status = groupServiceStatus(base.Status, incoming.Status)
	if base.ResponseMs <= 0 || (incoming.ResponseMs > 0 && incoming.ResponseMs < base.ResponseMs) {
		base.ResponseMs = incoming.ResponseMs
	}

	return base
}

func groupPrimaryURLAndAliases(primary, aliases, incoming string) (string, string) {
	primary = strings.TrimSpace(primary)
	incoming = strings.TrimSpace(incoming)
	if primary == "" {
		return incoming, aliases
	}
	if incoming == "" || strings.EqualFold(primary, incoming) {
		return primary, aliases
	}
	return primary, appendUniqueAlias(aliases, incoming)
}

func groupAliasCSV(existing, incoming string) string {
	out := existing
	for _, alias := range splitAliasCSV(incoming) {
		out = appendUniqueAlias(out, alias)
	}
	return out
}

func splitAliasCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		alias := strings.TrimSpace(part)
		if alias == "" {
			continue
		}
		out = append(out, alias)
	}
	return out
}

func appendUniqueAlias(existing, alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return strings.TrimSpace(existing)
	}

	items := splitAliasCSV(existing)
	seen := make(map[string]struct{}, len(items)+1)
	normalized := make([]string, 0, len(items)+1)
	for _, item := range items {
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}

	key := strings.ToLower(alias)
	if _, ok := seen[key]; !ok {
		normalized = append(normalized, alias)
	}
	return strings.Join(normalized, ",")
}

func groupMetadataField(base, incoming map[string]string, key string) {
	if strings.TrimSpace(base[key]) != "" {
		return
	}
	if incoming == nil {
		return
	}
	if value := strings.TrimSpace(incoming[key]); value != "" {
		base[key] = value
	}
}

func groupServiceStatus(existing, incoming string) string {
	rank := func(status string) int {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "up":
			return 3
		case "unknown":
			return 2
		case "down":
			return 1
		default:
			return 0
		}
	}

	if rank(incoming) > rank(existing) {
		return incoming
	}
	return existing
}

func cloneWebServiceForGrouping(svc agentmgr.DiscoveredWebService) agentmgr.DiscoveredWebService {
	cloned := svc
	if svc.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(svc.Metadata))
		for key, value := range svc.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}
