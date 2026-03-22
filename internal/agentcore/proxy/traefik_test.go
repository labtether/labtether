package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// TestTraefikProviderName
// ---------------------------------------------------------------------------

func TestTraefikProviderName(t *testing.T) {
	p := NewTraefikProvider()
	if p.Name() != "traefik" {
		t.Errorf("Name() = %q; want %q", p.Name(), "traefik")
	}
}

// ---------------------------------------------------------------------------
// TestTraefikDetectAndConnect
// ---------------------------------------------------------------------------

func TestTraefikDetectAndConnect(t *testing.T) {
	t.Run("detects traefik with API port exposed", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-123",
				Names: []string{"/traefik"},
				Image: "traefik:v3.0",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 8080, PublicPort: 8080, Type: "tcp"},
				},
			},
		}

		p := NewTraefikProvider()
		apiURL, ok := p.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true")
		}
		if apiURL != "http://localhost:8080" {
			t.Errorf("apiURL = %q; want %q", apiURL, "http://localhost:8080")
		}
	})

	t.Run("detects traefik with remapped API port", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-456",
				Names: []string{"/traefik"},
				Image: "docker.io/library/traefik:latest",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 8080, PublicPort: 9999, Type: "tcp"},
				},
			},
		}

		p := NewTraefikProvider()
		apiURL, ok := p.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true")
		}
		if apiURL != "http://localhost:9999" {
			t.Errorf("apiURL = %q; want %q", apiURL, "http://localhost:9999")
		}
	})

	t.Run("falls back to dashboard host rule when API port not exposed", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-dashboard",
				Names: []string{"/traefik"},
				Image: "traefik:v3.0",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 443, Type: "tcp"},
				},
				Labels: map[string]string{
					"traefik.http.routers.dashboard.rule":        "Host(`dashboard.example.com`)",
					"traefik.http.routers.dashboard.entrypoints": "websecure",
					"traefik.http.routers.dashboard.service":     "api@internal",
				},
			},
		}

		p := NewTraefikProvider()
		apiURL, ok := p.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true from dashboard label fallback")
		}
		if apiURL != "https://dashboard.example.com" {
			t.Errorf("apiURL = %q; want %q", apiURL, "https://dashboard.example.com")
		}
	})

	t.Run("dashboard fallback prefers http when tls hints absent", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-dashboard-http",
				Names: []string{"/traefik"},
				Image: "traefik:v3.0",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					"traefik.http.routers.dashboard.rule":    "Host(`dashboard-http.example.com`)",
					"traefik.http.routers.dashboard.service": "api@internal",
				},
			},
		}

		p := NewTraefikProvider()
		apiURL, ok := p.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true from dashboard label fallback")
		}
		if apiURL != "http://dashboard-http.example.com" {
			t.Errorf("apiURL = %q; want %q", apiURL, "http://dashboard-http.example.com")
		}
	})

	t.Run("skips when API port not exposed", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-789",
				Names: []string{"/traefik"},
				Image: "traefik:v3.0",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 8080, PublicPort: 0, Type: "tcp"},
				},
			},
		}

		p := NewTraefikProvider()
		_, ok := p.DetectAndConnect(containers)
		if ok {
			t.Error("DetectAndConnect returned true; want false (API port not exposed)")
		}
	})

	t.Run("skips non-traefik containers", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "nginx-123",
				Names: []string{"/nginx"},
				Image: "nginx:latest",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 8080, PublicPort: 8080, Type: "tcp"},
				},
			},
			{
				ID:    "plex-456",
				Names: []string{"/plex"},
				Image: "linuxserver/plex",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 32400, PublicPort: 32400, Type: "tcp"},
				},
			},
		}

		p := NewTraefikProvider()
		_, ok := p.DetectAndConnect(containers)
		if ok {
			t.Error("DetectAndConnect returned true; want false (no traefik container)")
		}
	})

	t.Run("empty containers list", func(t *testing.T) {
		p := NewTraefikProvider()
		_, ok := p.DetectAndConnect(nil)
		if ok {
			t.Error("DetectAndConnect returned true; want false (no containers)")
		}
	})
}

// ---------------------------------------------------------------------------
// TestTraefikFetchRoutes
// ---------------------------------------------------------------------------

