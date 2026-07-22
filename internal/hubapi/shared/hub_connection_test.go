package shared

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testHubConnectionResolver() *HubConnectionResolverDeps {
	return &HubConnectionResolverDeps{
		DefaultPort: "8443",
		ListInterfaces: func() ([]net.Interface, error) {
			return []net.Interface{{Index: 1, Name: "eth0", Flags: net.FlagUp}}, nil
		},
		ListInterfaceAddrs: func(net.Interface) ([]net.Addr, error) {
			return []net.Addr{&net.IPNet{
				IP:   net.ParseIP("192.168.48.3"),
				Mask: net.CIDRMask(24, 32),
			}}, nil
		},
		TrustModePlainHTTP:       "plain_http",
		TrustModePublicTLS:       "public_tls",
		TrustModeLabtetherCA:     "labtether_ca",
		TrustModeCustomTLS:       "custom_tls",
		BootstrapStrategyInstall: "install",
	}
}

func TestResolveHubConnectionSelectionRequestOriginPriorityAndTransportIsolation(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.TLSEnabled = true
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "http", "console.example:3000", true
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"

	selection := deps.ResolveHubConnectionSelection(req)
	if selection.HubURL != "http://console.example:3000" || selection.WSURL != "ws://console.example:3000/ws/agent" {
		t.Fatalf("unexpected request origin selection: %+v", selection)
	}
	if len(selection.Candidates) != 2 {
		t.Fatalf("expected request and LAN candidates, got %+v", selection.Candidates)
	}
	if got := selection.Candidates[1].HubURL; got != "https://192.168.48.3:8443" {
		t.Fatalf("forwarded request transport changed direct interface candidate: %q", got)
	}
}

func TestResolveHubConnectionSelectionDirectTLSUsesSecureSchemes(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	req := httptest.NewRequest(http.MethodGet, "https://127.0.0.1:28443/api/v1/discover", nil)
	req.Host = "127.0.0.1:28443"
	if req.TLS == nil {
		// Keep this test explicit even if httptest's URL handling changes.
		req.TLS = &tls.ConnectionState{}
	}

	selection := deps.ResolveHubConnectionSelection(req)
	if selection.HubURL != "https://127.0.0.1:28443" {
		t.Fatalf("expected direct TLS hub URL, got %q", selection.HubURL)
	}
	if selection.WSURL != "wss://127.0.0.1:28443/ws/agent" {
		t.Fatalf("expected direct TLS websocket URL, got %q", selection.WSURL)
	}
	if len(selection.Candidates) != 2 || selection.Candidates[0].Kind != "request" {
		t.Fatalf("expected direct loopback request before enumerated LAN candidate, got %+v", selection.Candidates)
	}
	if got := selection.Candidates[1].HubURL; got != "http://192.168.48.3:28443" {
		t.Fatalf("expected non-TLS hub interface scheme to remain independent, got %q", got)
	}
}

func TestResolveHubConnectionSelectionForwardedOriginRequiresTrust(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.ListInterfaces = func() ([]net.Interface, error) { return nil, nil }
	deps.TLSEnabled = true
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "http", "attacker.example", false
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")

	selection := deps.ResolveHubConnectionSelection(req)
	if selection.HubURL != "https://labtether:8443" {
		t.Fatalf("untrusted forwarded origin was advertised: %q", selection.HubURL)
	}
}

func TestResolveHubConnectionSelectionForwardedHTTPSDoesNotInheritBuiltInCA(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.ListInterfaces = func() ([]net.Interface, error) { return nil, nil }
	deps.TLSEnabled = true
	deps.CACertPEM = []byte("internal hub CA")
	deps.CurrentTLSCertType = func() string { return "self-signed" }
	deps.BootstrapStrategyPinnedCA = "pinned_ca_bootstrap"
	deps.BuildPinnedBootstrapURL = func(string, []byte) string {
		return "https://proxy.example/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=internal"
	}
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "https", "proxy.example:443", true
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"

	selection := deps.ResolveHubConnectionSelection(req)
	if len(selection.Candidates) != 1 {
		t.Fatalf("expected one forwarded request candidate, got %+v", selection.Candidates)
	}
	candidate := selection.Candidates[0]
	if candidate.TrustMode != "custom_tls" {
		t.Fatalf("forwarded HTTPS trust mode=%q, want custom_tls", candidate.TrustMode)
	}
	if candidate.BootstrapStrategy != "install" {
		t.Fatalf("forwarded HTTPS bootstrap strategy=%q, want install", candidate.BootstrapStrategy)
	}
	if candidate.BootstrapURL != "" {
		t.Fatalf("forwarded HTTPS exposed internal-CA bootstrap URL %q", candidate.BootstrapURL)
	}
}

