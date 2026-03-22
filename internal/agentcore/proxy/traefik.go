package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// TraefikProvider — reverse proxy provider for Traefik
// ---------------------------------------------------------------------------

// TraefikProvider discovers routing rules from a running Traefik instance
// via its HTTP API (port 8080 by default).
type TraefikProvider struct {
	client    *http.Client
	manualURL string // set via LABTETHER_TRAEFIK_URL to skip container detection
	basicUser string // optional basic auth user (LABTETHER_TRAEFIK_USER)
	basicPass string // optional basic auth password (LABTETHER_TRAEFIK_PASSWORD)
}

// NewTraefikProvider creates a TraefikProvider with a 5-second timeout HTTP client.
func NewTraefikProvider() *TraefikProvider {
	return &TraefikProvider{
		client:    &http.Client{Timeout: 5 * time.Second},
		manualURL: strings.TrimRight(os.Getenv("LABTETHER_TRAEFIK_URL"), "/"),
		basicUser: os.Getenv("LABTETHER_TRAEFIK_USER"),
		basicPass: os.Getenv("LABTETHER_TRAEFIK_PASSWORD"),
	}
}

// Name returns the provider identifier.
func (p *TraefikProvider) Name() string {
	return "traefik"
}

// ---------------------------------------------------------------------------
// DetectAndConnect — find Traefik among running Docker containers
// ---------------------------------------------------------------------------

// traefikAPIPort is the default internal port for the Traefik API/dashboard.
const traefikAPIPort = 8080
const traefikRouterLabelPrefixKey = "traefik.http.routers."

// DetectAndConnect inspects running containers to find a Traefik instance
// with its API port (8080) exposed to the host. Returns the API URL and
// true if found, or ("", false) otherwise.
func (p *TraefikProvider) DetectAndConnect(containers []dockerpkg.DockerContainer) (string, bool) {
	// Manual URL override takes precedence over container detection.
	if p.manualURL != "" {
		return p.manualURL, true
	}

	for _, c := range containers {
		img := normalizeDockerImage(c.Image)
		if img != "traefik" {
			continue
		}

		hostPort := FindHostPort(c.Ports, traefikAPIPort)
		if hostPort == 0 {
			// Fallback: some deployments expose the dashboard/API only via a
			// routed host (for example Host(`dashboard.example.com`)) instead of
			// publishing :8080 directly. If labels expose an api@internal router,
			// use that URL as the API base.
			if apiURL, ok := traefikDashboardURLFromLabels(c.Labels); ok {
				return apiURL, true
			}
			continue
		}

		return fmt.Sprintf("http://localhost:%d", hostPort), true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// FetchRoutes — query Traefik API for routing rules
// ---------------------------------------------------------------------------

// FetchRoutes queries the Traefik HTTP API for routers and services, then
// correlates them into Route entries.
func (p *TraefikProvider) FetchRoutes(apiURL string) ([]Route, error) {
	// Fetch routers.
	routers, err := p.fetchRouters(apiURL)
	if err != nil {
		return nil, fmt.Errorf("traefik routers: %w", err)
	}

	// Fetch services and build lookup map.
	svcItems, err := p.fetchServices(apiURL)
	if err != nil {
		return nil, fmt.Errorf("traefik services: %w", err)
	}
	svcMap := make(map[string]traefikServiceItem, len(svcItems))
	for _, s := range svcItems {
		svcMap[s.Name] = s
	}

	// Build Route list.
	var routes []Route
	for _, r := range routers {
		// Skip internal Traefik routers.
		if isTraefikInternal(r.Name) {
			continue
		}

		// Skip disabled routers.
		if r.Status != "enabled" {
			continue
		}

		// Extract all domains from the routing rule (handles OR rules).
		domains := extractHostsFromRule(r.Rule)
		if len(domains) == 0 {
			continue
		}

		// Determine TLS: explicit TLS config OR TLS-indicating entrypoint.
		tls := r.TLS != nil || hasSecureEntryPoint(r.EntryPoints)

		// Look up backend URL from the service map.
		backendURL := ""
		if svc, ok := svcMap[r.Service]; ok {
			if svc.LoadBalancer != nil && len(svc.LoadBalancer.Servers) > 0 {
				backendURL = svc.LoadBalancer.Servers[0].URL
			}
		}

		// Strip provider suffix from router name ("plex@docker" → "plex").
		routerName := stripProviderSuffix(r.Name)

		for _, domain := range domains {
			routes = append(routes, Route{
				Domain:     domain,
				BackendURL: backendURL,
				TLS:        tls,
				RouterName: routerName,
			})
		}
	}

	return routes, nil
}

// ---------------------------------------------------------------------------
// Traefik API response types
// ---------------------------------------------------------------------------

type traefikRouter struct {
	Name        string    `json:"name"`
	Rule        string    `json:"rule"`
	Service     string    `json:"service"`
	Status      string    `json:"status"`
	EntryPoints []string  `json:"entryPoints"`
	TLS         *struct{} `json:"tls,omitempty"`
}

type traefikServiceItem struct {
	Name         string               `json:"name"`
	LoadBalancer *traefikLoadBalancer `json:"loadBalancer,omitempty"`
}

type traefikLoadBalancer struct {
	Servers []traefikServer `json:"servers"`
}

type traefikServer struct {
	URL string `json:"url"`
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// traefikGet performs a GET request with optional basic auth.
func (p *TraefikProvider) traefikGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.basicUser != "" && p.basicPass != "" {
		req.SetBasicAuth(p.basicUser, p.basicPass)
	}
	return p.client.Do(req) // #nosec G704 -- Request target is the configured local Traefik admin API endpoint for this proxy instance.
}

// fetchRouters queries GET {apiURL}/api/http/routers?per_page=1000.
func (p *TraefikProvider) fetchRouters(apiURL string) ([]traefikRouter, error) {
	resp, err := p.traefikGet(apiURL + "/api/http/routers?per_page=1000")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from routers endpoint", resp.StatusCode)
	}

	var routers []traefikRouter
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		return nil, fmt.Errorf("decode routers: %w", err)
	}
	return routers, nil
}

