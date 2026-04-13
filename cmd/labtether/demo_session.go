package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

// demoSessionRateLimiter implements a sliding-window per-IP rate limiter
// specifically for the demo auto-auth endpoint. It is separate from the
// server-wide rateLimiter to keep the demo session budget isolated.
type demoSessionRateLimiter struct {
	mu       sync.Mutex
	windows  map[string]demoRateWindow
	limit    int
	window   time.Duration
	prunedAt time.Time
}

type demoRateWindow struct {
	count   int
	resetAt time.Time
}

func newDemoSessionRateLimiter(limit int, window time.Duration) *demoSessionRateLimiter {
	return &demoSessionRateLimiter{
		windows: make(map[string]demoRateWindow, 64),
		limit:   limit,
		window:  window,
	}
}

// allow returns true if the request from the given IP is within the rate limit.
func (rl *demoSessionRateLimiter) allow(ip string) bool {
	now := time.Now().UTC()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Prune expired entries at most once per minute.
	if len(rl.windows) > 50 && now.After(rl.prunedAt.Add(time.Minute)) {
		for k, v := range rl.windows {
			if now.After(v.resetAt) {
				delete(rl.windows, k)
			}
		}
		rl.prunedAt = now
	}

	w := rl.windows[ip]
	if w.resetAt.IsZero() || now.After(w.resetAt) {
		w = demoRateWindow{
			count:   0,
			resetAt: now.Add(rl.window),
		}
	}
	if w.count >= rl.limit {
		rl.windows[ip] = w
		return false
	}
	w.count++
	rl.windows[ip] = w
	return true
}

// handleDemoSession creates an authenticated session for the demo user and
// redirects to the console. It is an UNAUTHENTICATED endpoint — its entire
// purpose is to give anonymous visitors a read-only session.
//
// POST /api/demo/session?redirect=/some/path
func (s *apiServer) handleDemoSession(w http.ResponseWriter, r *http.Request) {
	// Only available in demo mode.
	if !s.demoMode {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Rate limit by client IP.
	clientIP := requestClientKey(r)
	if s.demoRateLimiter == nil || !s.demoRateLimiter.allow(clientIP) {
		servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// Look up the demo user.
	user, found, err := s.authStore.GetUserByUsername(demoUsername)
	if err != nil {
		log.Printf("demo session: failed to look up demo user: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "demo session unavailable")
		return
	}
	if !found {
		log.Printf("demo session: demo user %q does not exist", demoUsername)
		servicehttp.WriteError(w, http.StatusInternalServerError, "demo session unavailable")
		return
	}

	// Create a real session using the standard auth pattern.
	raw, hashed, err := auth.GenerateSessionToken()
	if err != nil {
		log.Printf("demo session: failed to generate session token: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expiresAt := time.Now().UTC().Add(auth.SessionDuration)
	_, err = s.authStore.CreateAuthSession(user.ID, hashed, expiresAt)
	if err != nil {
		log.Printf("demo session: failed to create auth session: %v", err)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	auth.SetSessionCookie(w, raw, auth.SessionDuration, s.tlsState.Enabled)

	// Redirect to the requested path, defaulting to "/".
	next := sanitizeNextPath(r.URL.Query().Get("redirect"))
	redirectTarget := "/"
	if parsed, err := url.ParseRequestURI(next); err == nil && parsed != nil && strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(parsed.Path, "//") {
		redirectTarget = parsed.Path
		if redirectTarget == "" {
			redirectTarget = "/"
		}
		if parsed.RawQuery != "" {
			redirectTarget += "?" + parsed.RawQuery
		}
	}
	http.Redirect(w, r, redirectTarget, http.StatusSeeOther)
}