func TestTraefikFetchRoutes(t *testing.T) {
	t.Run("fetches and parses routes correctly", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "plex@docker",
				Rule:        "Host(`plex.home.lab`)",
				Service:     "plex-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"websecure"},
			},
			{
				Name:        "sonarr@docker",
				Rule:        "Host(`sonarr.home.lab`)",
				Service:     "sonarr-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
		}

		services := []traefikServiceItem{
			{
				Name: "plex-svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.5:32400"},
					},
				},
			},
			{
				Name: "sonarr-svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.6:8989"},
					},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 2 {
			t.Fatalf("got %d routes; want 2", len(routes))
		}

		// Check plex route.
		plex := routes[0]
		if plex.Domain != "plex.home.lab" {
			t.Errorf("plex domain = %q; want %q", plex.Domain, "plex.home.lab")
		}
		if plex.BackendURL != "http://172.18.0.5:32400" {
			t.Errorf("plex backend = %q; want %q", plex.BackendURL, "http://172.18.0.5:32400")
		}
		if !plex.TLS {
			t.Error("plex TLS = false; want true (websecure entrypoint)")
		}
		if plex.RouterName != "plex" {
			t.Errorf("plex router name = %q; want %q", plex.RouterName, "plex")
		}

		// Check sonarr route.
		sonarr := routes[1]
		if sonarr.Domain != "sonarr.home.lab" {
			t.Errorf("sonarr domain = %q; want %q", sonarr.Domain, "sonarr.home.lab")
		}
		if sonarr.BackendURL != "http://172.18.0.6:8989" {
			t.Errorf("sonarr backend = %q; want %q", sonarr.BackendURL, "http://172.18.0.6:8989")
		}
		if sonarr.TLS {
			t.Error("sonarr TLS = true; want false (web entrypoint)")
		}
		if sonarr.RouterName != "sonarr" {
			t.Errorf("sonarr router name = %q; want %q", sonarr.RouterName, "sonarr")
		}
	})

	t.Run("filters @internal routes", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "api@internal",
				Rule:        "PathPrefix(`/api`)",
				Service:     "api@internal",
				Status:      "enabled",
				EntryPoints: []string{"traefik"},
			},
			{
				Name:        "dashboard@internal",
				Rule:        "PathPrefix(`/dashboard`)",
				Service:     "dashboard@internal",
				Status:      "enabled",
				EntryPoints: []string{"traefik"},
			},
			{
				Name:        "real-app@docker",
				Rule:        "Host(`app.home.lab`)",
				Service:     "app-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"websecure"},
			},
		}

		services := []traefikServiceItem{
			{
				Name: "app-svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.10:3000"},
					},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 1 {
			t.Fatalf("got %d routes; want 1 (@internal should be filtered)", len(routes))
		}

		if routes[0].RouterName != "real-app" {
			t.Errorf("route name = %q; want %q", routes[0].RouterName, "real-app")
		}
	})

	t.Run("filters disabled routes", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "disabled-app@docker",
				Rule:        "Host(`disabled.home.lab`)",
				Service:     "disabled-svc@docker",
				Status:      "disabled",
				EntryPoints: []string{"websecure"},
			},
			{
				Name:        "enabled-app@docker",
				Rule:        "Host(`enabled.home.lab`)",
				Service:     "enabled-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
		}

		services := []traefikServiceItem{
			{
				Name: "enabled-svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.20:8080"},
					},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 1 {
			t.Fatalf("got %d routes; want 1 (disabled should be filtered)", len(routes))
		}

		if routes[0].Domain != "enabled.home.lab" {
			t.Errorf("route domain = %q; want %q", routes[0].Domain, "enabled.home.lab")
		}
	})

	t.Run("TLS detection from entrypoints", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "https-ep@docker",
				Rule:        "Host(`https-ep.home.lab`)",
				Service:     "svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"https"},
			},
			{
				Name:        "tls-ep@docker",
				Rule:        "Host(`tls-ep.home.lab`)",
				Service:     "svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"tls"},
			},
			{
				Name:        "websecure-ep@docker",
				Rule:        "Host(`ws-ep.home.lab`)",
				Service:     "svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"websecure"},
			},
			{
				Name:        "http-only@docker",
				Rule:        "Host(`http.home.lab`)",
				Service:     "svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
		}

		services := []traefikServiceItem{
			{
				Name: "svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.30:8080"},
					},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 4 {
			t.Fatalf("got %d routes; want 4", len(routes))
		}

		// https entrypoint -> TLS
		if !routes[0].TLS {
			t.Error("https-ep TLS = false; want true")
		}
		// tls entrypoint -> TLS
		if !routes[1].TLS {
			t.Error("tls-ep TLS = false; want true")
		}
		// websecure entrypoint -> TLS
		if !routes[2].TLS {
			t.Error("websecure-ep TLS = false; want true")
		}
		// web entrypoint -> no TLS
		if routes[3].TLS {
			t.Error("http-only TLS = true; want false")
		}
	})

	t.Run("TLS detection from TLS config field", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "tls-config@docker",
				Rule:        "Host(`tls-config.home.lab`)",
				Service:     "svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"web"},
				TLS:         &struct{}{},
			},
		}

		services := []traefikServiceItem{
			{
				Name: "svc@docker",
				LoadBalancer: &traefikLoadBalancer{
					Servers: []traefikServer{
						{URL: "http://172.18.0.40:8080"},
					},
				},
			},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 1 {
			t.Fatalf("got %d routes; want 1", len(routes))
		}

		if !routes[0].TLS {
			t.Error("TLS = false; want true (TLS config present)")
		}
	})

	t.Run("handles no backend URL gracefully", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "no-backend@docker",
				Rule:        "Host(`nb.home.lab`)",
				Service:     "missing-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"websecure"},
			},
		}

		// No matching service in services list.
		services := []traefikServiceItem{}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 1 {
			t.Fatalf("got %d routes; want 1", len(routes))
		}

		if routes[0].BackendURL != "" {
			t.Errorf("backend URL = %q; want empty", routes[0].BackendURL)
		}
	})

	t.Run("strips provider suffix from router name", func(t *testing.T) {
		routers := []traefikRouter{
			{
				Name:        "plex-router@docker",
				Rule:        "Host(`plex.home.lab`)",
				Service:     "plex-svc@docker",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
			{
				Name:        "simple-name@file",
				Rule:        "Host(`simple.home.lab`)",
				Service:     "simple-svc@file",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
			{
				Name:        "no-suffix",
				Rule:        "Host(`nosuffix.home.lab`)",
				Service:     "nosuffix-svc",
				Status:      "enabled",
				EntryPoints: []string{"web"},
			},
		}

		services := []traefikServiceItem{}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/http/routers":
				json.NewEncoder(w).Encode(routers)
			case "/api/http/services":
				json.NewEncoder(w).Encode(services)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		p := NewTraefikProvider()
		routes, err := p.FetchRoutes(srv.URL)
		if err != nil {
			t.Fatalf("FetchRoutes returned error: %v", err)
		}

		if len(routes) != 3 {
			t.Fatalf("got %d routes; want 3", len(routes))
		}

		if routes[0].RouterName != "plex-router" {
			t.Errorf("route[0] name = %q; want %q", routes[0].RouterName, "plex-router")
		}
		if routes[1].RouterName != "simple-name" {
			t.Errorf("route[1] name = %q; want %q", routes[1].RouterName, "simple-name")
		}
		if routes[2].RouterName != "no-suffix" {
			t.Errorf("route[2] name = %q; want %q", routes[2].RouterName, "no-suffix")
		}
	})
}

// ---------------------------------------------------------------------------
// TestExtractHostFromRule
// ---------------------------------------------------------------------------

func TestExtractHostsFromRule(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want []string
	}{
		{
			name: "standard Host rule",
			rule: "Host(`plex.home.lab`)",
			want: []string{"plex.home.lab"},
		},
		{
			name: "Host with path prefix",
			rule: "Host(`app.home.lab`) && PathPrefix(`/api`)",
			want: []string{"app.home.lab"},
		},
		{
			name: "Host with multiple conditions",
			rule: "Host(`grafana.home.lab`) && Headers(`X-Custom`, `value`)",
			want: []string{"grafana.home.lab"},
		},
		{
			name: "multi-host OR rule",
			rule: "Host(`a.home.lab`) || Host(`b.home.lab`)",
			want: []string{"a.home.lab", "b.home.lab"},
		},
		{
			name: "no Host rule",
			rule: "PathPrefix(`/api`)",
			want: nil,
		},
		{
			name: "empty rule",
			rule: "",
			want: nil,
		},
		{
			name: "Host with subdomain",
			rule: "Host(`sub.domain.example.com`)",
			want: []string{"sub.domain.example.com"},
		},
		{
			name: "HostSNI rule (should not match Host pattern)",
			rule: "HostSNI(`*`)",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHostsFromRule(tt.rule)
			if len(got) != len(tt.want) {
				t.Errorf("extractHostsFromRule(%q) = %v; want %v", tt.rule, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractHostsFromRule(%q)[%d] = %q; want %q", tt.rule, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestIsTraefikInternal
// ---------------------------------------------------------------------------

func TestIsTraefikInternal(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"api@internal", true},
		{"dashboard@internal", true},
		{"noop@internal", true},
		{"plex@docker", false},
		{"app@file", false},
		{"internal", false},
		{"my-internal-app@docker", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTraefikInternal(tt.name)
			if got != tt.want {
				t.Errorf("isTraefikInternal(%q) = %v; want %v", tt.name, got, tt.want)
			}
		})
	}
}
