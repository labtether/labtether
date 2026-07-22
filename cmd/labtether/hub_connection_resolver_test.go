package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/certmgr"
)

func withMockHubInterfaces(t *testing.T, ifaces []net.Interface, addrs map[int][]net.Addr) {
	t.Helper()
	origList := listHubInterfaces
	origAddrs := listHubInterfaceAddrs
	listHubInterfaces = func() ([]net.Interface, error) {
		return ifaces, nil
	}
	listHubInterfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		return addrs[iface.Index], nil
	}
	t.Cleanup(func() {
		listHubInterfaces = origList
		listHubInterfaceAddrs = origAddrs
	})
}

func ipNet(cidr string) *net.IPNet {
	_, network, _ := net.ParseCIDR(cidr)
	return network
}

func withHealthyTailscaleHTTPSUnavailable(t *testing.T) {
	t.Helper()

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	tailscaleLookPath = func(file string) (string, error) {
		return "", errors.New("tailscale not installed")
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		return nil, errors.New("tailscale unavailable")
	}
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
	})
}

func TestResolveHubConnectionSelectionPrefersExactRequestOriginOverDiscoveredInterfaces(t *testing.T) {
	t.Setenv("API_PORT", "8080")
	withHealthyTailscaleHTTPSUnavailable(t)

	withMockHubInterfaces(t,
		[]net.Interface{
			{Index: 1, Name: "tailscale0", Flags: net.FlagUp},
			{Index: 2, Name: "en0", Flags: net.FlagUp},
		},
		map[int][]net.Addr{
			1: {ipNet("100.101.102.103/32")},
			2: {ipNet("192.168.50.10/24")},
		},
	)

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	req.Host = "localhost:8080"

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "http://localhost:8080" {
		t.Fatalf("expected exact request-origin hub URL, got %q", selection.HubURL)
	}
	if selection.WSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected exact request-origin ws URL, got %q", selection.WSURL)
	}
	if len(selection.Candidates) < 3 {
		t.Fatalf("expected multiple hub candidates, got %d", len(selection.Candidates))
	}
	if selection.Candidates[0].Kind != "request" {
		t.Fatalf("expected first candidate to be the exact request origin, got %q", selection.Candidates[0].Kind)
	}
	if selection.Candidates[1].Kind != "tailscale" {
		t.Fatalf("expected discovered tailscale candidate to remain available, got %q", selection.Candidates[1].Kind)
	}
}

func TestResolveHubConnectionSelectionPrefersHealthyTailscaleHTTPSURL(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		"remote_access.mode": "serve",
	}); err != nil {
		t.Fatalf("failed to seed runtime settings: %v", err)
	}

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": { "DNSName": "hub.homelab.ts.net." }
			}`), nil
		case "serve status --json":
			return []byte(`{"TCP":{"443":{"HTTPS":true,"Web":{"/":{"Proxy":"https://127.0.0.1:8443"}}}}}`), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	req.Host = "localhost:8080"

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "https://hub.homelab.ts.net" {
		t.Fatalf("expected ts.net hub URL, got %q", selection.HubURL)
	}
	if selection.WSURL != "wss://hub.homelab.ts.net/ws/agent" {
		t.Fatalf("expected ts.net ws URL, got %q", selection.WSURL)
	}
	if len(selection.Candidates) == 0 || selection.Candidates[0].Kind != "tailscale_https" {
		t.Fatalf("expected first candidate kind tailscale_https, got %+v", selection.Candidates)
	}
	if selection.Candidates[0].TrustMode != tlsTrustModePublicTLS {
		t.Fatalf("expected public TLS trust mode, got %q", selection.Candidates[0].TrustMode)
	}
	if selection.Candidates[0].BootstrapStrategy != tlsBootstrapStrategyInstall {
		t.Fatalf("expected install bootstrap strategy, got %q", selection.Candidates[0].BootstrapStrategy)
	}
	if strings.TrimSpace(selection.Candidates[0].PreferredReason) == "" {
		t.Fatalf("expected preferred reason for healthy tailscale https candidate")
	}
}

func TestEnrollmentTokensIncludesHubCandidates(t *testing.T) {
	t.Setenv("API_PORT", "8080")
	withHealthyTailscaleHTTPSUnavailable(t)

	withMockHubInterfaces(t,
		[]net.Interface{
			{Index: 1, Name: "tailscale0", Flags: net.FlagUp},
			{Index: 2, Name: "eth0", Flags: net.FlagUp},
		},
		map[int][]net.Addr{
			1: {ipNet("100.96.0.5/32")},
			2: {ipNet("10.0.0.20/24")},
		},
	)

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()

	sut.handleEnrollmentTokens(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		HubURL        string                   `json:"hub_url"`
		WSURL         string                   `json:"ws_url"`
		HubCandidates []hubConnectionCandidate `json:"hub_candidates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.HubURL != "http://localhost:8080" {
		t.Fatalf("expected exact request-origin hub_url, got %q", payload.HubURL)
	}
	if payload.WSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected exact request-origin ws_url, got %q", payload.WSURL)
	}
	if len(payload.HubCandidates) < 3 {
		t.Fatalf("expected at least 3 hub candidates, got %d", len(payload.HubCandidates))
	}
}

