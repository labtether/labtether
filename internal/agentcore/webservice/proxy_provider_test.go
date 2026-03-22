package webservice

import (
	"testing"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/agentmgr"
)

// ---------------------------------------------------------------------------
// TestFindHostPort
// ---------------------------------------------------------------------------

func TestFindHostPort(t *testing.T) {
	t.Run("finds mapped port", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
		}
		got := findHostPort(ports, 443)
		if got != 8443 {
			t.Errorf("findHostPort(ports, 443) = %d; want 8443", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		}
		got := findHostPort(ports, 3000)
		if got != 0 {
			t.Errorf("findHostPort(ports, 3000) = %d; want 0", got)
		}
	})

	t.Run("no public mapping", func(t *testing.T) {
		ports := []dockerpkg.DockerPort{
			{PrivatePort: 80, PublicPort: 0, Type: "tcp"},
		}
		got := findHostPort(ports, 80)
		if got != 0 {
			t.Errorf("findHostPort(ports, 80) = %d; want 0 (no public mapping)", got)
		}
	})

	t.Run("empty ports", func(t *testing.T) {
		got := findHostPort(nil, 80)
		if got != 0 {
			t.Errorf("findHostPort(nil, 80) = %d; want 0", got)
		}
	})
}

// ---------------------------------------------------------------------------
// TestEnrichServicesWithRoutes — basic enrichment + proxy-only service
// ---------------------------------------------------------------------------

