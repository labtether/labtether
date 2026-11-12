package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

// demoReadOnlyMiddleware blocks all mutating HTTP requests in demo mode.
// The POSTs it lets through change nothing shared: signing in or out of the
// visitor's own session, and the short ticket the iPhone app needs before it
// opens the live event stream (a read capability).
func demoReadOnlyMiddleware(next http.Handler) http.Handler {
	allowedPOSTPaths := map[string]bool{
		"/api/demo/session":   true,
		"/api/auth/login":     true,
		"/api/auth/login/2fa": true,
		"/api/auth/logout":    true,
		"/auth/login":         true,
		"/auth/login/2fa":     true,
		"/auth/logout":        true,
		"/ws/events/ticket":   true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.ToUpper(r.Method)

		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		if method == "POST" && allowedPOSTPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		servicehttp.WriteJSON(w, http.StatusForbidden, map[string]any{
			"error": "This is a read-only demo instance. Install LabTether to get full access.",
			"demo":  true,
		})
	})
}
