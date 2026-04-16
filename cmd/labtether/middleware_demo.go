package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// demoReadOnlyMiddleware blocks all mutating HTTP requests in demo mode.
func demoReadOnlyMiddleware(next http.Handler) http.Handler {
	allowedPOSTPaths := map[string]bool{
		"/api/demo/session":   true,
		"/api/auth/login":     true,
		"/api/auth/login/2fa": true,
		"/auth/login":         true,
		"/auth/login/2fa":     true,
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

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "This is a read-only demo instance. Install LabTether to get full access.",
			"demo":  true,
		})
	})
}
