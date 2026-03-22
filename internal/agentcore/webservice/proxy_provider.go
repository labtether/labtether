package webservice

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	proxypkg "github.com/labtether/labtether/internal/agentcore/proxy"
	"github.com/labtether/labtether/internal/agentmgr"
)

// ---------------------------------------------------------------------------
// ProxyRoute and ProxyProvider — type aliases to proxy/ subpackage
// ---------------------------------------------------------------------------

// ProxyRoute is an alias for proxy.Route.
type ProxyRoute = proxypkg.Route

// ProxyProvider is an alias for proxy.Provider.
type ProxyProvider = proxypkg.Provider

// findHostPort is an alias for proxy.FindHostPort.
var findHostPort = proxypkg.FindHostPort

// ---------------------------------------------------------------------------
// containerPortEntry — index helper for port bridging
// ---------------------------------------------------------------------------

// containerPortEntry maps a container ID to its host-mapped public port
// for a given private port.
type containerPortEntry struct {
	containerID string
	publicPort  int
}

// ---------------------------------------------------------------------------
// enrichServicesWithRoutes — core enrichment function
// ---------------------------------------------------------------------------

// enrichServicesWithRoutes correlates proxy routes with discovered services,
// enriching matched services with their proxied URL and creating proxy-only
// services for unmatched routes.
//
// The function uses three matching strategies:
//   - Strategy 0 (Traefik-specific): router label correlation
//     (route.RouterName -> traefik.http.routers.<name>.* label -> ContainerID).
//   - Strategy 1 (primary): Parse backend URL port -> look up containers with
//     that PrivatePort -> find service by ContainerID. This handles Docker port
//     bridging (e.g. container internal 80 -> host 8085).
//   - Strategy 2 (fallback): Direct port match (backend port == service URL port)
//     for non-Docker backends or when container info is unavailable.
func enrichServicesWithRoutes(
	services []agentmgr.DiscoveredWebService,
	routes []ProxyRoute,
	providerName string,
	hostAssetID string,
	hostIP string,
	containers []dockerpkg.DockerContainer,
) []agentmgr.DiscoveredWebService {
	if len(routes) == 0 {
		return services
	}

	isTraefik := strings.EqualFold(providerName, "traefik")

	// Build privatePort -> []containerPortEntry index from containers.
	// This maps each container-internal port to the containers that expose it
	// and their host-mapped public ports.
	privatePortIndex := make(map[int][]containerPortEntry)
	for _, c := range containers {
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				privatePortIndex[p.PrivatePort] = append(privatePortIndex[p.PrivatePort], containerPortEntry{
					containerID: c.ID,
					publicPort:  p.PublicPort,
				})
			}
		}
	}

	// Build containerID -> service index map.
	containerToSvc := make(map[string]int)
	for i, svc := range services {
		if svc.ContainerID != "" {
			containerToSvc[svc.ContainerID] = i
		}
	}

	// Traefik-specific router name -> service index map, used to disambiguate
	// common backend ports (for example many containers on PrivatePort 80).
	routerToSvc := make(map[string]int)
	if isTraefik {
		routerToSvc = buildTraefikRouterServiceIndex(containers, containerToSvc)
	}

	// Build publicPort -> service index map (for fallback matching).
	// Track ambiguous ports (multiple services on same port) to avoid
	// false matches in Strategy 2.
	publicPortToSvc := make(map[int]int)
	ambiguousPorts := make(map[int]bool)
	for i, svc := range services {
		if p := portFromURL(svc.URL); p > 0 {
			if _, exists := publicPortToSvc[p]; exists {
				ambiguousPorts[p] = true
			}
			publicPortToSvc[p] = i
		}
	}

	// Track which service indices have been enriched, to handle multi-domain.
	enriched := make(map[int]bool)

	// Traefik-specific dedup map: canonical backend identity -> service index.
	// This prevents duplicate records when multiple routed URLs target the same
	// backend and keeps secondary URLs in alt_urls.
	traefikBackendToSvc := make(map[string]int)

	// Process each route.
	for _, route := range routes {
		if route.Domain == "" {
			continue
		}

		proxiedURL := buildProxiedURL(route.Domain, route.TLS)
		backendPort := portFromURL(route.BackendURL)
		matched := false
		matchedSvcIdx := -1
		backendKey := ""

		if isTraefik {
			backendKey = canonicalBackendIdentity(route.BackendURL)
			if backendKey != "" {
				if svcIdx, ok := traefikBackendToSvc[backendKey]; ok {
					applyEnrichment(&services[svcIdx], proxiedURL, providerName, enriched, svcIdx)
					continue
				}
			}
		}

		// Strategy 0: Traefik router label correlation.
		// This allows deterministic matching even when backend ports are shared
		// by many containers or backend URL has no explicit port.
		if routerKey := normalizeRouterKey(route.RouterName); routerKey != "" {
			if svcIdx, ok := routerToSvc[routerKey]; ok {
				applyEnrichment(&services[svcIdx], proxiedURL, providerName, enriched, svcIdx)
				matched = true
				matchedSvcIdx = svcIdx
			}
		}

		// Strategy 1: Private port bridging via container lookup.
		if !matched && backendPort > 0 {
			if entries, ok := privatePortIndex[backendPort]; ok {
				for _, entry := range entries {
					if svcIdx, found := containerToSvc[entry.containerID]; found {
						applyEnrichment(&services[svcIdx], proxiedURL, providerName, enriched, svcIdx)
						matched = true
						matchedSvcIdx = svcIdx
						break
					}
				}
			}
		}

		// Strategy 2: Direct port match (fallback for non-Docker backends).
		// Skip ambiguous ports where multiple services share the same port.
		if !matched && backendPort > 0 && !ambiguousPorts[backendPort] {
			if svcIdx, found := publicPortToSvc[backendPort]; found {
				applyEnrichment(&services[svcIdx], proxiedURL, providerName, enriched, svcIdx)
				matched = true
				matchedSvcIdx = svcIdx
			}
		}

		// No match: create a proxy-only service.
		if !matched {
			proxyOnly := buildProxyOnlyService(route, providerName, hostAssetID, hostIP)
			services = append(services, proxyOnly)
			matchedSvcIdx = len(services) - 1
		}

		if isTraefik && backendKey != "" && matchedSvcIdx >= 0 {
			traefikBackendToSvc[backendKey] = matchedSvcIdx
		}
	}

	return services
}

