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

func TestResolveHubConnectionSelectionPrefersTailscaleForLoopbackHost(t *testing.T) {
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
	if selection.HubURL != "http://100.101.102.103:8080" {
		t.Fatalf("expected tailscale hub URL, got %q", selection.HubURL)
	}
	if selection.WSURL != "ws://100.101.102.103:8080/ws/agent" {
		t.Fatalf("expected tailscale ws URL, got %q", selection.WSURL)
	}
	if len(selection.Candidates) < 2 {
		t.Fatalf("expected multiple hub candidates, got %d", len(selection.Candidates))
	}
	if selection.Candidates[0].Kind != "tailscale" {
		t.Fatalf("expected first candidate to be tailscale, got %q", selection.Candidates[0].Kind)
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
	if payload.HubURL != "http://100.96.0.5:8080" {
		t.Fatalf("expected tailscale hub_url, got %q", payload.HubURL)
	}
	if payload.WSURL != "ws://100.96.0.5:8080/ws/agent" {
		t.Fatalf("expected tailscale ws_url, got %q", payload.WSURL)
	}
	if len(payload.HubCandidates) < 2 {
		t.Fatalf("expected at least 2 hub candidates, got %d", len(payload.HubCandidates))
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
