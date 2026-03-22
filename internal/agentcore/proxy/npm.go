package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
	"github.com/labtether/labtether/internal/securityruntime"
)

// ---------------------------------------------------------------------------
// NPM API response types
// ---------------------------------------------------------------------------

type npmTokenResponse struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

type npmProxyHost struct {
	ID            int      `json:"id"`
	DomainNames   []string `json:"domain_names"`
	ForwardHost   string   `json:"forward_host"`
	ForwardPort   int      `json:"forward_port"`
	ForwardScheme string   `json:"forward_scheme"`
	SSLForced     bool     `json:"ssl_forced"`
	Enabled       bool     `json:"enabled"`
}

// ---------------------------------------------------------------------------
// NPMProvider — reverse proxy discovery via Nginx Proxy Manager API
// ---------------------------------------------------------------------------

// NPMProvider discovers routing rules from Nginx Proxy Manager.
type NPMProvider struct {
	client      *http.Client
	email       string
	password    string
	manualURL   string // set via LABTETHER_NPM_URL to skip container detection
	cachedToken string
	tokenExpiry time.Time
}

// npmAdminPort is the default NPM admin API port.
const npmAdminPort = 81

// NewNPMProvider creates an NPMProvider with credentials from environment variables.
func NewNPMProvider() *NPMProvider {
	manualURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LABTETHER_NPM_URL")), "/")
	manualURL = normalizeHTTPSURL(manualURL)
	return &NPMProvider{
		client:    &http.Client{Timeout: 5 * time.Second},
		email:     os.Getenv("LABTETHER_NPM_EMAIL"),
		password:  os.Getenv("LABTETHER_NPM_PASSWORD"),
		manualURL: manualURL,
	}
}

// Name returns the provider identifier.
func (p *NPMProvider) Name() string {
	return "npm"
}

// DetectAndConnect inspects running containers for an NPM instance with
// admin port 81 exposed.
func (p *NPMProvider) DetectAndConnect(containers []dockerpkg.DockerContainer) (string, bool) {
	// Manual URL override takes precedence over container detection.
	if p.manualURL != "" {
		return p.manualURL, true
	}

	for _, c := range containers {
		normalized := normalizeDockerImage(c.Image)
		if normalized != "jc21/nginx-proxy-manager" {
			continue
		}

		hostPort := FindHostPort(c.Ports, npmAdminPort)
		if hostPort == 0 {
			log.Printf("[webservices] NPM container %s found but admin port %d not exposed — trying next container",
				c.ID, npmAdminPort)
			continue
		}

		scheme := "https"
		if allowInsecureTransportOptIn() {
			scheme = "http"
		}
		apiURL := fmt.Sprintf("%s://localhost:%d", scheme, hostPort)
		log.Printf("[webservices] NPM detected at %s (container %s)", apiURL, c.ID)
		return apiURL, true
	}
	return "", false
}

// FetchRoutes queries the NPM API and returns discovered proxy routes.
// NPM requires authentication, so this is a two-step process:
//  1. Authenticate via POST /api/tokens to get a JWT token.
//  2. Fetch proxy hosts via GET /api/nginx/proxy-hosts with the token.
func (p *NPMProvider) FetchRoutes(apiURL string) ([]Route, error) {
	if p.email == "" || p.password == "" {
		return nil, fmt.Errorf("NPM credentials not configured (set LABTETHER_NPM_EMAIL and LABTETHER_NPM_PASSWORD)")
	}

	// Step 1: Authenticate to get a JWT token (cached across cycles).
	token, err := p.getToken(apiURL)
	if err != nil {
		return nil, fmt.Errorf("NPM authentication failed: %w", err)
	}

	// Step 2: Fetch proxy hosts with the token.
	hosts, err := p.fetchProxyHosts(apiURL, token)
	if err != nil {
		return nil, fmt.Errorf("NPM proxy hosts fetch failed: %w", err)
	}

	// Step 3: Convert enabled proxy hosts to Routes.
	var routes []Route
	for _, host := range hosts {
		if !host.Enabled {
			continue
		}

		backendURL := fmt.Sprintf("%s://%s:%d", host.ForwardScheme, host.ForwardHost, host.ForwardPort)
		routerName := fmt.Sprintf("npm-host-%d", host.ID)

		for _, domain := range host.DomainNames {
			routes = append(routes, Route{
				Domain:     domain,
				BackendURL: backendURL,
				TLS:        host.SSLForced,
				RouterName: routerName,
			})
		}
	}

	return routes, nil
}

// getToken returns a cached JWT token if still valid, otherwise authenticates.
func (p *NPMProvider) getToken(apiURL string) (string, error) {
	if p.cachedToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.cachedToken, nil
	}
	token, err := p.authenticate(apiURL)
	if err != nil {
		return "", err
	}
	p.cachedToken = token
	// NPM tokens typically last 24h; cache conservatively for 1h.
	p.tokenExpiry = time.Now().Add(1 * time.Hour)
	return token, nil
}

// authenticate posts credentials to NPM's token endpoint and returns the JWT token.
func (p *NPMProvider) authenticate(apiURL string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"identity": p.email,
		"secret":   p.password,
	})
	if err != nil {
		return "", fmt.Errorf("marshal credentials: %w", err)
	}

	tokenURL, err := npmEndpointURL(apiURL, "/api/tokens")
	if err != nil {
		return "", err
	}
	req, err := securityruntime.NewOutboundRequestWithContext(context.Background(), http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := securityruntime.DoOutboundRequest(p.client, req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}

	var tokenResp npmTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return tokenResp.Token, nil
}

// fetchProxyHosts queries the NPM proxy hosts endpoint with bearer auth.
func (p *NPMProvider) fetchProxyHosts(apiURL, token string) ([]npmProxyHost, error) {
	hostsURL, err := npmEndpointURL(apiURL, "/api/nginx/proxy-hosts")
	if err != nil {
		return nil, err
	}
	req, err := securityruntime.NewOutboundRequestWithContext(context.Background(), http.MethodGet, hostsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := securityruntime.DoOutboundRequest(p.client, req)
	if err != nil {
		return nil, fmt.Errorf("proxy hosts request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy hosts endpoint returned HTTP %d", resp.StatusCode)
	}

	var hosts []npmProxyHost
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		return nil, fmt.Errorf("decode proxy hosts: %w", err)
	}

	return hosts, nil
}

func npmEndpointURL(base, path string) (string, error) {
	trimmedBase := strings.TrimSpace(base)
	if trimmedBase == "" {
		return "", fmt.Errorf("NPM API url is required")
	}
	parsed, err := url.Parse(trimmedBase)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid NPM API url %q", base)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