func TestEnrichServicesWithRoutes(t *testing.T) {
	hostAssetID := "asset-123"
	hostIP := "10.0.1.1"

	// Container "abc123" maps internal port 32400 → host port 32400 (Plex).
	containers := []dockerpkg.DockerContainer{
		{
			ID:    "abc123",
			Names: []string{"/plex"},
			Image: "linuxserver/plex",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 32400, PublicPort: 32400, Type: "tcp"},
			},
		},
	}

	// Service discovered from Docker (URL uses host-mapped port).
	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-1",
			ServiceKey:  "plex",
			Name:        "Plex",
			Category:    CatMedia,
			URL:         "http://10.0.1.1:32400",
			Source:      "docker",
			ContainerID: "abc123",
			HostAssetID: hostAssetID,
			IconKey:     "plex",
		},
	}

	// Route from proxy: plex.home.lab → http://10.0.1.5:32400
	routes := []ProxyRoute{
		{
			Domain:     "plex.home.lab",
			BackendURL: "http://10.0.1.5:32400",
			TLS:        true,
			RouterName: "plex-router",
		},
		// A proxy-only route with no matching container/service.
		{
			Domain:     "wiki.home.lab",
			BackendURL: "http://10.0.1.10:3000",
			TLS:        true,
			RouterName: "wiki-router",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "traefik", hostAssetID, hostIP, containers)

	// The Plex service should be enriched with the proxied URL.
	var plexSvc *agentmgr.DiscoveredWebService
	var wikiSvc *agentmgr.DiscoveredWebService
	for i := range result {
		if result[i].ServiceKey == "plex" {
			plexSvc = &result[i]
		}
		if result[i].Name == "Wiki.js" || result[i].Name == "wiki.home.lab" || result[i].Source == "proxy" {
			wikiSvc = &result[i]
		}
	}

	if plexSvc == nil {
		t.Fatal("plex service not found in result")
	}
	if plexSvc.URL != "https://plex.home.lab" {
		t.Errorf("plex URL = %q; want %q", plexSvc.URL, "https://plex.home.lab")
	}
	if plexSvc.Metadata["raw_url"] != "http://10.0.1.1:32400" {
		t.Errorf("plex raw_url = %q; want %q", plexSvc.Metadata["raw_url"], "http://10.0.1.1:32400")
	}
	if plexSvc.Metadata["proxy_provider"] != "traefik" {
		t.Errorf("plex proxy_provider = %q; want %q", plexSvc.Metadata["proxy_provider"], "traefik")
	}

	// The wiki route should create a proxy-only service.
	if wikiSvc == nil {
		t.Fatal("wiki proxy-only service not found in result")
	}
	if wikiSvc.Source != "proxy" {
		t.Errorf("wiki source = %q; want %q", wikiSvc.Source, "proxy")
	}
	if wikiSvc.URL != "https://wiki.home.lab" {
		t.Errorf("wiki URL = %q; want %q", wikiSvc.URL, "https://wiki.home.lab")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichServicesPrivatePortBridging
// ---------------------------------------------------------------------------

func TestEnrichServicesPrivatePortBridging(t *testing.T) {
	hostAssetID := "asset-456"
	hostIP := "10.0.1.1"

	// Container maps internal port 80 → host port 8085.
	containers := []dockerpkg.DockerContainer{
		{
			ID:    "nginx-abc",
			Names: []string{"/my-nginx"},
			Image: "nginx",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 80, PublicPort: 8085, Type: "tcp"},
			},
		},
	}

	// Service discovered via Docker: uses host-mapped port 8085.
	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-nginx",
			Name:        "my-nginx",
			URL:         "http://10.0.1.1:8085",
			Source:      "docker",
			ContainerID: "nginx-abc",
			HostAssetID: hostAssetID,
		},
	}

	// Proxy backend targets internal port 80 (not host port 8085).
	routes := []ProxyRoute{
		{
			Domain:     "nginx.home.lab",
			BackendURL: "http://10.0.1.1:80",
			TLS:        true,
			RouterName: "nginx-router",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "traefik", hostAssetID, hostIP, containers)

	// Should match via private port bridging: proxy port 80 → container PrivatePort 80 → ContainerID nginx-abc → service.
	var found *agentmgr.DiscoveredWebService
	for i := range result {
		if result[i].ContainerID == "nginx-abc" {
			found = &result[i]
			break
		}
	}

	if found == nil {
		t.Fatal("nginx service not found in result")
	}
	if found.URL != "https://nginx.home.lab" {
		t.Errorf("nginx URL = %q; want %q", found.URL, "https://nginx.home.lab")
	}
	if found.Metadata["raw_url"] != "http://10.0.1.1:8085" {
		t.Errorf("nginx raw_url = %q; want %q", found.Metadata["raw_url"], "http://10.0.1.1:8085")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichServicesMultiDomain
// ---------------------------------------------------------------------------

func TestEnrichServicesMultiDomain(t *testing.T) {
	hostAssetID := "asset-789"
	hostIP := "10.0.1.1"

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "plex-ctr",
			Names: []string{"/plex"},
			Image: "linuxserver/plex",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 32400, PublicPort: 32400, Type: "tcp"},
			},
		},
	}

	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-plex",
			ServiceKey:  "plex",
			Name:        "Plex",
			URL:         "http://10.0.1.1:32400",
			Source:      "docker",
			ContainerID: "plex-ctr",
			HostAssetID: hostAssetID,
		},
	}

	// Two routes point to the same backend.
	routes := []ProxyRoute{
		{
			Domain:     "plex.home.lab",
			BackendURL: "http://10.0.1.5:32400",
			TLS:        true,
			RouterName: "plex-primary",
		},
		{
			Domain:     "media.home.lab",
			BackendURL: "http://10.0.1.5:32400",
			TLS:        true,
			RouterName: "plex-alias",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "traefik", hostAssetID, hostIP, containers)

	var plexSvc *agentmgr.DiscoveredWebService
	for i := range result {
		if result[i].ServiceKey == "plex" {
			plexSvc = &result[i]
			break
		}
	}

	if plexSvc == nil {
		t.Fatal("plex service not found")
	}

	// First domain becomes the URL.
	if plexSvc.URL != "https://plex.home.lab" {
		t.Errorf("plex URL = %q; want %q", plexSvc.URL, "https://plex.home.lab")
	}

	// Second domain goes to alt_urls.
	altURLs := plexSvc.Metadata["alt_urls"]
	if altURLs == "" {
		t.Fatal("plex alt_urls is empty; want media.home.lab entry")
	}
	if altURLs != "https://media.home.lab" {
		t.Errorf("plex alt_urls = %q; want %q", altURLs, "https://media.home.lab")
	}
}

