package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// TestNPMDetectAndConnect
// ---------------------------------------------------------------------------

func TestNPMDetectAndConnect(t *testing.T) {
	provider := &NPMProvider{}

	t.Run("detects NPM container with admin port", func(t *testing.T) {
		t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")

		containers := []dockerpkg.DockerContainer{
			{
				ID:    "npm-abc123",
				Names: []string{"/nginx-proxy-manager"},
				Image: "jc21/nginx-proxy-manager:latest",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					{PrivatePort: 81, PublicPort: 8181, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
				},
			},
		}

		apiURL, ok := provider.DetectAndConnect(containers)
		if !ok {
			t.Fatal("expected DetectAndConnect to return true for NPM container")
		}
		if apiURL != "https://localhost:8181" {
			t.Errorf("apiURL = %q; want %q", apiURL, "https://localhost:8181")
		}
	})

	t.Run("skips non-NPM containers", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "traefik-abc",
				Names: []string{"/traefik"},
				Image: "traefik:v3.0",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 8080, PublicPort: 8080, Type: "tcp"},
				},
			},
			{
				ID:    "plex-abc",
				Names: []string{"/plex"},
				Image: "linuxserver/plex",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 32400, PublicPort: 32400, Type: "tcp"},
				},
			},
		}

		_, ok := provider.DetectAndConnect(containers)
		if ok {
			t.Fatal("expected DetectAndConnect to return false for non-NPM containers")
		}
	})

	t.Run("skips NPM container without admin port mapping", func(t *testing.T) {
		containers := []dockerpkg.DockerContainer{
			{
				ID:    "npm-no-admin",
				Names: []string{"/npm"},
				Image: "jc21/nginx-proxy-manager:2.10",
				State: "running",
				Ports: []dockerpkg.DockerPort{
					{PrivatePort: 80, PublicPort: 80, Type: "tcp"},
					{PrivatePort: 443, PublicPort: 443, Type: "tcp"},
					// No port 81 mapping
				},
			},
		}

		_, ok := provider.DetectAndConnect(containers)
		if ok {
			t.Fatal("expected DetectAndConnect to return false when admin port 81 is not mapped")
		}
	})

	t.Run("handles empty container list", func(t *testing.T) {
		_, ok := provider.DetectAndConnect(nil)
		if ok {
			t.Fatal("expected DetectAndConnect to return false for nil containers")
		}
	})
}

// ---------------------------------------------------------------------------
// TestNPMFetchRoutes
// ---------------------------------------------------------------------------

