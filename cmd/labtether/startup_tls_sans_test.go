package main

import (
	"reflect"
	"testing"
)

func TestBuiltInCertificateSANHosts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "IPv4 with port", raw: "https://192.0.2.25:8443", want: []string{"192.0.2.25"}},
		{name: "DNS with path", raw: "https://hub.example.test/base/", want: []string{"hub.example.test"}},
		{name: "IPv6 with port", raw: "https://[2001:db8::25]:8443", want: []string{"2001:db8::25"}},
		{name: "reject HTTP", raw: "http://192.0.2.25:8080", want: nil},
		{name: "reject malformed", raw: "not-a-url", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := builtInCertificateSANHosts(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("builtInCertificateSANHosts(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestShouldUseTailscaleCertificate(t *testing.T) {
	if !shouldUseTailscaleCertificate("hub.tailnet.ts.net", nil) {
		t.Fatal("expected Tailscale certificate to be preferred when no external URL is configured")
	}
	if !shouldUseTailscaleCertificate("hub.tailnet.ts.net", []string{"hub.tailnet.ts.net"}) {
		t.Fatal("expected matching Tailscale domain to be accepted")
	}
	if shouldUseTailscaleCertificate("hub.tailnet.ts.net", []string{"192.0.2.25"}) {
		t.Fatal("expected configured external IP to prefer its covering built-in certificate")
	}
	if shouldUseTailscaleCertificate("hub.tailnet.ts.net", []string{"hub.example.test"}) {
		t.Fatal("expected a different external hostname to prefer its covering built-in certificate")
	}
}