func TestResolveHubConnectionSelectionUsesTrustedForwardedHTTPOriginWithoutChangingInterfaceTLS(t *testing.T) {
	t.Setenv("API_PORT", "8443")
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t,
		[]net.Interface{{Index: 1, Name: "eth0", Flags: net.FlagUp}},
		map[int][]net.Addr{
			1: {&net.IPNet{IP: net.ParseIP("192.168.48.3"), Mask: net.CIDRMask(24, 32)}},
		},
	)

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "console.example:3000")
	req.Header.Set("X-Forwarded-Proto", "http")

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "http://console.example:3000" {
		t.Fatalf("expected trusted public HTTP origin, got %q", selection.HubURL)
	}
	if selection.WSURL != "ws://console.example:3000/ws/agent" {
		t.Fatalf("expected trusted public WS origin, got %q", selection.WSURL)
	}
	if len(selection.Candidates) != 2 {
		t.Fatalf("expected request and interface candidates, got %+v", selection.Candidates)
	}
	if got := selection.Candidates[1].HubURL; got != "https://192.168.48.3:8443" {
		t.Fatalf("forwarded scheme or port leaked into interface candidate: %q", got)
	}
	if got := selection.Candidates[1].WSURL; got != "wss://192.168.48.3:8443/ws/agent" {
		t.Fatalf("forwarded scheme or port leaked into interface websocket candidate: %q", got)
	}
}

func TestResolveHubConnectionSelectionIgnoresForwardingFromUntrustedSource(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "198.51.100.25:40000"
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "https://labtether:8443" {
		t.Fatalf("expected direct TLS origin for untrusted forwarded headers, got %q", selection.HubURL)
	}
	if selection.WSURL != "wss://labtether:8443/ws/agent" {
		t.Fatalf("expected direct TLS websocket origin for untrusted forwarded headers, got %q", selection.WSURL)
	}
}

func TestResolveHubConnectionSelectionRejectsMalformedTrustedForwardedOrigins(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	tests := []struct {
		name           string
		host           string
		proto          string
		forwarded      string
		extraHost      string
		extraProto     string
		extraForwarded string
	}{
		{name: "host path", host: "attacker.example/path", proto: "https"},
		{name: "host credentials", host: "user@attacker.example", proto: "https"},
		{name: "invalid proto", host: "attacker.example", proto: "javascript"},
		{name: "missing proto", host: "attacker.example"},
		{name: "missing host", proto: "https"},
		{name: "unbracketed ipv6", host: "2001:db8::1", proto: "https"},
		{
			name:      "malformed x pair does not fall through to forwarded",
			host:      "attacker.example",
			forwarded: "for=127.0.0.1;proto=http;host=attacker.example",
		},
		{name: "standard unterminated host quote", forwarded: `for=127.0.0.1;proto=https;host="attacker.example`},
		{name: "standard duplicate host", forwarded: "for=127.0.0.1;proto=https;host=one.example;host=two.example"},
		{name: "standard duplicate proto", forwarded: "for=127.0.0.1;proto=https;proto=http;host=attacker.example"},
		{name: "standard missing host", forwarded: "for=127.0.0.1;proto=https"},
		{name: "standard missing proto", forwarded: "for=127.0.0.1;host=attacker.example"},
		{name: "x forwarded host list", host: "attacker.example, proxy.example", proto: "https"},
		{name: "x forwarded proto list", host: "proxy.example", proto: "http, https"},
		{name: "duplicate x forwarded host headers", host: "attacker.example", extraHost: "proxy.example", proto: "https"},
		{name: "duplicate x forwarded proto headers", host: "proxy.example", proto: "http", extraProto: "https"},
		{
			name:      "standard forwarded list",
			forwarded: "for=198.51.100.9;proto=http;host=attacker.example, for=127.0.0.1;proto=https;host=proxy.example",
		},
		{
			name:           "duplicate standard forwarded headers",
			forwarded:      "for=198.51.100.9;proto=http;host=attacker.example",
			extraForwarded: "for=127.0.0.1;proto=https;host=proxy.example",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			sut.tlsState.Enabled = true
			req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/api/v1/discover", nil)
			req.Host = "labtether:8443"
			req.RemoteAddr = "127.0.0.1:40000"
			if tc.host != "" {
				req.Header.Set("X-Forwarded-Host", tc.host)
			}
			if tc.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.proto)
			}
			if tc.forwarded != "" {
				req.Header.Set("Forwarded", tc.forwarded)
			}
			if tc.extraHost != "" {
				req.Header.Add("X-Forwarded-Host", tc.extraHost)
			}
			if tc.extraProto != "" {
				req.Header.Add("X-Forwarded-Proto", tc.extraProto)
			}
			if tc.extraForwarded != "" {
				req.Header.Add("Forwarded", tc.extraForwarded)
			}

			selection := sut.resolveHubConnectionSelection(req)
			if selection.HubURL != "https://labtether:8443" {
				t.Fatalf("malformed forwarded origin was advertised: %q", selection.HubURL)
			}
		})
	}
}

