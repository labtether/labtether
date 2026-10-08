package main

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/certmgr"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type discoverResponse struct {
	Hub           string                   `json:"hub"`
	APIURL        string                   `json:"api_url"`
	WSURL         string                   `json:"ws_url"`
	EnrollURL     string                   `json:"enroll_url"`
	HubURL        string                   `json:"hub_url"`
	HubWSURL      string                   `json:"hub_ws_url"`
	HubCandidates []hubConnectionCandidate `json:"hub_candidates"`
}

func disableTailscaleResolutionForTest(t *testing.T) {
	t.Helper()

	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(string) (string, error) {
		return "", net.ErrClosed
	}
	tailscaleRunner = func(time.Duration, string, ...string) ([]byte, error) {
		t.Fatalf("tailscale CLI should not run when disabled for this test")
		return nil, nil
	}
	tailscaleFallbackPaths = func() []string { return nil }
}

func TestDiscoverEndpoint(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.Hub != "labtether" {
		t.Fatalf("expected hub=labtether, got %q", resp.Hub)
	}
	if resp.APIURL != "http://localhost:8080" {
		t.Fatalf("expected api_url=http://localhost:8080, got %q", resp.APIURL)
	}
	if resp.WSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected ws_url=ws://localhost:8080/ws/agent, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "http://localhost:8080/api/v1/enroll" {
		t.Fatalf("expected enroll_url, got %q", resp.EnrollURL)
	}
	if resp.HubURL != resp.APIURL {
		t.Fatalf("expected hub_url to mirror api_url, got hub_url=%q api_url=%q", resp.HubURL, resp.APIURL)
	}
	if resp.HubWSURL != resp.WSURL {
		t.Fatalf("expected hub_ws_url to mirror ws_url, got hub_ws_url=%q ws_url=%q", resp.HubWSURL, resp.WSURL)
	}
	if len(resp.HubCandidates) == 0 {
		t.Fatalf("expected discover response to include hub_candidates")
	}
}

