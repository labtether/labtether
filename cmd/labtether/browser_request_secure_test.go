package main

import (
	"net/http/httptest"
	"testing"
)

// The session cookie stays Secure unless a trusted proxy (the bundled console
// on loopback here) states the browser used plain HTTP. A missing or untrusted
// protocol must not downgrade it.
func TestBrowserRequestIsSecureDropsOnlyOnTrustedHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, remote, proto string
		want                bool
	}{
		{"console says http", "127.0.0.1:4000", "http", false},
		{"console says https", "127.0.0.1:4000", "https", true},
		{"console sends nothing", "127.0.0.1:4000", "", true},
		{"client claims http", "203.0.113.7:4000", "http", true},
		{"direct plain HTTP", "203.0.113.7:4000", "", true},
	} {
		req := httptest.NewRequest("POST", "/auth/login", nil)
		req.RemoteAddr = tc.remote
		if tc.proto != "" {
			req.Header.Set("X-Forwarded-Proto", tc.proto)
		}
		if got := browserRequestIsSecure(req); got != tc.want {
			t.Errorf("%s: secure = %t, want %t", tc.name, got, tc.want)
		}
	}
}
