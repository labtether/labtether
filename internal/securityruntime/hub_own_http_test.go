package securityruntime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func hubOwnGet(rawURL string) error {
	resp, err := GetHubOwnHTTP(context.Background(), rawURL, 2*time.Second)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// The hub's own plain http check must stay on this host or a private network.
// A public or cloud metadata address is refused, also when a name resolves to
// one at connect time, so the path cannot be used to reach anything else.
func TestHubOwnHTTPStaysOnThisHostOrAPrivateNetwork(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "")
	for _, raw := range []string{
		"http://8.8.8.8:8080/healthz",
		"http://169.254.169.254/healthz",
		"http://169.254.10.1:8080/healthz",
		"https://127.0.0.1:8443/healthz",
		"http://user:pw@127.0.0.1:8080/healthz",
	} {
		if err := hubOwnGet(raw); err == nil {
			t.Errorf("GetHubOwnHTTP(%q) was not refused", raw)
		}
	}

	original := lookupIPAddrs
	t.Cleanup(func() { lookupIPAddrs = original })
	for name, resolved := range map[string][]string{
		"a public address":   {"172.18.0.5", "93.184.216.34"},
		"the cloud metadata": {"169.254.169.254"},
	} {
		lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
			addrs := make([]net.IPAddr, 0, len(resolved))
			for _, ip := range resolved {
				addrs = append(addrs, net.IPAddr{IP: net.ParseIP(ip)})
			}
			return addrs, nil
		}
		if err := hubOwnGet("http://labtether:8080/healthz"); err == nil || !errors.Is(err, ErrNotHubOwnAddress) {
			t.Errorf("a name that resolves to %s must be refused, got %v", name, err)
		}
	}

	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	if err := hubOwnGet("http://172.18.0.5:8080/healthz"); err == nil || !strings.Contains(err.Error(), "private host") {
		t.Errorf("ALLOW_PRIVATE=false must refuse a private address, got %v", err)
	}
}

// A redirect must not carry the check to another service on the same host.
func TestHubOwnHTTPDoesNotFollowARedirectToAnotherOrigin(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "")
	var otherRequests atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		otherRequests.Add(1)
	}))
	t.Cleanup(other.Close)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/admin", http.StatusFound)
	}))
	t.Cleanup(hub.Close)

	if err := hubOwnGet(hub.URL + "/healthz"); err == nil || !strings.Contains(err.Error(), "redirect away") {
		t.Fatalf("a redirect to another origin must be refused, got %v", err)
	}
	if otherRequests.Load() != 0 {
		t.Fatalf("the other origin was reached %d times", otherRequests.Load())
	}
}
