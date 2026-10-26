package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func loginAttempt(handler http.HandlerFunc, remoteAddr, forwardedFor string) int {
	// An invalid body is rejected after the rate limits but before any
	// password hashing, which keeps this test fast.
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec.Code
}

// One client used to drain the global login budget and lock every other user
// out. Its own per-client ceiling now trips first.
func TestOneClientCannotExhaustGlobalLoginLimit(t *testing.T) {
	sut := newTestAPIServer(t)
	handler := sut.buildHTTPHandlers(nil, nil, nil)["/auth/login"]

	limited := 0
	for i := 0; i < 2*loginGlobalLimitForTest; i++ {
		if loginAttempt(handler, "198.51.100.7:40000", "") == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("a single client was never rate limited")
	}
	if code := loginAttempt(handler, "198.51.100.8:40000", ""); code == http.StatusTooManyRequests {
		t.Fatal("another client was locked out by one noisy client")
	}
}

const loginGlobalLimitForTest = 120

func TestTrustedRequestClientIPHonoursOnlyTrustedProxies(t *testing.T) {
	t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", "10.9.0.0/16")
	t.Setenv("LABTETHER_TRUST_PROXY_HOSTS", "")
	cases := []struct{ remote, forwarded, want string }{
		{"198.51.100.7:1", "203.0.113.5", "198.51.100.7"},          // untrusted peer cannot spoof
		{"10.9.0.4:1", "203.0.113.5", "203.0.113.5"},               // trusted proxy
		{"10.9.0.4:1", "203.0.113.66, 203.0.113.5", "203.0.113.5"}, // only the appended hop counts
		{"127.0.0.1:1", "203.0.113.5", "203.0.113.5"},              // loopback proxy is trusted
		{"10.9.0.4:1", "not-an-ip", "10.9.0.4"},                    // malformed header falls back
		{"10.9.0.4:1", "", "10.9.0.4"},                             // no header
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = tc.remote
		if tc.forwarded != "" {
			req.Header.Set("X-Forwarded-For", tc.forwarded)
		}
		if got := trustedRequestClientIP(req); got != tc.want {
			t.Errorf("remote=%s xff=%q: client ip = %q, want %q", tc.remote, tc.forwarded, got, tc.want)
		}
	}
}
