package securityruntime

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// Plain http per connection must stay on the local network: RFC 1918, IPv6
// unique-local, tailnet, .local and .ts.net; never link-local, cloud metadata,
// loopback or a public address.
func TestLocalNetworkHostRule(t *testing.T) {
	for host, want := range map[string]bool{
		"192.168.1.20": true, "10.0.0.5": true, "172.20.1.1": true, "100.101.102.103": true,
		"fd7a:115c:a1e0::1": true, "169.254.10.1": false, "fe80::1": false,
		"169.254.169.254": false, "169.254.170.2": false, "fd00:ec2::254": false,
		"homeassistant.local": true, "ha.tail1234.ts.net": true,
		"8.8.8.8": false, "127.0.0.1": false, "::1": false, "172.32.0.1": false, "100.128.0.1": false,
		"example.com": false, ".local": false, "local": false, "ts.net": false,
	} {
		if got := IsLocalNetworkHost(host); got != want {
			t.Errorf("IsLocalNetworkHost(%q) = %v, want %v", host, got, want)
		}
	}
	for _, raw := range []string{"https://192.168.1.20:8123", "http://8.8.8.8:8123", "http://user:pw@192.168.1.20:8123", "http://example.com"} {
		if _, err := ValidateLocalHTTPURL(raw); err == nil {
			t.Errorf("ValidateLocalHTTPURL(%q) accepted", raw)
		}
	}
	if _, err := ValidateLocalHTTPURL("http://homeassistant.local:8123"); err != nil {
		t.Errorf("a .local name must be accepted: %v", err)
	}
}

// A .local name that resolves to a public address at connect time must be
// refused, so DNS cannot send the token somewhere public over plain http.
func TestLocalHTTPClientRefusesANameThatResolvesPublic(t *testing.T) {
	original := lookupIPAddrs
	t.Cleanup(func() { lookupIPAddrs = original })
	lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.168.1.20")}, {IP: net.ParseIP("93.184.216.34")}}, nil
	}
	client, err := NewLocalHTTPClient("http://homeassistant.local:8123", 2*time.Second)
	if err != nil {
		t.Fatalf("NewLocalHTTPClient: %v", err)
	}
	_, err = client.Get("http://homeassistant.local:8123/api/")
	if err == nil || !errors.Is(err, ErrNotLocalNetworkAddress) || !strings.Contains(err.Error(), "outside your network") {
		t.Fatalf("public resolution not refused: %v", err)
	}
	_, err = client.Get("http://other.local:8123/api/")
	if err == nil {
		t.Fatal("the client must not reach another origin")
	}
}

// A .local name made to resolve to the cloud metadata address must be refused
// at connect time with the switch on (link-local is not "my network").
func TestLocalHTTPClientRefusesANameThatResolvesToMetadata(t *testing.T) {
	original := lookupIPAddrs
	t.Cleanup(func() { lookupIPAddrs = original })
	lookupIPAddrs = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	}
	client, err := NewLocalHTTPClient("http://homeassistant.local:8123", 2*time.Second)
	if err != nil {
		t.Fatalf("NewLocalHTTPClient: %v", err)
	}
	if _, err := client.Get("http://homeassistant.local:8123/api/"); err == nil || !errors.Is(err, ErrNotLocalNetworkAddress) {
		t.Fatalf("metadata address not refused: %v", err)
	}
	if _, err := ValidateLocalHTTPURL("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Fatal("the metadata address must never be accepted")
	}
}

// LABTETHER_OUTBOUND_ALLOW_PRIVATE=false is the operator's choice: the switch
// may not reopen private addresses, when saving or on a later connection.
func TestLocalHTTPKeepsTheOperatorsPrivateSetting(t *testing.T) {
	client, err := NewLocalHTTPClient("http://192.168.1.20:8123", 2*time.Second)
	if err != nil {
		t.Fatalf("NewLocalHTTPClient with private allowed by default: %v", err)
	}
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	if _, err := ValidateLocalHTTPURL("http://192.168.1.20:8123"); err == nil {
		t.Fatal("ALLOW_PRIVATE=false must refuse 192.168.x with the switch on")
	}
	if _, err := client.Get("http://192.168.1.20:8123/api/"); err == nil || !strings.Contains(err.Error(), "private host") {
		t.Fatalf("a connection after ALLOW_PRIVATE=false must be refused: %v", err)
	}
}
