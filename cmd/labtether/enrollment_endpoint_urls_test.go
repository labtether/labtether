package main

import (
	"bytes"
	"encoding/json"
	"github.com/labtether/labtether/internal/enrollment"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnroll_IncludesCACertPEM(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.CACertPEM = []byte("-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----\n")

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "ca-cert-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.CACertPEM != string(sut.tlsState.CACertPEM) {
		t.Fatalf("expected ca_cert_pem=%q, got %q", string(sut.tlsState.CACertPEM), resp.CACertPEM)
	}
}

func TestEnroll_OmitsCACertPEMWhenNil(t *testing.T) {
	sut := newTestAPIServer(t)
	// caCertPEM is nil by default

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "no-ca-cert-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify ca_cert_pem is omitted from JSON (not present as a key)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, exists := raw["ca_cert_pem"]; exists {
		t.Fatalf("expected ca_cert_pem to be omitted when caCertPEM is nil, but it was present: %v", raw["ca_cert_pem"])
	}

	// Also verify the typed response has empty string
	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.CACertPEM != "" {
		t.Fatalf("expected empty ca_cert_pem, got %q", resp.CACertPEM)
	}
}

func TestEnrollFullFlow_URLSchemes(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)

	// Test with TLS disabled — URLs should use http/ws
	rawToken, _ := mustCreateEnrollmentToken(t, sut)

	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "scheme-test-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.HubWSURL != "ws://localhost:8080/ws/agent" {
		t.Fatalf("expected ws:// URL, got %q", resp.HubWSURL)
	}
	if resp.HubAPIURL != "http://localhost:8080" {
		t.Fatalf("expected http:// URL, got %q", resp.HubAPIURL)
	}

	// Test with TLS enabled — URLs should use https/wss
	sut.tlsState.Enabled = true
	rawToken2, _ := mustCreateEnrollmentToken(t, sut)

	enrollPayload2, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken2,
		Hostname:        "scheme-test-node-tls",
		Platform:        "linux",
	})

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload2))
	req2.Host = "localhost:8080"
	req2.RemoteAddr = "127.0.0.1:12345"
	rec2 := httptest.NewRecorder()
	sut.handleEnroll(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp2 enrollment.EnrollResponse
	json.Unmarshal(rec2.Body.Bytes(), &resp2)
	if resp2.HubWSURL != "wss://localhost:8080/ws/agent" {
		t.Fatalf("expected wss:// URL, got %q", resp2.HubWSURL)
	}
	if resp2.HubAPIURL != "https://localhost:8080" {
		t.Fatalf("expected https:// URL, got %q", resp2.HubAPIURL)
	}
}

func TestEnrollFullFlow_UsesExternalURLForHubEndpoints(t *testing.T) {
	disableTailscaleResolutionForTest(t)

	sut := newTestAPIServer(t)
	sut.externalURL = "https://hub.example.com:9443"

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "external-url-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "attacker.local:9999"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.HubAPIURL != "https://hub.example.com:9443" {
		t.Fatalf("expected external hub_api_url, got %q", resp.HubAPIURL)
	}
	if resp.HubWSURL != "wss://hub.example.com:9443/ws/agent" {
		t.Fatalf("expected external hub_ws_url, got %q", resp.HubWSURL)
	}
}

func TestEnrollFullFlow_PrefersTailscaleHubEndpointsWhenAvailable(t *testing.T) {
	t.Setenv(envTailscaleManaged, "true")

	sut := newTestAPIServer(t)

	originalLookPath := tailscaleLookPath
	originalRunner := tailscaleRunner
	originalFallbackPaths := tailscaleFallbackPaths
	t.Cleanup(func() {
		tailscaleLookPath = originalLookPath
		tailscaleRunner = originalRunner
		tailscaleFallbackPaths = originalFallbackPaths
	})

	tailscaleLookPath = func(file string) (string, error) {
		return "/usr/local/bin/tailscale", nil
	}
	tailscaleFallbackPaths = func() []string { return nil }
	tailscaleRunner = func(_ time.Duration, path string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "status --json":
			return []byte(`{
				"BackendState": "Running",
				"CurrentTailnet": { "Name": "homelab.ts.net" },
				"Self": {
					"DNSName": "hub.homelab.ts.net.",
					"TailscaleIPs": ["100.101.102.103"]
				}
			}`), nil
		case "serve status --json":
			return []byte(`{
				"TCP": {
					"443": {
						"HTTPS": true,
						"Web": {
							"/": {
								"Proxy": "http://127.0.0.1:3000"
							}
						}
					}
				}
			}`), nil
		default:
			t.Fatalf("unexpected tailscale invocation: %v", args)
			return nil, nil
		}
	}

	rawToken, _ := mustCreateEnrollmentToken(t, sut)
	enrollPayload, _ := json.Marshal(enrollment.EnrollRequest{
		EnrollmentToken: rawToken,
		Hostname:        "tailscale-enroll-node",
		Platform:        "linux",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(enrollPayload))
	req.Host = "localhost:8443"
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	sut.handleEnroll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp enrollment.EnrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.HubAPIURL != "https://hub.homelab.ts.net" {
		t.Fatalf("expected tailscale hub_api_url, got %q", resp.HubAPIURL)
	}
	if resp.HubWSURL != "wss://hub.homelab.ts.net/ws/agent" {
		t.Fatalf("expected tailscale hub_ws_url, got %q", resp.HubWSURL)
	}
}