// ---------------------------------------------------------------------------
// applyEnrichment — enrich a service with proxy info
// ---------------------------------------------------------------------------

// applyEnrichment handles enriching a discovered service with proxy route info.
// It handles three cases:
//   - First enrichment by any provider: saves original URL to raw_url, sets
//     proxy_provider, replaces URL with proxied URL.
//   - Additional domain from same provider: appends to alt_urls.
//   - Already enriched service (same or different provider): appends to
//     alt_urls only (does not overwrite URL or raw_url).
func applyEnrichment(svc *agentmgr.DiscoveredWebService, proxiedURL, providerName string, enriched map[int]bool, idx int) {
	if svc.Metadata == nil {
		svc.Metadata = make(map[string]string)
	}

	existingProvider := svc.Metadata["proxy_provider"]

	if existingProvider == "" {
		// First enrichment by any provider.
		svc.Metadata["raw_url"] = svc.URL
		svc.Metadata["proxy_provider"] = providerName
		svc.URL = proxiedURL
		enriched[idx] = true
	} else if existingProvider == providerName && enriched[idx] {
		// Additional domain from same provider — append to alt_urls.
		appendServiceAliasURL(svc, proxiedURL)
	} else {
		// Already enriched service — alt_urls only.
		appendServiceAliasURL(svc, proxiedURL)
	}
}

// ---------------------------------------------------------------------------
// buildProxyOnlyService — create a service for unmatched proxy routes
// ---------------------------------------------------------------------------

// buildProxyOnlyService creates a new DiscoveredWebService for a proxy route
// that couldn't be matched to an existing Docker-discovered service. It uses
// the service registry to attempt matching by unique backend port for serviceKey,
// category, and iconKey.
func buildProxyOnlyService(route ProxyRoute, providerName, hostAssetID, hostIP string) agentmgr.DiscoveredWebService {
	proxiedURL := buildProxiedURL(route.Domain, route.TLS)
	name := domainToName(route.Domain)
	serviceKey := ""
	category := CatOther
	iconKey := ""

	// Try to identify the service by backend port.
	if backendPort := portFromURL(route.BackendURL); backendPort > 0 {
		if known, ok := LookupUniqueByPort(backendPort); ok {
			serviceKey = known.Key
			name = known.Name
			category = known.Category
			iconKey = known.IconKey
		}
	}

	// If port-based inference is ambiguous/unavailable, fallback to domain/router hints.
	if serviceKey == "" {
		if known, ok := LookupByHint(route.Domain); ok {
			serviceKey = known.Key
			name = known.Name
			category = known.Category
			iconKey = known.IconKey
		} else if known, ok := LookupByHint(route.RouterName); ok {
			serviceKey = known.Key
			name = known.Name
			category = known.Category
			iconKey = known.IconKey
		}
	}

	idSeed := hostAssetID
	if idSeed == "" {
		idSeed = hostIP
	}
	id := makeServiceID(idSeed, "proxy", route.Domain)

	svc := agentmgr.DiscoveredWebService{
		ID:          id,
		ServiceKey:  serviceKey,
		Name:        name,
		Category:    category,
		URL:         proxiedURL,
		Source:      "proxy",
		HostAssetID: hostAssetID,
		IconKey:     iconKey,
		Metadata: map[string]string{
			"proxy_provider": providerName,
			"router_name":    route.RouterName,
		},
	}

	if route.BackendURL != "" {
		svc.Metadata["backend_url"] = route.BackendURL
		svc.Metadata["raw_url"] = route.BackendURL // enables health-check DNS fallback
	}

	return svc
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// buildProxiedURL constructs the user-facing URL from a domain and TLS flag.
func buildProxiedURL(domain string, tls bool) string {
	if tls {
		return "https://" + domain
	}
	return "http://" + domain
}

// portFromURL extracts the explicit port number from a URL string.
// Returns 0 if no explicit port is present — does NOT infer 80/443 from scheme,
// because implicit ports cause false matches in the port-to-service index.
func portFromURL(rawURL string) int {
	if rawURL == "" {
		return 0
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	if parsed.Host == "" {
		return 0
	}

	// Only return explicitly specified ports.
	_, portStr, err := splitHostPort(parsed.Host)
	if err == nil && portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			return p
		}
	}

	return 0
}

