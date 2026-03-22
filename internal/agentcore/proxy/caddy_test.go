package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// TestCaddyDetectAndConnect
// ---------------------------------------------------------------------------

func TestCaddyDetectAndConnect(t *testing.T) {
	t.Run("detects caddy image", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "caddy-abc",
				Names: []string{"/caddy"},
				Image: "caddy:2",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
					{PrivatePort: 2019, PublicPort: 2019, Type: "tcp"},
				},
			},
		}

		provider := NewCaddyProvider()
		apiURL, ok := provider.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true for caddy image")
		}
		if apiURL != "http://localhost:2019" {
			t.Errorf("apiURL = %q; want %q", apiURL, "http://localhost:2019")
		}
	})

	t.Run("detects caddy-docker-proxy image", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "cdp-xyz",
				Names: []string{"/caddy-docker-proxy"},
				Image: "lucaslorentz/caddy-docker-proxy:2.8",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 443, Type: "tcp"},
					{PrivatePort: 2019, PublicPort: 12019, Type: "tcp"},
				},
			},
		}

		provider := NewCaddyProvider()
		apiURL, ok := provider.DetectAndConnect(containers)
		if !ok {
			t.Fatal("DetectAndConnect returned false; want true for caddy-docker-proxy image")
		}
		if apiURL != "http://localhost:12019" {
			t.Errorf("apiURL = %q; want %q", apiURL, "http://localhost:12019")
		}
	})

	t.Run("skips when admin port not exposed", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "caddy-no-admin",
				Names: []string{"/caddy"},
				Image: "caddy:latest",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
					// No port 2019 exposed
				},
			},
		}

		provider := NewCaddyProvider()
		_, ok := provider.DetectAndConnect(containers)
		if ok {
			t.Fatal("DetectAndConnect returned true; want false when admin port 2019 not exposed")
		}
	})

	t.Run("skips unrelated containers", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "nginx-abc",
				Names: []string{"/nginx"},
				Image: "nginx:latest",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				},
			},
		}

		provider := NewCaddyProvider()
		_, ok := provider.DetectAndConnect(containers)
		if ok {
			t.Fatal("DetectAndConnect returned true; want false for non-caddy container")
		}
	})
}

// ---------------------------------------------------------------------------
// TestCaddyFetchRoutes
// ---------------------------------------------------------------------------

