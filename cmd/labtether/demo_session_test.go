package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

func TestHandleDemoSession_SetsCookie(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true
	s.demoRateLimiter = newDemoSessionRateLimiter(10, time.Minute)

	// Bootstrap demo user first.
	if err := s.bootstrapDemoUser(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/demo/session", nil)
	req.RemoteAddr = "192.168.1.10:54321"
	rec := httptest.NewRecorder()
	s.handleDemoSession(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected session cookie to be set")
	}
}

func TestHandleDemoSession_DisabledWhenNotDemoMode(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = false

	req := httptest.NewRequest("GET", "/api/demo/session", nil)
	rec := httptest.NewRecorder()
	s.handleDemoSession(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when not in demo mode, got %d", rec.Code)
	}
}

func TestHandleDemoSession_RateLimitExceeded(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true
	s.demoRateLimiter = newDemoSessionRateLimiter(2, time.Minute)

	if err := s.bootstrapDemoUser(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	// Exhaust rate limit.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/demo/session", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleDemoSession(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("request %d: expected 303, got %d", i+1, rec.Code)
		}
	}

	// Third request should be rate limited.
	req := httptest.NewRequest("GET", "/api/demo/session", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	s.handleDemoSession(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit, got %d", rec.Code)
	}
}

func TestHandleDemoSession_RedirectsToRequestedPath(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true
	s.demoRateLimiter = newDemoSessionRateLimiter(10, time.Minute)

	if err := s.bootstrapDemoUser(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/demo/session?redirect=/en/dashboard", nil)
	req.RemoteAddr = "192.168.1.10:54321"
	rec := httptest.NewRecorder()
	s.handleDemoSession(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if location != "/en/dashboard" {
		t.Fatalf("expected redirect to /en/dashboard, got %q", location)
	}
}

func TestDemoSessionRateLimiter(t *testing.T) {
	rl := newDemoSessionRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.allow("10.0.0.1") {
		t.Fatal("4th request should be denied")
	}

	// Different IP should still be allowed.
	if !rl.allow("10.0.0.2") {
		t.Fatal("different IP should be allowed")
	}
}