func TestEnrichServicesTraefikProxyOnlyBackendDedup(t *testing.T) {
	hostAssetID := "asset-traefik-proxy-dedup"
	hostIP := "10.0.1.1"

	routes := []ProxyRoute{
		{
			Domain:     "grafana.home.lab",
			BackendURL: "http://10.0.1.10:3000",
			TLS:        true,
			RouterName: "grafana-primary",
		},
		{
			Domain:     "monitor.home.lab",
			BackendURL: "http://10.0.1.10:3000",
			TLS:        true,
			RouterName: "grafana-alias",
		},
		{
			Domain:     "monitor.home.lab",
			BackendURL: "http://10.0.1.10:3000",
			TLS:        true,
			RouterName: "grafana-alias-duplicate",
		},
	}

	result := enrichServicesWithRoutes(nil, routes, "traefik", hostAssetID, hostIP, nil)
	if len(result) != 1 {
		t.Fatalf("got %d services; want 1 deduplicated proxy-only service", len(result))
	}

	svc := result[0]
	if svc.URL != "https://grafana.home.lab" {
		t.Errorf("URL = %q; want %q", svc.URL, "https://grafana.home.lab")
	}
	if svc.Metadata["backend_url"] != "http://10.0.1.10:3000" {
		t.Errorf("backend_url = %q; want %q", svc.Metadata["backend_url"], "http://10.0.1.10:3000")
	}
	if svc.Metadata["alt_urls"] != "https://monitor.home.lab" {
		t.Errorf("alt_urls = %q; want %q", svc.Metadata["alt_urls"], "https://monitor.home.lab")
	}
}