func TestResolveHubConnectionSelectionUsesTrustedStandardForwardedOrigin(t *testing.T) {
	t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", "10.0.0.0/8")
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://labtether:8080/api/v1/discover", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "10.0.0.15:40000"
	req.Header.Set("Forwarded", `for=10.0.0.10;proto="https";host="console.example:9443"`)

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "https://console.example:9443" {
		t.Fatalf("expected trusted standard Forwarded origin, got %q", selection.HubURL)
	}
	if selection.WSURL != "wss://console.example:9443/ws/agent" {
		t.Fatalf("expected trusted standard Forwarded websocket origin, got %q", selection.WSURL)
	}
}

func TestResolveHubConnectionSelection_IgnoresHTTPExternalURLWhenTLSEnabled(t *testing.T) {
	t.Setenv("API_PORT", "8443")
	withHealthyTailscaleHTTPSUnavailable(t)

	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.externalURL = "http://hub.example.com:8080"

	req := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	req.Host = "secure.local:8443"

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "https://secure.local:8443" {
		t.Fatalf("expected secure request host hub URL, got %q", selection.HubURL)
	}
	if selection.WSURL != "wss://secure.local:8443/ws/agent" {
		t.Fatalf("expected secure request host ws URL, got %q", selection.WSURL)
	}
	if len(selection.Candidates) == 0 {
		t.Fatalf("expected at least one candidate")
	}
	if selection.Candidates[0].Kind == "external" {
		t.Fatalf("expected insecure external URL candidate to be ignored")
	}
}

func TestResolveHubConnectionSelection_BuiltInTLSUsesPinnedBootstrap(t *testing.T) {
	t.Setenv("API_PORT", "8443")
	withHealthyTailscaleHTTPSUnavailable(t)

	withMockHubInterfaces(t,
		[]net.Interface{
			{Index: 1, Name: "tailscale0", Flags: net.FlagUp},
		},
		map[int][]net.Addr{
			1: {ipNet("100.96.0.5/32")},
		},
	)

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)

	req := httptest.NewRequest(http.MethodGet, "/settings/enrollment", nil)
	req.Host = "localhost:8443"

	selection := sut.resolveHubConnectionSelection(req)
	if len(selection.Candidates) == 0 {
		t.Fatalf("expected at least one hub candidate")
	}
	candidate := selection.Candidates[0]
	if candidate.TrustMode != tlsTrustModeLabtetherCA {
		t.Fatalf("expected trust_mode=%q, got %q", tlsTrustModeLabtetherCA, candidate.TrustMode)
	}
	if candidate.BootstrapStrategy != tlsBootstrapStrategyPinnedCA {
		t.Fatalf("expected bootstrap_strategy=%q, got %q", tlsBootstrapStrategyPinnedCA, candidate.BootstrapStrategy)
	}
	if !strings.Contains(candidate.BootstrapURL, "/api/v1/agent/bootstrap.sh?ca_fingerprint_sha256=") {
		t.Fatalf("expected pinned bootstrap URL, got %q", candidate.BootstrapURL)
	}
	if strings.TrimSpace(candidate.PreferredReason) == "" {
		t.Fatalf("expected preferred_reason to be populated")
	}
}

