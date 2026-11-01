package main

import (
	"net"
	"net/http/httptest"
	"testing"
)

// The Hub later uses its root SSH key against this address, so it must be the
// agent itself: never a reverse proxy or the Docker bridge gateway, and never
// an address an untrusted peer claims in a header.
func TestAgentSSHSourceAddressSkipsProxiesAndGateways(t *testing.T) {
	withMockHubInterfaces(t, []net.Interface{{Index: 1, Name: "eth0"}}, map[int][]net.Addr{
		1: {&net.IPNet{IP: net.ParseIP("172.18.0.5"), Mask: net.CIDRMask(16, 32)}},
	})
	t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", "198.51.100.0/24")

	for _, tc := range []struct {
		name, remoteAddr, forwardedFor, want string
	}{
		{name: "direct agent", remoteAddr: "192.0.2.20:51000", want: "192.0.2.20"},
		{name: "untrusted peer header ignored", remoteAddr: "192.0.2.20:51000", forwardedFor: "192.0.2.99", want: "192.0.2.20"},
		{name: "docker bridge gateway", remoteAddr: "172.18.0.1:51000", want: ""},
		{name: "loopback proxy forwards agent", remoteAddr: "127.0.0.1:51000", forwardedFor: "192.0.2.30", want: "192.0.2.30"},
		{name: "trusted proxy without client address", remoteAddr: "198.51.100.7:51000", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws/agent", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tc.forwardedFor)
			}
			if got := agentSSHSourceAddress(req); got != tc.want {
				t.Fatalf("agentSSHSourceAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}
