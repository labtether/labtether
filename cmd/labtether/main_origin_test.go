package main

import (
	"net/http/httptest"
	"testing"
)

func TestCheckSameOrigin_AllowsEmptyOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"

	if !checkSameOrigin(req) {
		t.Fatal("expected empty origin to be allowed")
	}
}

func TestCheckSameOrigin_AllowsSameHostDifferentPort(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "https://hub.local:3000")

	if !checkSameOrigin(req) {
		t.Fatal("expected same-host origin to be allowed")
	}
}

func TestCheckSameOrigin_AllowsLoopbackAliasOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://127.0.0.1:8080/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:3000")

	if !checkSameOrigin(req) {
		t.Fatal("expected loopback alias origin to be allowed")
	}
}

func TestCheckSameOrigin_AllowsIPv6LoopbackAliasOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://[::1]:8080/ws/events", nil)
	req.Host = "[::1]:8080"
	req.Header.Set("Origin", "http://127.0.0.1:3000")

	if !checkSameOrigin(req) {
		t.Fatal("expected IPv4/IPv6 loopback aliases to be allowed")
	}
}

func TestCheckSameOrigin_DeniesMismatchedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "https://evil.example")

	if checkSameOrigin(req) {
		t.Fatal("expected mismatched origin host to be denied")
	}
}

func TestCheckSameOrigin_AllowsTrustedForwardedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "https://labtether:8080/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("X-Forwarded-Host", "hub.example.ts.net")

	if !checkSameOrigin(req) {
		t.Fatal("expected trusted forwarded host to be allowed")
	}
}

func TestCheckSameOrigin_DeniesUntrustedForwardedHost(t *testing.T) {
	req := httptest.NewRequest("GET", "https://labtether:8080/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "198.51.100.25:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("X-Forwarded-Host", "hub.example.ts.net")

	if checkSameOrigin(req) {
		t.Fatal("expected untrusted forwarded host to be denied")
	}
}

func TestCheckSameOrigin_AllowsTrustedForwardedHeaderHost(t *testing.T) {
	req := httptest.NewRequest("GET", "https://labtether:8080/ws/events", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "10.0.0.15:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("Forwarded", "for=10.0.0.10;proto=https;host=hub.example.ts.net")

	if !checkSameOrigin(req) {
		t.Fatal("expected trusted Forwarded host to be allowed")
	}
}

func TestCheckSameOrigin_AllowsNullOriginForNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "null")

	if !checkSameOrigin(req) {
		t.Fatal("expected null origin to be allowed for native stream path")
	}
}

func TestCheckSameOrigin_DeniesNullOriginOutsideNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/ws/events", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "null")

	if checkSameOrigin(req) {
		t.Fatal("expected null origin to be denied outside native stream path")
	}
}

func TestCheckSameOrigin_AllowsFileOriginForNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/desktop/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "file://")

	if !checkSameOrigin(req) {
		t.Fatal("expected file origin to be allowed for native stream path")
	}
}

func TestCheckSameOrigin_AllowsLabTetherLocalOriginForNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/desktop/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "labtether-local://bundle")

	if !checkSameOrigin(req) {
		t.Fatal("expected labtether-local origin to be allowed for native stream path")
	}
}

func TestCheckSameOrigin_DeniesLabTetherLocalOriginOutsideNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/ws/events", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "labtether-local://bundle")

	if checkSameOrigin(req) {
		t.Fatal("expected labtether-local origin to be denied outside native stream path")
	}
}

func TestCheckSameOrigin_DeniesUnknownCustomOriginForNativeStreamPath(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/desktop/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "chrome-extension://abc123")

	if checkSameOrigin(req) {
		t.Fatal("expected unknown custom origin scheme to be denied")
	}
}
