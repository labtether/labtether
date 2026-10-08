package webservice

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/serviceregistry"
	"net/url"
	"strconv"
	"strings"
)

func normalizeLabTetherPresentation(services []agentmgr.DiscoveredWebService) {
	if len(services) == 0 {
		return
	}

	hostHasConsole := make(map[string]bool)

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
			continue
		}

		component := classifyLabTetherComponent(*svc)
		if component == "" {
			continue
		}

		if svc.Metadata == nil {
			svc.Metadata = make(map[string]string)
		}
		svc.Metadata["labtether_component"] = component

		switch component {
		case labtetherConsole:
			svc.Name = "LabTether Console"
			hostHasConsole[strings.TrimSpace(svc.HostAssetID)] = true
			delete(svc.Metadata, "hidden")
		case labtetherAPI:
			svc.Name = "LabTether API"
		}
	}

	for i := range services {
		svc := &services[i]
		if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey || svc.Metadata == nil {
			continue
		}
		if svc.Metadata["labtether_component"] != labtetherAPI {
			continue
		}
		if hostHasConsole[strings.TrimSpace(svc.HostAssetID)] {
			svc.Metadata["hidden"] = "true"
		} else {
			delete(svc.Metadata, "hidden")
		}
	}
}

func classifyLabTetherComponent(svc agentmgr.DiscoveredWebService) string {
	if strings.TrimSpace(svc.ServiceKey) != labtetherServiceKey {
		return ""
	}

	if svc.Metadata != nil {
		switch strings.ToLower(strings.TrimSpace(svc.Metadata["labtether_component"])) {
		case labtetherConsole:
			return labtetherConsole
		case labtetherAPI:
			return labtetherAPI
		}
	}

	port := servicePortFromURL(svc.URL)
	if port == 0 && svc.Metadata != nil {
		port = servicePortFromURL(svc.Metadata["raw_url"])
	}
	if port == 0 && svc.Metadata != nil {
		port = servicePortFromURL(svc.Metadata["backend_url"])
	}

	switch port {
	case 3000:
		return labtetherConsole
	case 8080, 8443:
		return labtetherAPI
	}

	if svc.Metadata != nil {
		switch strings.TrimSpace(svc.Metadata["health_path"]) {
		case "/api/health":
			return labtetherConsole
		case "/healthz", "/version":
			return labtetherAPI
		}
	}

	return ""
}

func servicePortFromURL(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return 0
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		return 0
	}
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return 0
	}
	return value
}

func isHiddenService(svc agentmgr.DiscoveredWebService) bool {
	if svc.Metadata == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(svc.Metadata["hidden"]), "true")
}

// enrichServicesFromRegistry applies the hub's service registry to fill in
// missing category, icon, and name data. This ensures services are correctly
// classified even when reported by agents running older registry versions.
func enrichServicesFromRegistry(services []agentmgr.DiscoveredWebService) {
	for i := range services {
		svc := &services[i]
		// Skip services already classified (have a service key and icon).
		if svc.ServiceKey != "" && svc.IconKey != "" {
			continue
		}
		// Try matching by Docker image first.
		if image := svc.Metadata["image"]; image != "" {
			if known, ok := serviceregistry.LookupByDockerImage(image); ok {
				applyRegistryMatch(svc, known)
				continue
			}
		}
		// Try matching by name, URL domain, or router name hints.
		hints := []string{svc.Name}
		if svc.Metadata != nil {
			if rn := svc.Metadata["router_name"]; rn != "" {
				hints = append(hints, rn)
			}
		}
		if svc.URL != "" {
			hints = append(hints, svc.URL)
		}
		for _, hint := range hints {
			if known, ok := serviceregistry.LookupByHint(hint); ok {
				applyRegistryMatch(svc, known)
				break
			}
		}
	}
}

// applyRegistryMatch fills in missing fields from a registry match without
// overwriting values the agent already set.
func applyRegistryMatch(svc *agentmgr.DiscoveredWebService, known serviceregistry.KnownService) {
	if svc.ServiceKey == "" {
		svc.ServiceKey = known.Key
	}
	if svc.IconKey == "" {
		svc.IconKey = known.IconKey
	}
	if svc.Category == "" || svc.Category == serviceregistry.CatOther {
		svc.Category = known.Category
	}
	// Only update name if it looks auto-generated (e.g. "Port 8080" or container name).
	if svc.Name == "" {
		svc.Name = known.Name
	}
}