func TestEnrichServicesTraefikSkipsDuplicatePrimaryAlias(t *testing.T) {
	hostAssetID := "asset-traefik-alias-dedup"
	hostIP := "10.0.1.1"

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "app-ctr",
			Names: []string{"/app"},
			Image: "nginx",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
		},
	}

	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-app",
			Name:        "app",
			URL:         "http://10.0.1.1:8080",
			Source:      "docker",
			ContainerID: "app-ctr",
			HostAssetID: hostAssetID,
		},
	}

	routes := []ProxyRoute{
		{
			Domain:     "app.home.lab",
			BackendURL: "http://172.18.0.10:80",
			TLS:        true,
			RouterName: "app-primary",
		},
		{
			Domain:     "app.home.lab",
			BackendURL: "http://172.18.0.10:80",
			TLS:        true,
			RouterName: "app-duplicate",
		},
		{
			Domain:     "app-alt.home.lab",
			BackendURL: "http://172.18.0.10:80",
			TLS:        true,
			RouterName: "app-alias",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "traefik", hostAssetID, hostIP, containers)
	if len(result) != 1 {
		t.Fatalf("got %d services; want 1", len(result))
	}

	svc := result[0]
	if svc.URL != "https://app.home.lab" {
		t.Errorf("URL = %q; want %q", svc.URL, "https://app.home.lab")
	}
	if svc.Metadata["alt_urls"] != "https://app-alt.home.lab" {
		t.Errorf("alt_urls = %q; want %q", svc.Metadata["alt_urls"], "https://app-alt.home.lab")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichServicesDoubleEnrichGuard
// ---------------------------------------------------------------------------

func TestEnrichServicesDoubleEnrichGuard(t *testing.T) {
	hostAssetID := "asset-guard"
	hostIP := "10.0.1.1"

	containers := []dockerpkg.DockerContainer{
		{
			ID:    "grafana-ctr",
			Names: []string{"/grafana"},
			Image: "grafana/grafana",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
		},
	}

	// Service already enriched by a previous proxy provider (e.g. Traefik).
	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-grafana",
			ServiceKey:  "grafana",
			Name:        "Grafana",
			URL:         "https://grafana.home.lab",
			Source:      "docker",
			ContainerID: "grafana-ctr",
			HostAssetID: hostAssetID,
			Metadata: map[string]string{
				"raw_url":        "http://10.0.1.1:3000",
				"proxy_provider": "traefik",
			},
		},
	}

	// Second provider (Caddy) also routes to the same backend.
	routes := []ProxyRoute{
		{
			Domain:     "grafana.caddy.lab",
			BackendURL: "http://10.0.1.1:3000",
			TLS:        true,
			RouterName: "grafana-caddy",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "caddy", hostAssetID, hostIP, containers)

	var grafanaSvc *agentmgr.DiscoveredWebService
	for i := range result {
		if result[i].ServiceKey == "grafana" {
			grafanaSvc = &result[i]
			break
		}
	}

	if grafanaSvc == nil {
		t.Fatal("grafana service not found")
	}

	// URL should NOT be overwritten by the second provider.
	if grafanaSvc.URL != "https://grafana.home.lab" {
		t.Errorf("grafana URL = %q; want %q (should not be overwritten)", grafanaSvc.URL, "https://grafana.home.lab")
	}

	// proxy_provider should remain "traefik" (first provider).
	if grafanaSvc.Metadata["proxy_provider"] != "traefik" {
		t.Errorf("proxy_provider = %q; want %q", grafanaSvc.Metadata["proxy_provider"], "traefik")
	}

	// raw_url should remain the original.
	if grafanaSvc.Metadata["raw_url"] != "http://10.0.1.1:3000" {
		t.Errorf("raw_url = %q; want %q", grafanaSvc.Metadata["raw_url"], "http://10.0.1.1:3000")
	}

	// The second provider's domain should be in alt_urls.
	altURLs := grafanaSvc.Metadata["alt_urls"]
	if altURLs != "https://grafana.caddy.lab" {
		t.Errorf("alt_urls = %q; want %q", altURLs, "https://grafana.caddy.lab")
	}
}

// ---------------------------------------------------------------------------
// TestEnrichServicesTraefikRouterLabelCorrelation
// ---------------------------------------------------------------------------

func TestEnrichServicesTraefikRouterLabelCorrelation(t *testing.T) {
	hostAssetID := "asset-router-map"
	hostIP := "10.0.1.1"

	// Two containers share private port 80, which is ambiguous by port alone.
	containers := []dockerpkg.DockerContainer{
		{
			ID:    "app1-ctr",
			Names: []string{"/app1"},
			Image: "nginx",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 80, PublicPort: 8081, Type: "tcp"},
			},
			Labels: map[string]string{
				"traefik.http.routers.app1.rule": "Host(`app1.home.lab`)",
			},
		},
		{
			ID:    "app2-ctr",
			Names: []string{"/app2"},
			Image: "nginx",
			State: "running",
			Ports: []dockerpkg.DockerPort{
				{PrivatePort: 80, PublicPort: 8082, Type: "tcp"},
			},
			Labels: map[string]string{
				"traefik.http.routers.app2.rule": "Host(`app2.home.lab`)",
			},
		},
	}

	services := []agentmgr.DiscoveredWebService{
		{
			ID:          "svc-app1",
			Name:        "app1",
			URL:         "http://10.0.1.1:8081",
			Source:      "docker",
			ContainerID: "app1-ctr",
			HostAssetID: hostAssetID,
		},
		{
			ID:          "svc-app2",
			Name:        "app2",
			URL:         "http://10.0.1.1:8082",
			Source:      "docker",
			ContainerID: "app2-ctr",
			HostAssetID: hostAssetID,
		},
	}

	routes := []ProxyRoute{
		{
			Domain:     "app2.home.lab",
			BackendURL: "http://172.18.0.10:80",
			TLS:        true,
			RouterName: "app2",
		},
		{
			Domain:     "app1.home.lab",
			BackendURL: "",
			TLS:        false,
			RouterName: "app1",
		},
	}

	result := enrichServicesWithRoutes(services, routes, "traefik", hostAssetID, hostIP, containers)
	if len(result) != 2 {
		t.Fatalf("got %d services; want 2 (no proxy-only entries expected)", len(result))
	}

	var app1Svc *agentmgr.DiscoveredWebService
	var app2Svc *agentmgr.DiscoveredWebService
	for i := range result {
		if result[i].ContainerID == "app1-ctr" {
			app1Svc = &result[i]
		}
		if result[i].ContainerID == "app2-ctr" {
			app2Svc = &result[i]
		}
	}
	if app1Svc == nil || app2Svc == nil {
		t.Fatal("expected both app1 and app2 services")
	}

	if app2Svc.URL != "https://app2.home.lab" {
		t.Errorf("app2 URL = %q; want %q", app2Svc.URL, "https://app2.home.lab")
	}
	if app2Svc.Metadata["raw_url"] != "http://10.0.1.1:8082" {
		t.Errorf("app2 raw_url = %q; want %q", app2Svc.Metadata["raw_url"], "http://10.0.1.1:8082")
	}

	// Even without backend port, route can match via router label correlation.
	if app1Svc.URL != "http://app1.home.lab" {
		t.Errorf("app1 URL = %q; want %q", app1Svc.URL, "http://app1.home.lab")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestBuildProxyOnlyServiceAmbiguousPortUsesGenericLabels(t *testing.T) {
	if _, found := LookupByPort(3000); !found {
		t.Fatal("expected legacy LookupByPort(3000) match for ambiguous-port precondition")
	}
	if _, found := LookupUniqueByPort(3000); found {
		t.Fatal("expected unique lookup for port 3000 to be unresolved")
	}

	route := ProxyRoute{
		Domain:     "portal.home.lab",
		BackendURL: "http://10.0.1.10:3000",
		TLS:        true,
		RouterName: "portal-router",
	}

	svc := buildProxyOnlyService(route, "traefik", "asset-1", "10.0.1.1")
	if svc.Name != "portal" {
		t.Fatalf("name = %q, want %q", svc.Name, "portal")
	}
	if svc.Category != CatOther {
		t.Fatalf("category = %q, want %q", svc.Category, CatOther)
	}
	if svc.ServiceKey != "" {
		t.Fatalf("serviceKey = %q, want empty", svc.ServiceKey)
	}
	if svc.IconKey != "" {
		t.Fatalf("iconKey = %q, want empty", svc.IconKey)
	}
}

func TestBuildProxyOnlyServiceClassifiesByDomainHint(t *testing.T) {
	route := ProxyRoute{
		Domain:     "grafana.home.lab",
		BackendURL: "",
		TLS:        true,
		RouterName: "grafana-router",
	}

	svc := buildProxyOnlyService(route, "traefik", "asset-1", "10.0.1.1")
	if svc.ServiceKey != "grafana" {
		t.Fatalf("serviceKey = %q, want %q", svc.ServiceKey, "grafana")
	}
	if svc.Name != "Grafana" {
		t.Fatalf("name = %q, want %q", svc.Name, "Grafana")
	}
	if svc.Category != CatMonitoring {
		t.Fatalf("category = %q, want %q", svc.Category, CatMonitoring)
	}
	if svc.IconKey != "grafana" {
		t.Fatalf("iconKey = %q, want %q", svc.IconKey, "grafana")
	}
}

func TestBuildProxiedURL(t *testing.T) {
	tests := []struct {
		domain string
		tls    bool
		want   string
	}{
		{"app.home.lab", true, "https://app.home.lab"},
		{"app.home.lab", false, "http://app.home.lab"},
	}
	for _, tt := range tests {
		got := buildProxiedURL(tt.domain, tt.tls)
		if got != tt.want {
			t.Errorf("buildProxiedURL(%q, %v) = %q; want %q", tt.domain, tt.tls, got, tt.want)
		}
	}
}

func TestPortFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want int
	}{
		{"http://10.0.1.5:32400", 32400},
		{"https://example.com:8443/path", 8443},
		{"http://10.0.1.5", 0},
		{"https://secure.local", 0},
		{"", 0},
		{"not-a-url", 0},
	}
	for _, tt := range tests {
		got := portFromURL(tt.url)
		if got != tt.want {
			t.Errorf("portFromURL(%q) = %d; want %d", tt.url, got, tt.want)
		}
	}
}