func TestDiscoverEndpoint_UsesExternalURLForHubEndpoints(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.externalURL = "https://hub.example.com:9443"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "attacker.local:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://hub.example.com:9443" {
		t.Fatalf("expected api_url to use sanitized external host, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://hub.example.com:9443/ws/agent" {
		t.Fatalf("expected ws_url to use sanitized external host, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://hub.example.com:9443/api/v1/enroll" {
		t.Fatalf("expected enroll_url to use sanitized external host, got %q", resp.EnrollURL)
	}
}

func TestDiscoverEndpoint_IgnoresHTTPExternalURLWhenTLSEnabled(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.externalURL = "http://hub.example.com:8080"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "secure.local:8443"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://secure.local:8443" {
		t.Fatalf("expected api_url to fall back to secure request host, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://secure.local:8443/ws/agent" {
		t.Fatalf("expected ws_url to fall back to secure request host, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://secure.local:8443/api/v1/enroll" {
		t.Fatalf("expected enroll_url to fall back to secure request host, got %q", resp.EnrollURL)
	}
}

func TestDiscoverEndpoint_PrefersExactRequestOriginOverDiscoveredCandidates(t *testing.T) {
	t.Setenv("API_PORT", "8080")

	disableTailscaleResolutionForTest(t)
	withMockHubInterfaces(t,
		[]net.Interface{
			{Index: 1, Name: "tailscale0", Flags: net.FlagUp},
			{Index: 2, Name: "en0", Flags: net.FlagUp},
		},
		map[int][]net.Addr{
			1: {ipNet("100.96.0.5/32")},
			2: {ipNet("192.168.1.40/24")},
		},
	)

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.HubURL != "http://localhost:8080" {
		t.Fatalf("expected exact request-origin hub_url, got %q", resp.HubURL)
	}
	if len(resp.HubCandidates) < 3 {
		t.Fatalf("expected at least three hub candidates, got %d", len(resp.HubCandidates))
	}
	if resp.HubCandidates[0].Kind != "request" {
		t.Fatalf("expected first hub candidate kind request, got %q", resp.HubCandidates[0].Kind)
	}
	if resp.HubCandidates[1].Kind != "tailscale" {
		t.Fatalf("expected discovered tailscale candidate to remain available, got %q", resp.HubCandidates[1].Kind)
	}
}

func TestDiscoverEndpoint_UsesTrustedForwardedHTTPOrigin(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "console.example:3000")
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "http://console.example:3000" {
		t.Fatalf("expected api_url to use trusted public origin, got %q", resp.APIURL)
	}
	if resp.WSURL != "ws://console.example:3000/ws/agent" {
		t.Fatalf("expected ws_url to use trusted public origin, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "http://console.example:3000/api/v1/enroll" {
		t.Fatalf("expected enroll_url to use trusted public origin, got %q", resp.EnrollURL)
	}
}

func TestDiscoverEndpoint_BuiltInCAForwardedHTTPSUsesOperatorManagedTrust(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	sut := newTestAPIServer(t)
	sut.externalURL = "https://proxy.example"
	sut.tlsState.Enabled = true
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "proxy.example:443")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://proxy.example" || resp.WSURL != "wss://proxy.example/ws/agent" {
		t.Fatalf("unexpected forwarded discover origin: %+v", resp)
	}
	if len(resp.HubCandidates) != 1 {
		t.Fatalf("expected one forwarded candidate, got %+v", resp.HubCandidates)
	}
	candidate := resp.HubCandidates[0]
	if candidate.Kind != "external" {
		t.Fatalf("expected matching external candidate to remain deduplicated, got kind=%q", candidate.Kind)
	}
	if candidate.TrustMode != tlsTrustModeCustomTLS {
		t.Fatalf("discover trust_mode=%q, want %q", candidate.TrustMode, tlsTrustModeCustomTLS)
	}
	if candidate.BootstrapStrategy != tlsBootstrapStrategyInstall {
		t.Fatalf("discover bootstrap_strategy=%q, want %q", candidate.BootstrapStrategy, tlsBootstrapStrategyInstall)
	}
	if candidate.BootstrapURL != "" {
		t.Fatalf("discover exposed internal-CA bootstrap URL %q", candidate.BootstrapURL)
	}
}

func TestBuildHTTPHandlers_DiscoverEndpointAllowsUnauthenticatedRequests(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	discoverHandler, ok := handlers["/api/v1/discover"]
	if !ok || discoverHandler == nil {
		t.Fatalf("expected /api/v1/discover handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	discoverHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected unauthenticated discover request to return 200, got %d", rec.Code)
	}
}

func TestBuildHTTPHandlers_DiscoverEndpointRejectsNonGETWithoutAuth(t *testing.T) {
	sut := newTestAPIServer(t)
	handlers := sut.buildHTTPHandlers(nil, nil, nil)

	discoverHandler, ok := handlers["/api/v1/discover"]
	if !ok || discoverHandler == nil {
		t.Fatalf("expected /api/v1/discover handler")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	rec := httptest.NewRecorder()
	discoverHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected discover POST to return 405 without auth, got %d", rec.Code)
	}
}

func TestDiscoverRejectsNonGET(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discover", nil)
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHubSchemes_PlainHTTP(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = false

	httpScheme, wsScheme := sut.hubSchemes()
	if httpScheme != "http" {
		t.Fatalf("expected http, got %q", httpScheme)
	}
	if wsScheme != "ws" {
		t.Fatalf("expected ws, got %q", wsScheme)
	}
}

func TestHubSchemes_TLS(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	httpScheme, wsScheme := sut.hubSchemes()
	if httpScheme != "https" {
		t.Fatalf("expected https, got %q", httpScheme)
	}
	if wsScheme != "wss" {
		t.Fatalf("expected wss, got %q", wsScheme)
	}
}

func TestDiscoverEndpoint_TLS(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	sut.handleDiscover(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp discoverResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	if resp.APIURL != "https://localhost:8080" {
		t.Fatalf("expected https api_url, got %q", resp.APIURL)
	}
	if resp.WSURL != "wss://localhost:8080/ws/agent" {
		t.Fatalf("expected wss ws_url, got %q", resp.WSURL)
	}
	if resp.EnrollURL != "https://localhost:8080/api/v1/enroll" {
		t.Fatalf("expected https enroll_url, got %q", resp.EnrollURL)
	}
}
