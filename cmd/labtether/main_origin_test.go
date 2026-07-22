package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

func TestCheckSameOrigin_AllowsEmptyOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"

	if !checkSameOrigin(req) {
		t.Fatal("expected empty origin to be allowed")
	}
}

func TestCheckSameOrigin_AllowsExactNetworkOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local:8443/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8443"
	req.Header.Set("Origin", "https://hub.local:8443")

	if !checkSameOrigin(req) {
		t.Fatal("expected exact network origin to be allowed")
	}
}

func TestCheckSameOrigin_DeniesSameHostDifferentPort(t *testing.T) {
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "https://hub.local:3000")

	if checkSameOrigin(req) {
		t.Fatal("expected another service on the same hostname to be denied")
	}
}

func TestCheckSameOrigin_AllowsExplicitCrossPortOrigin(t *testing.T) {
	t.Setenv("LABTETHER_CORS_ALLOWED_ORIGINS", "https://hub.local:3000")
	req := httptest.NewRequest("GET", "https://hub.local/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "hub.local:8080"
	req.Header.Set("Origin", "https://hub.local:3000")

	if !checkSameOrigin(req) {
		t.Fatal("expected explicitly allowlisted cross-port origin to be allowed")
	}
}

func TestCheckSameOrigin_DeniesCrossPortLoopbackAliasOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://127.0.0.1:8080/terminal/sessions/session-1/stream?ticket=t", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:3000")

	if checkSameOrigin(req) {
		t.Fatal("expected cross-port/cross-scheme loopback origin to be denied")
	}
}

func TestCheckSameOrigin_DeniesDifferentLoopbackAlias(t *testing.T) {
	req := httptest.NewRequest("GET", "http://127.0.0.1:8080/ws/events", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:8080")

	if checkSameOrigin(req) {
		t.Fatal("expected a different loopback hostname to require explicit allowlisting")
	}
}

func TestCheckSameOrigin_DeniesCrossPortIPv6LoopbackAliasOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "https://[::1]:8080/ws/events", nil)
	req.Host = "[::1]:8080"
	req.Header.Set("Origin", "http://127.0.0.1:3000")

	if checkSameOrigin(req) {
		t.Fatal("expected cross-port/cross-scheme IPv4/IPv6 loopback origin to be denied")
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
	req.Header.Set("X-Forwarded-Proto", "https")

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
	req.Header.Set("X-Forwarded-Proto", "https")

	if checkSameOrigin(req) {
		t.Fatal("expected untrusted forwarded host to be denied")
	}
}

func TestCheckSameOrigin_AllowsTrustedForwardedHeaderHost(t *testing.T) {
	t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", "10.0.0.0/8")
	req := httptest.NewRequest("GET", "https://labtether:8080/ws/events", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "10.0.0.15:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("Forwarded", "for=10.0.0.10;proto=https;host=hub.example.ts.net")

	if !checkSameOrigin(req) {
		t.Fatal("expected trusted Forwarded host to be allowed")
	}
}

func TestCheckSameOrigin_DeniesForwardedHostFromUnconfiguredPrivateSource(t *testing.T) {
	req := httptest.NewRequest("GET", "https://labtether:8080/ws/events", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "10.0.0.15:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("Forwarded", "for=10.0.0.10;proto=https;host=hub.example.ts.net")

	if checkSameOrigin(req) {
		t.Fatal("expected an unconfigured private-network proxy source to be denied")
	}
}

func TestCheckSameOrigin_DeniesForwardedHostOutsideConfiguredProxyCIDR(t *testing.T) {
	t.Setenv("LABTETHER_TRUST_PROXY_CIDRS", "10.1.0.0/16,not-a-cidr")
	req := httptest.NewRequest("GET", "https://labtether:8080/ws/events", nil)
	req.Host = "labtether:8080"
	req.RemoteAddr = "10.2.0.15:40000"
	req.Header.Set("Origin", "https://hub.example.ts.net")
	req.Header.Set("X-Forwarded-Host", "hub.example.ts.net")
	req.Header.Set("X-Forwarded-Proto", "https")

	if checkSameOrigin(req) {
		t.Fatal("expected a source outside the configured proxy CIDR to be denied")
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

func TestWithAuthNativeStreamOriginRequiresValidOneTimeTicket(t *testing.T) {
	sut := newTestAPIServer(t)
	user, _, err := sut.authStore.BootstrapFirstUser("stream-owner", "unused-test-hash")
	if err != nil {
		t.Fatalf("bootstrap user: %v", err)
	}
	rawSessionToken := "valid-cookie-session-token"
	if _, err := sut.authStore.CreateAuthSession(user.ID, auth.HashToken(rawSessionToken), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create auth session: %v", err)
	}

	for _, tc := range []struct {
		name   string
		origin string
		query  string
	}{
		{name: "null without ticket", origin: "null"},
		{name: "file without ticket", origin: "file://"},
		{name: "custom scheme without ticket", origin: "labtether-local://bundle"},
		{name: "null invalid ticket", origin: "null", query: "?ticket=invalid"},
		{name: "network origin invalid ticket", origin: "https://hub.local", query: "?ticket=invalid"},
		{name: "network origin empty ticket", origin: "https://hub.local", query: "?ticket="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := sut.withAuth(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "https://hub.local/terminal/sessions/session-1/stream"+tc.query, nil)
			req.Host = "hub.local"
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: rawSessionToken})
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
			}
			if called {
				t.Fatal("invalid or absent ticket fell through to the session cookie")
			}
		})
	}
}

