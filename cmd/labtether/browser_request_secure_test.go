package main

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

// The session cookie's Secure flag follows the protocol the browser used. Only
// a trusted proxy (the bundled console on loopback here) may say it was HTTP;
// a client cannot talk the Hub out of Secure over its own TLS.
func TestBrowserRequestIsSecureFollowsTrustedProxyOnly(t *testing.T) {
	for _, tc := range []struct {
		name, remote, proto string
		tls                 bool
		want                bool
	}{
		{"console says http", "127.0.0.1:4000", "http", true, false},
		{"console says https", "127.0.0.1:4000", "https", false, true},
		{"client spoofs http over TLS", "203.0.113.7:4000", "http", true, true},
		{"direct plain HTTP", "203.0.113.7:4000", "", false, false},
	} {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = tc.remote
		if tc.proto != "" {
			req.Header.Set("X-Forwarded-Proto", tc.proto)
		}
		if tc.tls {
			req.TLS = &tls.ConnectionState{}
		}
		if got := browserRequestIsSecure(req); got != tc.want {
			t.Errorf("%s: secure = %t, want %t", tc.name, got, tc.want)
		}
	}
}
