package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// With the Hub's own TLS off behind an HTTPS reverse proxy, an API key must
// work when a proxy the owner listed says the client used HTTPS, and must
// still be refused when any other client (or a proxy saying "http") sends the
// same headers. The fake key is unknown, so getting past the HTTPS rule shows
// up as 401 "invalid api key" and being stopped by it as 403.
func TestAPIKeyHTTPSRuleTrustsOnlyListedProxies(t *testing.T) {
	for _, tc := range []struct {
		name, cidrs, hosts, remote, proto, forwarded string
		want                                         int
	}{
		{name: "listed proxy CIDR says https", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "https", want: http.StatusUnauthorized},
		{name: "listed proxy host says https", hosts: "192.0.2.10", remote: "192.0.2.10:41000", proto: "https", want: http.StatusUnauthorized},
		{name: "listed proxy, upper case", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "HTTPS", want: http.StatusUnauthorized},
		{name: "listed proxy, first of a list", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "https, http", want: http.StatusUnauthorized},
		{name: "loopback proxy says https", remote: "127.0.0.1:41000", proto: "https", want: http.StatusUnauthorized},
		{name: "unlisted client says https", cidrs: "192.0.2.0/24", remote: "203.0.113.7:41000", proto: "https", want: http.StatusForbidden},
		{name: "listed proxy says http", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "http", want: http.StatusForbidden},
		{name: "listed proxy, not exactly https", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "httpsx", want: http.StatusForbidden},
		{name: "listed proxy, first value http", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", proto: "http, https", want: http.StatusForbidden},
		{name: "proxy says http, client adds Forwarded https", remote: "127.0.0.1:41000", proto: "http", forwarded: "proto=https", want: http.StatusForbidden},
		{name: "listed proxy, no header", cidrs: "192.0.2.0/24", remote: "192.0.2.10:41000", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", tc.cidrs)
			t.Setenv("LABTETHER_TRUST_PROXY_HOSTS", tc.hosts)
			sut := newTestAPIServer(t)
			sut.tlsState.Enabled = false
			handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "http://hub.example:8080/api/v2/whoami", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("Authorization", "Bearer lt_fake_notarealkey123456")
			if tc.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.proto)
			}
			if tc.forwarded != "" {
				req.Header.Set("Forwarded", tc.forwarded)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