func TestResolveHubConnectionSelectionMatchingExternalForwardedHTTPSReclassifiesTrust(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.ListInterfaces = func() ([]net.Interface, error) { return nil, nil }
	deps.ExternalURL = "https://proxy.example"
	deps.TLSEnabled = true
	deps.CACertPEM = []byte("internal hub CA")
	deps.CurrentTLSCertType = func() string { return "self-signed" }
	deps.BootstrapStrategyPinnedCA = "pinned_ca_bootstrap"
	deps.BuildPinnedBootstrapURL = func(string, []byte) string {
		return "https://proxy.example/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=internal"
	}
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "https", "proxy.example:443", true
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"

	selection := deps.ResolveHubConnectionSelection(req)
	if len(selection.Candidates) != 1 || selection.Candidates[0].Kind != "external" {
		t.Fatalf("expected one deduplicated external candidate, got %+v", selection.Candidates)
	}
	candidate := selection.Candidates[0]
	if candidate.TrustMode != "custom_tls" || candidate.BootstrapStrategy != "install" || candidate.BootstrapURL != "" {
		t.Fatalf("matching forwarded external candidate retained internal-CA metadata: %+v", candidate)
	}
}

func TestResolveHubConnectionSelectionMatchingHostDifferentSchemeDoesNotReclassifyExternal(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.ListInterfaces = func() ([]net.Interface, error) { return nil, nil }
	deps.ExternalURL = "https://proxy.example"
	deps.TLSEnabled = true
	deps.CACertPEM = []byte("internal hub CA")
	deps.CurrentTLSCertType = func() string { return "self-signed" }
	deps.BootstrapStrategyPinnedCA = "pinned_ca_bootstrap"
	deps.BuildPinnedBootstrapURL = func(string, []byte) string {
		return "https://proxy.example/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=internal"
	}
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "http", "proxy.example", true
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"

	selection := deps.ResolveHubConnectionSelection(req)
	if len(selection.Candidates) != 2 {
		t.Fatalf("expected distinct HTTPS external and HTTP request origins, got %+v", selection.Candidates)
	}
	external := selection.Candidates[0]
	if external.Kind != "external" || external.TrustMode != "labtether_ca" ||
		external.BootstrapStrategy != "pinned_ca_bootstrap" || external.BootstrapURL == "" {
		t.Fatalf("scheme-mismatched forwarding changed external trust metadata: %+v", external)
	}
	request := selection.Candidates[1]
	if request.Kind != "request" || request.HubURL != "http://proxy.example" || request.TrustMode != "plain_http" {
		t.Fatalf("expected separate plain-HTTP request origin, got %+v", request)
	}
}

func TestResolveHubConnectionSelectionMatchingHealthyTailscalePreservesPublicTrust(t *testing.T) {
	t.Parallel()

	deps := testHubConnectionResolver()
	deps.ListInterfaces = func() ([]net.Interface, error) { return nil, nil }
	deps.TLSEnabled = true
	deps.CACertPEM = []byte("internal hub CA")
	deps.CurrentTLSCertType = func() string { return "self-signed" }
	deps.InspectTailscaleServeStatus = func() HubTailscaleServeStatus {
		return HubTailscaleServeStatus{
			DesiredMode:     "serve",
			ServeConfigured: true,
			TSNetURL:        "https://hub.example.ts.net",
		}
	}
	deps.ResolveTrustedForwardedRequestOrigin = func(*http.Request) (string, string, bool) {
		return "https", "hub.example.ts.net:443", true
	}
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"

	selection := deps.ResolveHubConnectionSelection(req)
	if len(selection.Candidates) != 1 {
		t.Fatalf("expected one deduplicated Tailscale candidate, got %+v", selection.Candidates)
	}
	candidate := selection.Candidates[0]
	if candidate.Kind != "tailscale_https" || candidate.TrustMode != "public_tls" || candidate.BootstrapStrategy != "install" {
		t.Fatalf("healthy Tailscale public metadata was overwritten: %+v", candidate)
	}
}