func TestWithAuthNativeStreamOriginAcceptsValidOneTimeTicket(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := contextWithPrincipal(context.Background(), "native-stream-user", auth.RoleOperator)
	ticket, _, err := sut.issueStreamTicket(ctx, "session-1")
	if err != nil {
		t.Fatalf("issue stream ticket: %v", err)
	}

	called := false
	handler := sut.withAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := userIDFromContext(r.Context()); got != "native-stream-user" {
			t.Fatalf("principal = %q, want native-stream-user", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "https://hub.local/terminal/sessions/session-1/stream?ticket="+ticket, nil)
	req.Host = "hub.local"
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("valid one-time ticket did not authenticate: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCORSMiddlewareBlocksCrossPortCookieMutation(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	sut := &apiServer{}

	req := httptest.NewRequest(http.MethodPost, "https://hub.local:8443/assets/manual", strings.NewReader(`{}`))
	req.Host = "hub.local:8443"
	req.Header.Set("Origin", "https://hub.local:9443")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	sut.corsMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("protected mutation handler should not run")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS origin %q", got)
	}
}

func TestCORSMiddlewareBlocksCrossOriginUnauthenticatedMutation(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})
	sut := &apiServer{}

	// Bootstrap and login are intentionally unauthenticated, but a browser on
	// an attacker-controlled origin must not be able to mutate them through the
	// directly reachable hub listener.
	req := httptest.NewRequest(http.MethodPost, "https://hub.local:8443/auth/bootstrap", strings.NewReader(`{"username":"owner"}`))
	req.Host = "hub.local:8443"
	req.Header.Set("Origin", "https://attacker.example")
	rec := httptest.NewRecorder()

	sut.corsMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("cross-origin unauthenticated mutation status=%d called=%t", rec.Code, called)
	}
}

func TestCORSMiddlewareAllowsExactOriginCookieMutation(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	sut := &apiServer{}

	req := httptest.NewRequest(http.MethodPost, "https://hub.local:8443/assets/manual", strings.NewReader(`{}`))
	req.Host = "hub.local:8443"
	req.Header.Set("Origin", "https://hub.local:8443")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	sut.corsMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("exact-origin mutation status=%d called=%t", rec.Code, called)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://hub.local:8443" {
		t.Fatalf("CORS origin = %q", got)
	}
}

func TestCORSMiddlewareRejectsMissingOriginCrossSiteBrowserMutation(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	sut := &apiServer{}

	req := httptest.NewRequest(http.MethodDelete, "https://hub.local:8443/auth/account", nil)
	req.Host = "hub.local:8443"
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	sut.corsMiddleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || called {
		t.Fatalf("missing-origin browser mutation status=%d called=%t", rec.Code, called)
	}
}