func TestResolveHubConnectionSelection_BuiltInTLSForwardedHTTPSUsesOperatorManagedTrust(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)

	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/settings/enrollment", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "proxy.example")
	req.Header.Set("X-Forwarded-Proto", "https")

	selection := sut.resolveHubConnectionSelection(req)
	if selection.HubURL != "https://proxy.example" || selection.WSURL != "wss://proxy.example/ws/agent" {
		t.Fatalf("unexpected forwarded HTTPS selection: %+v", selection)
	}
	if len(selection.Candidates) != 1 {
		t.Fatalf("expected one forwarded request candidate, got %+v", selection.Candidates)
	}
	candidate := selection.Candidates[0]
	if candidate.TrustMode != tlsTrustModeCustomTLS {
		t.Fatalf("expected trust_mode=%q, got %q", tlsTrustModeCustomTLS, candidate.TrustMode)
	}
	if candidate.BootstrapStrategy != tlsBootstrapStrategyInstall {
		t.Fatalf("expected bootstrap_strategy=%q, got %q", tlsBootstrapStrategyInstall, candidate.BootstrapStrategy)
	}
	if candidate.BootstrapURL != "" {
		t.Fatalf("forwarded HTTPS origin exposed internal-CA bootstrap URL %q", candidate.BootstrapURL)
	}
	if !strings.Contains(strings.ToLower(candidate.PreferredReason), "reverse proxy") {
		t.Fatalf("expected reverse-proxy trust explanation, got %q", candidate.PreferredReason)
	}
}

func TestResolveHubConnectionSelection_BuiltInTLSMatchingExternalForwardedHTTPSReclassifiesTrust(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	sut := newTestAPIServer(t)
	sut.externalURL = "https://proxy.example"
	sut.tlsState.Enabled = true
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)

	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/settings/enrollment", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "proxy.example:443")
	req.Header.Set("X-Forwarded-Proto", "https")

	selection := sut.resolveHubConnectionSelection(req)
	if len(selection.Candidates) != 1 || selection.Candidates[0].Kind != "external" {
		t.Fatalf("expected one deduplicated external candidate, got %+v", selection.Candidates)
	}
	candidate := selection.Candidates[0]
	if candidate.TrustMode != tlsTrustModeCustomTLS {
		t.Fatalf("expected trust_mode=%q, got %q", tlsTrustModeCustomTLS, candidate.TrustMode)
	}
	if candidate.BootstrapStrategy != tlsBootstrapStrategyInstall {
		t.Fatalf("expected bootstrap_strategy=%q, got %q", tlsBootstrapStrategyInstall, candidate.BootstrapStrategy)
	}
	if candidate.BootstrapURL != "" {
		t.Fatalf("deduplicated proxy origin exposed internal-CA bootstrap URL %q", candidate.BootstrapURL)
	}
}

func TestResolveHubConnectionSelection_BuiltInTLSMatchingHostForwardedHTTPDoesNotReclassifyExternal(t *testing.T) {
	withHealthyTailscaleHTTPSUnavailable(t)
	withMockHubInterfaces(t, nil, map[int][]net.Addr{})

	ca, err := certmgr.GenerateCA()
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}
	sut := newTestAPIServer(t)
	sut.externalURL = "https://proxy.example"
	sut.tlsState.Enabled = true
	sut.tlsState.Source = tlsSourceBuiltIn
	sut.tlsState.CACertPEM = certmgr.CertPEM(ca.Cert)

	req := httptest.NewRequest(http.MethodGet, "https://labtether:8443/settings/enrollment", nil)
	req.Host = "labtether:8443"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Forwarded-Host", "proxy.example")
	req.Header.Set("X-Forwarded-Proto", "http")

	selection := sut.resolveHubConnectionSelection(req)
	if len(selection.Candidates) != 2 {
		t.Fatalf("expected distinct HTTPS external and HTTP request candidates, got %+v", selection.Candidates)
	}
	external := selection.Candidates[0]
	if external.Kind != "external" || external.TrustMode != tlsTrustModeLabtetherCA ||
		external.BootstrapStrategy != tlsBootstrapStrategyPinnedCA || external.BootstrapURL == "" {
		t.Fatalf("scheme-mismatched forwarding changed external trust metadata: %+v", external)
	}
	request := selection.Candidates[1]
	if request.Kind != "request" || request.HubURL != "http://proxy.example" ||
		request.TrustMode != tlsTrustModePlainHTTP || request.BootstrapStrategy != tlsBootstrapStrategyInstall {
		t.Fatalf("expected separate plain-HTTP request candidate, got %+v", request)
	}
}