func TestNPMFetchRoutes(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	// Set up mock server that simulates NPM API.
	tokenIssued := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tokens":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			// Decode and verify credentials.
			var creds struct {
				Identity string `json:"identity"`
				Secret   string `json:"secret"`
			}
			if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if creds.Identity != "admin@home.lab" || creds.Secret != "test-password" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			tokenIssued = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"token":   "test-jwt-token",
				"expires": "2026-12-31T23:59:59Z",
			})

		case "/api/nginx/proxy-hosts":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			// Verify auth header.
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer test-jwt-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			hosts := []npmProxyHost{
				{
					ID:            1,
					DomainNames:   []string{"plex.home.lab"},
					ForwardHost:   "192.168.1.50",
					ForwardPort:   32400,
					ForwardScheme: "http",
					SSLForced:     true,
					Enabled:       true,
				},
				{
					ID:            2,
					DomainNames:   []string{"grafana.home.lab", "monitoring.home.lab"},
					ForwardHost:   "192.168.1.10",
					ForwardPort:   3000,
					ForwardScheme: "http",
					SSLForced:     false,
					Enabled:       true,
				},
				{
					ID:            3,
					DomainNames:   []string{"disabled.home.lab"},
					ForwardHost:   "192.168.1.99",
					ForwardPort:   8080,
					ForwardScheme: "http",
					SSLForced:     false,
					Enabled:       false, // disabled — should be filtered out
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(hosts)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &NPMProvider{
		client:   server.Client(),
		email:    "admin@home.lab",
		password: "test-password",
	}

	routes, err := provider.FetchRoutes(server.URL)
	if err != nil {
		t.Fatalf("FetchRoutes returned error: %v", err)
	}

	// Verify authentication happened.
	if !tokenIssued {
		t.Fatal("expected token endpoint to be called for authentication")
	}

	// Expected routes:
	// - plex.home.lab (host 1, single domain)
	// - grafana.home.lab (host 2, first domain)
	// - monitoring.home.lab (host 2, second domain)
	// - disabled.home.lab should NOT appear (host 3 is disabled)
	if len(routes) != 3 {
		t.Fatalf("got %d routes; want 3", len(routes))
	}

	// Route 0: plex.home.lab
	assertRoute(t, routes[0], "plex.home.lab", "http://192.168.1.50:32400", true, "npm-host-1")

	// Route 1: grafana.home.lab (first domain of host 2)
	assertRoute(t, routes[1], "grafana.home.lab", "http://192.168.1.10:3000", false, "npm-host-2")

	// Route 2: monitoring.home.lab (second domain of host 2)
	assertRoute(t, routes[2], "monitoring.home.lab", "http://192.168.1.10:3000", false, "npm-host-2")
}

func assertRoute(t *testing.T, route Route, domain, backendURL string, tls bool, routerName string) {
	t.Helper()
	if route.Domain != domain {
		t.Errorf("route.Domain = %q; want %q", route.Domain, domain)
	}
	if route.BackendURL != backendURL {
		t.Errorf("route.BackendURL = %q; want %q", route.BackendURL, backendURL)
	}
	if route.TLS != tls {
		t.Errorf("route.TLS = %v; want %v", route.TLS, tls)
	}
	if route.RouterName != routerName {
		t.Errorf("route.RouterName = %q; want %q", route.RouterName, routerName)
	}
}

// ---------------------------------------------------------------------------
// TestNPMFetchRoutesNoCredentials
// ---------------------------------------------------------------------------

func TestNPMFetchRoutesNoCredentials(t *testing.T) {
	t.Run("empty email and password", func(t *testing.T) {
		provider := &NPMProvider{
			client:   http.DefaultClient,
			email:    "",
			password: "",
		}

		_, err := provider.FetchRoutes("https://localhost:8181")
		if err == nil {
			t.Fatal("expected error when credentials are empty")
		}
		expected := "NPM credentials not configured (set LABTETHER_NPM_EMAIL and LABTETHER_NPM_PASSWORD)"
		if err.Error() != expected {
			t.Errorf("error = %q; want %q", err.Error(), expected)
		}
	})

	t.Run("empty email only", func(t *testing.T) {
		provider := &NPMProvider{
			client:   http.DefaultClient,
			email:    "",
			password: "some-password",
		}

		_, err := provider.FetchRoutes("https://localhost:8181")
		if err == nil {
			t.Fatal("expected error when email is empty")
		}
	})

	t.Run("empty password only", func(t *testing.T) {
		provider := &NPMProvider{
			client:   http.DefaultClient,
			email:    "admin@home.lab",
			password: "",
		}

		_, err := provider.FetchRoutes("https://localhost:8181")
		if err == nil {
			t.Fatal("expected error when password is empty")
		}
	})
}

// ---------------------------------------------------------------------------
// TestNPMFetchRoutesAuthFailure
// ---------------------------------------------------------------------------

func TestNPMFetchRoutesAuthFailure(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tokens" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Invalid credentials"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := &NPMProvider{
		client:   server.Client(),
		email:    "wrong@email.com",
		password: "wrong-password",
	}

	_, err := provider.FetchRoutes(server.URL)
	if err == nil {
		t.Fatal("expected error on auth failure")
	}
}

// ---------------------------------------------------------------------------
// TestNPMName
// ---------------------------------------------------------------------------

func TestNPMName(t *testing.T) {
	provider := &NPMProvider{}
	if provider.Name() != "npm" {
		t.Errorf("Name() = %q; want %q", provider.Name(), "npm")
	}
}