func TestDomainToName(t *testing.T) {
	tests := []struct {
		domain string
		want   string
	}{
		{"plex.home.lab", "plex"},
		{"my-app.example.com", "my-app"},
		{"singleword", "singleword"},
		{"", ""},
	}
	for _, tt := range tests {
		got := domainToName(tt.domain)
		if got != tt.want {
			t.Errorf("domainToName(%q) = %q; want %q", tt.domain, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestEnrichServicesNoContainers — enrichment is a no-op when no containers
// ---------------------------------------------------------------------------

func TestEnrichServicesNoContainers(t *testing.T) {
	// When no containers are available, enrichment should be a no-op
	services := []agentmgr.DiscoveredWebService{
		{ID: "svc1", Name: "Plex", URL: "http://192.168.1.50:32400", Source: "docker", HostAssetID: "host1"},
	}

	result := enrichServicesWithRoutes(services, nil, "traefik", "host1", "192.168.1.50", nil)
	if len(result) != 1 {
		t.Fatalf("got %d services, want 1", len(result))
	}
	if result[0].URL != "http://192.168.1.50:32400" {
		t.Errorf("URL should be unchanged: %q", result[0].URL)
	}
}

func TestAppendAltURL(t *testing.T) {
	tests := []struct {
		existing string
		newURL   string
		want     string
	}{
		{"", "https://a.home.lab", "https://a.home.lab"},
		{"https://a.home.lab", "https://b.home.lab", "https://a.home.lab,https://b.home.lab"},
		{"https://a.home.lab", "https://a.home.lab", "https://a.home.lab"},
		{"https://a.home.lab,https://b.home.lab", "https://B.home.lab", "https://a.home.lab,https://b.home.lab"},
		{"https://a.home.lab, https://a.home.lab", " https://b.home.lab ", "https://a.home.lab,https://b.home.lab"},
	}
	for _, tt := range tests {
		got := appendAltURL(tt.existing, tt.newURL)
		if got != tt.want {
			t.Errorf("appendAltURL(%q, %q) = %q; want %q", tt.existing, tt.newURL, got, tt.want)
		}
	}
}

func TestCanonicalBackendIdentity(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"http://10.0.1.10:3000", "http://10.0.1.10:3000/"},
		{"https://EXAMPLE.local", "https://example.local:443/"},
		{"http://api.local/path", "http://api.local:80/path"},
		{"not a url", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := canonicalBackendIdentity(tt.raw)
		if got != tt.want {
			t.Errorf("canonicalBackendIdentity(%q) = %q; want %q", tt.raw, got, tt.want)
		}
	}
}

func TestTraefikRoutersFromLabels(t *testing.T) {
	labels := map[string]string{
		"traefik.http.routers.App.Rule":        "Host(`app.home.lab`)",
		"traefik.http.routers.app.entrypoints": "websecure",
		"traefik.http.middlewares.a.b":         "true",
		"other.label":                          "x",
	}

	got := traefikRoutersFromLabels(labels)
	if len(got) != 1 {
		t.Fatalf("len(traefikRoutersFromLabels) = %d; want 1", len(got))
	}
	if _, ok := got["app"]; !ok {
		t.Fatalf("router set missing %q", "app")
	}
}
