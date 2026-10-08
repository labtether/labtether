package securityruntime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func allowTestLoopbackHTTP(t *testing.T) {
	t.Helper()
	t.Setenv(envAllowInsecureTransport, "true")
	t.Setenv(envOutboundAllowLoopback, "true")
}

func testLoopbackAddressPolicy() OutboundAddressPolicy {
	return OutboundAddressPolicy{AllowLoopback: true}
}

func TestOriginRestrictedOutboundHTTPClientAllowsSameOriginRedirect(t *testing.T) {
	allowTestLoopbackHTTP(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/finish", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewOriginRestrictedOutboundHTTPClient(nil, testLoopbackAddressPolicy(), server.URL)
	if err != nil {
		t.Fatalf("create restricted client: %v", err)
	}
	resp, err := client.Get(server.URL + "/start")
	if err != nil {
		t.Fatalf("follow same-origin redirect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestOriginRestrictedOutboundHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	allowTestLoopbackHTTP(t)
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/secret", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := NewOriginRestrictedOutboundHTTPClient(nil, testLoopbackAddressPolicy(), source.URL, target.URL)
	if err != nil {
		t.Fatalf("create restricted client: %v", err)
	}
	if _, err := client.Get(source.URL); err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("expected cross-origin redirect rejection, got %v", err)
	}
	if got := targetHits.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want 0", got)
	}
}

func TestOriginRestrictedOutboundHTTPClientRejectsDirectUnapprovedOrigin(t *testing.T) {
	allowTestLoopbackHTTP(t)
	var targetHits atomic.Int32
	allowed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer allowed.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	client, err := NewOriginRestrictedOutboundHTTPClient(nil, testLoopbackAddressPolicy(), allowed.URL)
	if err != nil {
		t.Fatalf("create restricted client: %v", err)
	}
	if _, err := client.Get(target.URL); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatalf("expected unapproved-origin rejection, got %v", err)
	}
	if got := targetHits.Load(); got != 0 {
		t.Fatalf("unapproved target received %d requests, want 0", got)
	}
}

func TestOriginRestrictedOutboundHTTPClientRejectsUnsafeAddresses(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "false")
	t.Setenv(envOutboundAllowLoopback, "false")
	t.Setenv(envOutboundAllowLinkLocal, "false")
	for _, rawURL := range []string{
		"https://0.1.2.3/identity",
		"https://127.0.0.1/identity",
		"https://10.0.0.8/identity",
		"https://100.64.0.1/identity",
		"https://169.254.169.254/latest/meta-data",
		"https://192.0.2.1/identity",
		"https://198.18.0.1/identity",
		"https://[::1]/identity",
		"https://[fe80::1]/identity",
		"https://[2001:db8::1]/identity",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := NewOriginRestrictedOutboundHTTPClient(nil, OutboundAddressPolicy{}, rawURL); err == nil {
				t.Fatalf("expected %s to be rejected", rawURL)
			}
		})
	}
}

func TestOriginRestrictedOutboundHTTPClientPrivateOptInStaysNarrow(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "true")
	policy := OutboundAddressPolicy{AllowPrivate: true}
	for _, rawURL := range []string{
		"https://0.1.2.3/identity",
		"https://192.0.2.1/identity",
		"https://198.18.0.1/identity",
		"https://240.0.0.1/identity",
		"https://[2001:db8::1]/identity",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := NewOriginRestrictedOutboundHTTPClient(nil, policy, rawURL); err == nil {
				t.Fatalf("private opt-in must not allow special-use target %s", rawURL)
			}
		})
	}
	for _, rawURL := range []string{
		"https://10.0.0.8/identity",
		"https://100.64.0.1/identity",
		"https://[fd7a:115c:a1e0::1]/identity",
	} {
		t.Run("allowed-"+rawURL, func(t *testing.T) {
			if _, err := NewOriginRestrictedOutboundHTTPClient(nil, policy, rawURL); err != nil {
				t.Fatalf("private opt-in should allow private target %s: %v", rawURL, err)
			}
		})
	}
}

func TestOriginRestrictedOutboundHTTPClientRejectsMixedDNSAnswers(t *testing.T) {
	t.Setenv(envOutboundAllowPrivate, "false")
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "identity.example.com" {
			return nil, errors.New("unexpected host")
		}
		return []net.IPAddr{
			{IP: net.ParseIP("8.8.8.8")},
			{IP: net.ParseIP("10.0.0.8")},
		}, nil
	})

	if _, err := NewOriginRestrictedOutboundHTTPClient(nil, OutboundAddressPolicy{}, "https://identity.example.com"); err == nil {
		t.Fatal("expected mixed public/private DNS answers to be rejected")
	} else if !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}
}

func TestOriginRestrictedOutboundHTTPClientDisablesEnvironmentProxy(t *testing.T) {
	allowTestLoopbackHTTP(t)
	client, err := NewOriginRestrictedOutboundHTTPClient(nil, testLoopbackAddressPolicy(), "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("create restricted client: %v", err)
	}
	originTransport, ok := client.Transport.(originRestrictedRoundTripper)
	if !ok {
		t.Fatalf("transport type = %T, want originRestrictedRoundTripper", client.Transport)
	}
	validatingTransport, ok := originTransport.base.(outboundValidatingRoundTripper)
	if !ok {
		t.Fatalf("nested transport type = %T, want outboundValidatingRoundTripper", originTransport.base)
	}
	transport, ok := validatingTransport.base.(*http.Transport)
	if !ok {
		t.Fatalf("base transport type = %T, want *http.Transport", validatingTransport.base)
	}
	if transport.Proxy != nil {
		t.Fatal("restricted client must ignore environment proxies")
	}
}

func TestOriginRestrictedOutboundHTTPClientRechecksTightenedPolicyAtDial(t *testing.T) {
	allowTestLoopbackHTTP(t)
	var dialLookups atomic.Int32
	var serverHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	rawURL := "http://identity.example.com:" + serverURL.Port()
	withMockLookupIPAddrs(t, func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "identity.example.com" {
			return nil, errors.New("unexpected host")
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	originalLookupDialIPAddrs := lookupDialIPAddrs
	lookupDialIPAddrs = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "identity.example.com" {
			return nil, errors.New("unexpected dial host")
		}
		dialLookups.Add(1)
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	t.Cleanup(func() { lookupDialIPAddrs = originalLookupDialIPAddrs })

	client, err := NewOriginRestrictedOutboundHTTPClient(nil, testLoopbackAddressPolicy(), rawURL)
	if err != nil {
		t.Fatalf("create client while loopback is allowed: %v", err)
	}
	t.Setenv(envOutboundAllowLoopback, "false")
	if _, err := client.Get(rawURL); err == nil {
		t.Fatal("expected loopback address returned at dial time to be rejected")
	} else if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback rejection, got %v", err)
	}
	if got := dialLookups.Load(); got != 1 {
		t.Fatalf("dial-time DNS checks = %d, want 1", got)
	}
	if got := serverHits.Load(); got != 0 {
		t.Fatalf("rebound target received %d requests, want 0", got)
	}
}
