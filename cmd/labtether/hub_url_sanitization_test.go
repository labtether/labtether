package main

import (
	"testing"
)

// Tests cover the local aliases and the sanitizedExternalHubURL server method
// defined in hub_url_sanitization.go.
//
// httpURLToWS, sanitizeExternalBaseURL, and sanitizeHostPort are thin aliases
// to internal/hubapi/shared. sanitizedExternalHubURL contains branching TLS
// logic that lives solely in cmd/labtether and warrants direct coverage.

// ---------------------------------------------------------------------------
// httpURLToWS
// ---------------------------------------------------------------------------

func TestHubURLHTTPURLToWS(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"http://example.com", "ws://example.com"},
		{"https://example.com", "wss://example.com"},
		{"https://example.com:8443/path", "wss://example.com:8443/path"},
		{"http://10.0.0.1:8080", "ws://10.0.0.1:8080"},
		// Input with no scheme prefix falls back to ws:// prefix.
		{"example.com", "ws://example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := httpURLToWS(tc.input); got != tc.want {
				t.Fatalf("httpURLToWS(%q): expected %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeHostPort
// ---------------------------------------------------------------------------

func TestHubURLSanitizeHostPort(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantOK  bool
		wantOut string
	}{
		{"valid hostname", "example.com", true, "example.com"},
		{"valid hostname with port", "example.com:8080", true, "example.com:8080"},
		{"valid IPv4", "192.168.1.1", true, "192.168.1.1"},
		{"valid IPv4 with port", "192.168.1.1:9090", true, "192.168.1.1:9090"},
		{"valid IPv6 bracketed", "[::1]:8080", true, "[::1]:8080"},
		{"empty string rejected", "", false, ""},
		{"contains space rejected", "ex ample.com", false, ""},
		{"contains slash rejected", "example.com/path", false, ""},
		{"contains at-sign rejected", "user@example.com", false, ""},
		{"port out of range rejected", "example.com:99999", false, ""},
		{"port zero rejected", "example.com:0", false, ""},
		{"non-numeric port rejected", "example.com:abc", false, ""},
		{"underscore in hostname allowed", "my_host.local", true, "my_host.local"},
		{"hyphen in hostname allowed", "my-host.local", true, "my-host.local"},
		{"unbracketed IPv6 ambiguous colon rejected", "::1", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sanitizeHostPort(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("sanitizeHostPort(%q): expected ok=%v, got ok=%v (result=%q)", tc.input, tc.wantOK, ok, got)
			}
			if ok && got != tc.wantOut {
				t.Fatalf("sanitizeHostPort(%q): expected %q, got %q", tc.input, tc.wantOut, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeExternalBaseURL
// ---------------------------------------------------------------------------

func TestHubURLSanitizeExternalBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantOK  bool
		wantOut string
	}{
		{
			name:    "valid http URL",
			input:   "http://hub.local:8080",
			wantOK:  true,
			wantOut: "http://hub.local:8080",
		},
		{
			name:    "valid https URL",
			input:   "https://hub.example.com",
			wantOK:  true,
			wantOut: "https://hub.example.com",
		},
		{
			name:    "trailing slash stripped",
			input:   "https://hub.example.com/",
			wantOK:  true,
			wantOut: "https://hub.example.com",
		},
		{
			name:    "query string stripped",
			input:   "http://hub.local:8080?foo=bar",
			wantOK:  true,
			wantOut: "http://hub.local:8080",
		},
		{
			name:    "fragment stripped",
			input:   "http://hub.local:8080#section",
			wantOK:  true,
			wantOut: "http://hub.local:8080",
		},
		{
			name:   "userinfo rejected",
			input:  "http://user:pass@hub.local",
			wantOK: false,
		},
		{
			name:   "no scheme rejected",
			input:  "hub.local:8080",
			wantOK: false,
		},
		{
			name:   "ftp scheme rejected",
			input:  "ftp://hub.local",
			wantOK: false,
		},
		{
			name:   "empty string rejected",
			input:  "",
			wantOK: false,
		},
		{
			name:    "whitespace trimmed before parse",
			input:   "  http://hub.local  ",
			wantOK:  true,
			wantOut: "http://hub.local",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sanitizeExternalBaseURL(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("sanitizeExternalBaseURL(%q): expected ok=%v, got ok=%v (result=%q)", tc.input, tc.wantOK, ok, got)
			}
			if ok && got != tc.wantOut {
				t.Fatalf("sanitizeExternalBaseURL(%q): expected %q, got %q", tc.input, tc.wantOut, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizedExternalHubURL (apiServer method)
// ---------------------------------------------------------------------------

func TestHubURLSanitizedExternalHubURL(t *testing.T) {
	// nil receiver must not panic and must return empty/false.
	t.Run("nil server returns false", func(t *testing.T) {
		var s *apiServer
		url, ok := s.sanitizedExternalHubURL()
		if ok || url != "" {
			t.Fatalf("expected ('', false) for nil server, got (%q, %v)", url, ok)
		}
	})

	t.Run("TLS disabled, http URL accepted", func(t *testing.T) {
		s := &apiServer{externalURL: "http://hub.local:8080", tlsState: TLSState{Enabled: false}}
		url, ok := s.sanitizedExternalHubURL()
		if !ok || url != "http://hub.local:8080" {
			t.Fatalf("expected ('http://hub.local:8080', true), got (%q, %v)", url, ok)
		}
	})

	t.Run("TLS disabled, https URL accepted", func(t *testing.T) {
		s := &apiServer{externalURL: "https://hub.example.com", tlsState: TLSState{Enabled: false}}
		url, ok := s.sanitizedExternalHubURL()
		if !ok || url != "https://hub.example.com" {
			t.Fatalf("expected ('https://hub.example.com', true), got (%q, %v)", url, ok)
		}
	})

	t.Run("TLS enabled, https URL accepted", func(t *testing.T) {
		s := &apiServer{externalURL: "https://secure.hub.com:8443", tlsState: TLSState{Enabled: true}}
		url, ok := s.sanitizedExternalHubURL()
		if !ok || url != "https://secure.hub.com:8443" {
			t.Fatalf("expected ('https://secure.hub.com:8443', true), got (%q, %v)", url, ok)
		}
	})

	t.Run("TLS enabled, http URL rejected", func(t *testing.T) {
		s := &apiServer{externalURL: "http://hub.local:8080", tlsState: TLSState{Enabled: true}}
		url, ok := s.sanitizedExternalHubURL()
		if ok || url != "" {
			t.Fatalf("expected ('', false) for http URL in TLS mode, got (%q, %v)", url, ok)
		}
	})

	t.Run("empty externalURL returns false", func(t *testing.T) {
		s := &apiServer{externalURL: "", tlsState: TLSState{Enabled: false}}
		url, ok := s.sanitizedExternalHubURL()
		if ok || url != "" {
			t.Fatalf("expected ('', false) for empty externalURL, got (%q, %v)", url, ok)
		}
	})

	t.Run("invalid URL returns false", func(t *testing.T) {
		s := &apiServer{externalURL: "not-a-url", tlsState: TLSState{Enabled: false}}
		url, ok := s.sanitizedExternalHubURL()
		if ok || url != "" {
			t.Fatalf("expected ('', false) for invalid URL, got (%q, %v)", url, ok)
		}
	})

	t.Run("whitespace-padded valid URL sanitized", func(t *testing.T) {
		s := &apiServer{externalURL: "  https://hub.example.com  ", tlsState: TLSState{Enabled: true}}
		url, ok := s.sanitizedExternalHubURL()
		if !ok || url != "https://hub.example.com" {
			t.Fatalf("expected ('https://hub.example.com', true), got (%q, %v)", url, ok)
		}
	})
}