// fetchServices queries GET {apiURL}/api/http/services?per_page=1000.
func (p *TraefikProvider) fetchServices(apiURL string) ([]traefikServiceItem, error) {
	resp, err := p.traefikGet(apiURL + "/api/http/services?per_page=1000")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from services endpoint", resp.StatusCode)
	}

	var services []traefikServiceItem
	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		return nil, fmt.Errorf("decode services: %w", err)
	}
	return services, nil
}

// isTraefikInternal returns true if the router name ends with "@internal",
// indicating a Traefik-internal router (API, dashboard, ping, etc.).
func isTraefikInternal(name string) bool {
	return strings.HasSuffix(name, "@internal")
}

var traefikHostFuncRegex = regexp.MustCompile(`(?i)(Host|HostRegexp)\(([^)]*)\)`)

// extractHostsFromRule extracts host tokens from Traefik rules.
// It supports:
//   - Host(`a.com`)
//   - Host(`a.com`,`b.com`)
//   - Host("a.com")
//   - HostRegexp(`...`) when the token is concrete (non-pattern)
func extractHostsFromRule(rule string) []string {
	allMatches := traefikHostFuncRegex.FindAllStringSubmatch(rule, -1)
	var hosts []string
	seen := make(map[string]struct{})
	for _, m := range allMatches {
		if len(m) < 3 {
			continue
		}
		fnName := strings.ToLower(strings.TrimSpace(m[1]))
		args := parseTraefikCallArgs(m[2])
		for _, raw := range args {
			host := strings.TrimSpace(strings.Trim(raw, "`\"'"))
			if host == "" {
				continue
			}
			if fnName == "hostregexp" && strings.ContainsAny(host, "{}*[]+?") {
				// Pattern-only rules are not directly navigable domains.
				continue
			}
			if _, ok := seen[host]; ok {
				continue
			}
			seen[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func parseTraefikCallArgs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// traefikDashboardURLFromLabels infers a reachable Traefik API base URL from
// container labels when port 8080 is not host-exposed.
//
// It looks for routers where:
//   - service = api@internal
//   - rule contains a Host(...) domain
//
// then builds http(s)://<domain> using entrypoint/TLS hints.
func traefikDashboardURLFromLabels(labels map[string]string) (string, bool) {
	if len(labels) == 0 {
		return "", false
	}

	normalized := make(map[string]string, len(labels))
	for key, value := range labels {
		normalized[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}

	for key, value := range normalized {
		if !strings.HasPrefix(key, traefikRouterLabelPrefixKey) || !strings.HasSuffix(key, ".service") {
			continue
		}
		if strings.ToLower(strings.TrimSpace(value)) != "api@internal" {
			continue
		}

		routerKey := strings.TrimSuffix(strings.TrimPrefix(key, traefikRouterLabelPrefixKey), ".service")
		if routerKey == "" {
			continue
		}

		rule := normalized[traefikRouterLabelPrefixKey+routerKey+".rule"]
		domains := extractHostsFromRule(rule)
		if len(domains) == 0 {
			continue
		}

		entryPoints := splitCommaTrimmed(normalized[traefikRouterLabelPrefixKey+routerKey+".entrypoints"])
		tls := hasSecureEntryPoint(entryPoints)
		if !tls {
			// Explicit TLS settings also imply HTTPS routing.
			if normalized[traefikRouterLabelPrefixKey+routerKey+".tls"] != "" ||
				normalized[traefikRouterLabelPrefixKey+routerKey+".tls.certresolver"] != "" {
				tls = true
			}
		}

		scheme := "http"
		if tls {
			scheme = "https"
		}
		return scheme + "://" + domains[0], true
	}

	return "", false
}

func splitCommaTrimmed(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// secureEntryPoints lists Traefik entrypoint names that imply TLS termination.
var secureEntryPoints = map[string]bool{
	"websecure": true,
	"https":     true,
	"tls":       true,
}

// hasSecureEntryPoint returns true if any of the entrypoints indicate TLS.
func hasSecureEntryPoint(entryPoints []string) bool {
	for _, ep := range entryPoints {
		if secureEntryPoints[strings.ToLower(ep)] {
			return true
		}
	}
	return false
}

// stripProviderSuffix removes the "@provider" suffix from a Traefik router
// or service name (e.g. "plex-router@docker" → "plex-router").
func stripProviderSuffix(name string) string {
	if idx := strings.LastIndex(name, "@"); idx >= 0 {
		return name[:idx]
	}
	return name
}
