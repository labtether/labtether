package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

func httpURLToWS(httpURL string) string { return shared.HTTPURLToWS(httpURL) }

func sanitizeExternalBaseURL(raw string) (string, bool) {
	return shared.SanitizeExternalBaseURL(raw)
}

func sanitizeHostPort(raw string) (string, bool) { return shared.SanitizeHostPort(raw) }

// apiServer methods that reference server state stay here.

// resolveHubURL returns the hub's externally reachable HTTP base URL.
// It prefers the LABTETHER_EXTERNAL_URL env var, then falls back to
// synthesising the URL from the request's Host header.
func (s *apiServer) resolveHubURL(r *http.Request) string {
	return s.resolveHubConnectionSelection(r).HubURL
}

// sanitizedExternalHubURL returns a validated external base URL suitable for
// advertising in enroll/discover responses and install scripts.
// In TLS mode, only https:// external URLs are accepted to prevent
// advertising insecure bootstrap endpoints.
func (s *apiServer) sanitizedExternalHubURL() (string, bool) {
	if s == nil {
		return "", false
	}
	sanitized, ok := sanitizeExternalBaseURL(strings.TrimSpace(s.externalURL))
	if !ok {
		return "", false
	}
	if !s.tlsState.Enabled {
		return sanitized, true
	}
	parsed, err := url.Parse(sanitized)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	return sanitized, true
}