func TestCaddyFetchRoutes(t *testing.T) {
	// Build a mock Caddy config with two servers:
	// - srv0: TLS server (:443) with reverse_proxy and static_response routes
	// - srv1: HTTP server (:80) with a reverse_proxy route
	config := caddyConfig{}
	config.Apps.HTTP.Servers = map[string]caddyServer{
		"srv0": {
			Listen: []string{":443"},
			Routes: []caddyRoute{
				{
					Match: []caddyMatch{{Host: []string{"plex.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "172.18.0.5:32400"}},
						},
					},
				},
				{
					Match: []caddyMatch{{Host: []string{"grafana.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "172.18.0.10:3000"}},
						},
					},
				},
				{
					// static_response handler should be skipped
					Match: []caddyMatch{{Host: []string{"redirect.home.lab"}}},
					Handle: []caddyHandler{
						{Handler: "static_response"},
					},
				},
			},
		},
		"srv1": {
			Listen: []string{":80"},
			Routes: []caddyRoute{
				{
					Match: []caddyMatch{{Host: []string{"http-app.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "172.18.0.20:8080"}},
						},
					},
				},
			},
		},
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(configBytes)
	}))
	defer ts.Close()

	provider := NewCaddyProvider()
	routes, err := provider.FetchRoutes(ts.URL)
	if err != nil {
		t.Fatalf("FetchRoutes returned error: %v", err)
	}

	// Expect 3 routes: 2 from TLS server (plex, grafana) + 1 from HTTP server (http-app)
	// static_response route should be skipped
	if len(routes) != 3 {
		t.Fatalf("got %d routes; want 3", len(routes))
	}

	// Build a lookup by domain for easier assertions.
	byDomain := make(map[string]Route)
	for _, r := range routes {
		byDomain[r.Domain] = r
	}

	// Check plex route (TLS server).
	plex, ok := byDomain["plex.home.lab"]
	if !ok {
		t.Fatal("plex.home.lab route not found")
	}
	if plex.BackendURL != "http://172.18.0.5:32400" {
		t.Errorf("plex backend = %q; want %q", plex.BackendURL, "http://172.18.0.5:32400")
	}
	if !plex.TLS {
		t.Error("plex TLS = false; want true")
	}

	// Check grafana route (TLS server).
	grafana, ok := byDomain["grafana.home.lab"]
	if !ok {
		t.Fatal("grafana.home.lab route not found")
	}
	if grafana.BackendURL != "http://172.18.0.10:3000" {
		t.Errorf("grafana backend = %q; want %q", grafana.BackendURL, "http://172.18.0.10:3000")
	}
	if !grafana.TLS {
		t.Error("grafana TLS = false; want true")
	}

	// Check http-app route (HTTP server — not TLS).
	httpApp, ok := byDomain["http-app.home.lab"]
	if !ok {
		t.Fatal("http-app.home.lab route not found")
	}
	if httpApp.BackendURL != "http://172.18.0.20:8080" {
		t.Errorf("http-app backend = %q; want %q", httpApp.BackendURL, "http://172.18.0.20:8080")
	}
	if httpApp.TLS {
		t.Error("http-app TLS = true; want false")
	}

	// Verify static_response route was skipped.
	if _, found := byDomain["redirect.home.lab"]; found {
		t.Error("redirect.home.lab route should not be included (static_response handler)")
	}
}

// ---------------------------------------------------------------------------
// TestCaddyFetchRoutesAlternateTLSPorts
// ---------------------------------------------------------------------------

func TestCaddyFetchRoutesAlternateTLSPorts(t *testing.T) {
	config := caddyConfig{}
	config.Apps.HTTP.Servers = map[string]caddyServer{
		"srv0": {
			Listen: []string{":8443"},
			Routes: []caddyRoute{
				{
					Match: []caddyMatch{{Host: []string{"app.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "10.0.0.5:9000"}},
						},
					},
				},
			},
		},
		"srv1": {
			Listen: []string{":9443"},
			Routes: []caddyRoute{
				{
					Match: []caddyMatch{{Host: []string{"secure.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "10.0.0.6:8080"}},
						},
					},
				},
			},
		},
		"srv2": {
			Listen: []string{":10443"},
			Routes: []caddyRoute{
				{
					Match: []caddyMatch{{Host: []string{"extra.home.lab"}}},
					Handle: []caddyHandler{
						{
							Handler:   "reverse_proxy",
							Upstreams: []caddyUpstream{{Dial: "10.0.0.7:3000"}},
						},
					},
				},
			},
		},
	}

	configBytes, _ := json.Marshal(config)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(configBytes)
	}))
	defer ts.Close()

	provider := NewCaddyProvider()
	routes, err := provider.FetchRoutes(ts.URL)
	if err != nil {
		t.Fatalf("FetchRoutes returned error: %v", err)
	}

	for _, r := range routes {
		if !r.TLS {
			t.Errorf("route %q TLS = false; want true (alternate TLS port)", r.Domain)
		}
	}
}

// ---------------------------------------------------------------------------
// TestCaddyFetchRoutesEmptyConfig
// ---------------------------------------------------------------------------

func TestCaddyFetchRoutesEmptyConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	provider := NewCaddyProvider()
	routes, err := provider.FetchRoutes(ts.URL)
	if err != nil {
		t.Fatalf("FetchRoutes returned error on empty config: %v", err)
	}
	if len(routes) != 0 {
		t.Errorf("got %d routes; want 0 for empty config", len(routes))
	}
}

// ---------------------------------------------------------------------------
// TestCaddyName
// ---------------------------------------------------------------------------

func TestCaddyName(t *testing.T) {
	provider := NewCaddyProvider()
	if provider.Name() != "caddy" {
		t.Errorf("Name() = %q; want %q", provider.Name(), "caddy")
	}
}
