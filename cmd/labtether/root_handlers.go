package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

func (s *apiServer) handleHubRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"service":      "labtether-hub",
		"message":      "LabTether hub API is running.",
		"console_url":  consoleURLFromRequest(r),
		"healthz_path": "/healthz",
		"readyz_path":  "/readyz",
		"version_path": "/version",
	})
}

// consoleURLFromRequest names the bundled console. Its port 3000 serves plain
// HTTP even when the Hub API uses TLS; set LABTETHER_CONSOLE_URL when a TLS
// front end serves the console instead.
func consoleURLFromRequest(r *http.Request) string {
	override := strings.TrimSpace(envOrDefault("LABTETHER_CONSOLE_URL", ""))
	if override != "" {
		return override
	}

	host := hostWithoutPort(r)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:3000", host)
}

func hostWithoutPort(r *http.Request) string {
	if r == nil {
		return "localhost"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "localhost"
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		if trimmed := strings.TrimSpace(parsedHost); trimmed != "" {
			return strings.Trim(trimmed, "[]")
		}
	}
	host = strings.Trim(host, "[]")
	if idx := strings.LastIndex(host, ":"); idx > -1 && strings.Count(host, ":") == 1 {
		host = host[:idx]
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "localhost"
	}
	return host
}