// splitHostPort splits a host:port string, handling IPv6 bracket notation.
// This is a lightweight version that doesn't require a valid port number.
func splitHostPort(hostport string) (host, port string, err error) {
	if hostport == "" {
		return "", "", fmt.Errorf("empty hostport")
	}

	// Handle [ipv6]:port
	if strings.HasPrefix(hostport, "[") {
		end := strings.LastIndex(hostport, "]")
		if end < 0 {
			return "", "", fmt.Errorf("missing closing bracket")
		}
		host = hostport[1:end]
		rest := hostport[end+1:]
		if rest == "" {
			return host, "", nil
		}
		if rest[0] != ':' {
			return "", "", fmt.Errorf("unexpected character after bracket")
		}
		return host, rest[1:], nil
	}

	// Handle host:port (with at most one colon for port detection).
	lastColon := strings.LastIndex(hostport, ":")
	if lastColon < 0 {
		return hostport, "", nil
	}
	return hostport[:lastColon], hostport[lastColon+1:], nil
}

// appendAltURL inserts a URL into a comma-separated list of alternate URLs,
// preserving order and suppressing duplicates.
func appendAltURL(existing, newURL string) string {
	newURL = strings.TrimSpace(newURL)
	if newURL == "" {
		return strings.TrimSpace(existing)
	}

	parts := strings.Split(existing, ",")
	normalized := make([]string, 0, len(parts)+1)
	seen := make(map[string]struct{}, len(parts)+1)

	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, candidate)
	}

	key := strings.ToLower(newURL)
	if _, exists := seen[key]; !exists {
		normalized = append(normalized, newURL)
	}

	return strings.Join(normalized, ",")
}

func appendServiceAliasURL(svc *agentmgr.DiscoveredWebService, proxiedURL string) {
	if strings.EqualFold(strings.TrimSpace(svc.URL), strings.TrimSpace(proxiedURL)) {
		return
	}
	svc.Metadata["alt_urls"] = appendAltURL(svc.Metadata["alt_urls"], proxiedURL)
}

func canonicalBackendIdentity(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return ""
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "" {
		scheme = "http"
	}

	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}

	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = "/"
	}

	if port != "" {
		return scheme + "://" + host + ":" + port + path
	}
	return scheme + "://" + host + path
}

const traefikRouterLabelPrefix = "traefik.http.routers."

func normalizeRouterKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// buildTraefikRouterServiceIndex builds a routerName -> service index map for
// routers that can be uniquely tied to a single discovered service.
func buildTraefikRouterServiceIndex(containers []dockerpkg.DockerContainer, containerToSvc map[string]int) map[string]int {
	candidates := make(map[string]map[int]struct{})

	for _, c := range containers {
		svcIdx, ok := containerToSvc[c.ID]
		if !ok {
			continue
		}
		for routerName := range traefikRoutersFromLabels(c.Labels) {
			if _, exists := candidates[routerName]; !exists {
				candidates[routerName] = make(map[int]struct{})
			}
			candidates[routerName][svcIdx] = struct{}{}
		}
	}

	out := make(map[string]int)
	for routerName, svcSet := range candidates {
		if len(svcSet) != 1 {
			continue
		}
		for svcIdx := range svcSet {
			out[routerName] = svcIdx
		}
	}
	return out
}

// traefikRoutersFromLabels extracts distinct Traefik HTTP router names from
// container labels, normalized to lower-case.
func traefikRoutersFromLabels(labels map[string]string) map[string]struct{} {
	out := make(map[string]struct{})
	if len(labels) == 0 {
		return out
	}
	for key := range labels {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if !strings.HasPrefix(lowerKey, traefikRouterLabelPrefix) {
			continue
		}
		remainder := strings.TrimPrefix(lowerKey, traefikRouterLabelPrefix)
		dot := strings.IndexByte(remainder, '.')
		if dot <= 0 {
			continue
		}
		router := strings.TrimSpace(remainder[:dot])
		if router == "" {
			continue
		}
		out[router] = struct{}{}
	}
	return out
}

// domainToName extracts a human-readable name from a domain by taking the
// first label (e.g. "plex.home.lab" -> "plex").
func domainToName(domain string) string {
	if domain == "" {
		return ""
	}
	parts := strings.SplitN(domain, ".", 2)
	return parts[0]
}
