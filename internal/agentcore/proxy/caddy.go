package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// Caddy JSON config response types
// ---------------------------------------------------------------------------

type caddyConfig struct {
	Apps struct {
		HTTP struct {
			Servers map[string]caddyServer `json:"servers"`
		} `json:"http"`
	} `json:"apps"`
}

type caddyServer struct {
	Listen []string     `json:"listen"`
	Routes []caddyRoute `json:"routes"`
}

type caddyRoute struct {
	Match  []caddyMatch   `json:"match"`
	Handle []caddyHandler `json:"handle"`
}

type caddyMatch struct {
	Host []string `json:"host"`
}

type caddyHandler struct {
	Handler   string          `json:"handler"`
	Upstreams []caddyUpstream `json:"upstreams,omitempty"`
	Routes    []caddyRoute    `json:"routes,omitempty"`
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}

// ---------------------------------------------------------------------------
// CaddyProvider — reverse proxy discovery via Caddy admin API
// ---------------------------------------------------------------------------

// CaddyProvider discovers routing rules from a Caddy reverse proxy by
// querying its admin API on port 2019. The admin API must be explicitly
// exposed by the user (it is not exposed by default).
type CaddyProvider struct {
	client    *http.Client
	manualURL string // set via LABTETHER_CADDY_URL to skip container detection
}

// NewCaddyProvider creates a CaddyProvider with a 5-second timeout HTTP client.
func NewCaddyProvider() *CaddyProvider {
	return &CaddyProvider{
		client:    &http.Client{Timeout: 5 * time.Second},
		manualURL: strings.TrimRight(os.Getenv("LABTETHER_CADDY_URL"), "/"),
	}
}

// Name returns the provider identifier.
func (p *CaddyProvider) Name() string {
	return "caddy"
}

// caddyAdminPort is the default Caddy admin API port.
const caddyAdminPort = 2019

// caddyImages are the Docker images recognized as Caddy.
var caddyImages = []string{"caddy", "lucaslorentz/caddy-docker-proxy"}

// DetectAndConnect inspects running containers for a Caddy instance with
// admin port 2019 exposed. Returns the admin API URL and true if found.
func (p *CaddyProvider) DetectAndConnect(containers []dockerpkg.DockerContainer) (string, bool) {
	// Manual URL override takes precedence over container detection.
	if p.manualURL != "" {
		return p.manualURL, true
	}

	for _, c := range containers {
		normalized := normalizeDockerImage(c.Image)
		isCaddy := false
		for _, img := range caddyImages {
			if normalized == img {
				isCaddy = true
				break
			}
		}
		if !isCaddy {
			continue
		}

		hostPort := FindHostPort(c.Ports, caddyAdminPort)
		if hostPort == 0 {
			log.Printf("[webservices] caddy container %s found but admin port %d not exposed — trying next container",
				c.ID, caddyAdminPort)
			continue
		}

		apiURL := fmt.Sprintf("http://localhost:%d", hostPort)
		log.Printf("[webservices] caddy detected at %s (container %s)", apiURL, c.ID)
		return apiURL, true
	}
	return "", false
}

// caddyTLSPorts are the listen port suffixes that indicate TLS termination.
var caddyTLSPorts = map[string]bool{
	":443":   true,
	":8443":  true,
	":9443":  true,
	":10443": true,
}

// isServerTLS checks if any of the server's listen addresses use a TLS port.
func isServerTLS(listen []string) bool {
	for _, addr := range listen {
		// Listen addresses may be ":443" or "0.0.0.0:443" or "[::]:443".
		// We check if the address ends with a known TLS port suffix.
		for tlsPort := range caddyTLSPorts {
			if strings.HasSuffix(addr, tlsPort) {
				return true
			}
		}
	}
	return false
}

// FetchRoutes queries the Caddy admin API and returns discovered proxy routes.
func (p *CaddyProvider) FetchRoutes(apiURL string) ([]Route, error) {
	resp, err := p.client.Get(apiURL + "/config/")
	if err != nil {
		return nil, fmt.Errorf("caddy config request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("caddy config returned status %d", resp.StatusCode)
	}

	var config caddyConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("caddy config decode failed: %w", err)
	}

	var routes []Route

	for serverName, server := range config.Apps.HTTP.Servers {
		tls := isServerTLS(server.Listen)

		for _, route := range server.Routes {
			backendURL := firstCaddyBackendURL(route.Handle)
			if backendURL == "" {
				continue // skip non-reverse_proxy routes (static_response, etc.)
			}

			// Extract hosts from match rules.
			var hosts []string
			for _, m := range route.Match {
				hosts = append(hosts, m.Host...)
			}

			// Create a route for each host.
			for _, host := range hosts {
				routes = append(routes, Route{
					Domain:     host,
					BackendURL: backendURL,
					TLS:        tls,
					RouterName: serverName,
				})
			}
		}
	}

	return routes, nil
}

func firstCaddyBackendURL(handlers []caddyHandler) string {
	for _, handler := range handlers {
		if handler.Handler == "reverse_proxy" && len(handler.Upstreams) > 0 && handler.Upstreams[0].Dial != "" {
			return "http://" + handler.Upstreams[0].Dial
		}
		if len(handler.Routes) > 0 {
			for _, nested := range handler.Routes {
				if backend := firstCaddyBackendURL(nested.Handle); backend != "" {
					return backend
				}
			}
		}
	}
	return ""
}
