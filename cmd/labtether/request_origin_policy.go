package main

import (
	"context"
	"github.com/labtether/labtether/internal/servicehttp"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// checkSameOrigin validates that the Origin header exactly matches the public
// origin seen by the browser. Host-wide cookies are shared across ports, so
// treating another service on the same hostname as same-origin would let that
// service make credentialed API calls. Trusted proxies must forward both the
// original host and scheme; intentional cross-origin callers must be listed in
// LABTETHER_CORS_ALLOWED_ORIGINS.
//
// Non-browser clients (agents, curl) that don't send Origin are allowed through.
func checkSameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true // non-browser clients don't send Origin
	}

	// Native iOS WKWebView pages may present an opaque Origin ("null"), file://,
	// or the app's custom desktop bundle scheme (labtether-local://). Allow only
	// for stream endpoints already protected by one-time stream tickets.
	if strings.EqualFold(origin, "null") {
		return isNativeAppStreamPath(r.URL.Path)
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if isNativeAppOriginScheme(u.Scheme) {
		return isNativeAppStreamPath(r.URL.Path)
	}
	if !isNetworkOriginScheme(u.Scheme) {
		return false
	}
	originValue, ok := normalizedNetworkOrigin(u)
	if !ok {
		return false
	}

	for _, requestOrigin := range sameOriginAllowedOrigins(r) {
		if networkOriginsMatch(originValue, requestOrigin) {
			return true
		}
	}
	for _, allowed := range configuredCORSAllowedOrigins() {
		if networkOriginsMatch(originValue, allowed) {
			return true
		}
	}

	return false
}

type networkOrigin struct {
	scheme string
	host   string
	port   string
}

func normalizedNetworkOrigin(u *url.URL) (networkOrigin, bool) {
	if u == nil || !isNetworkOriginScheme(u.Scheme) || u.User != nil || u.Hostname() == "" {
		return networkOrigin{}, false
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return networkOrigin{}, false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	port := strings.TrimSpace(u.Port())
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return networkOrigin{}, false
		}
	}
	return networkOrigin{
		scheme: scheme,
		host:   strings.ToLower(strings.Trim(strings.TrimSpace(u.Hostname()), "[]")),
		port:   port,
	}, true
}

func parseNetworkOrigin(raw string) (networkOrigin, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return networkOrigin{}, false
	}
	return normalizedNetworkOrigin(parsed)
}

func networkOriginsMatch(left, right networkOrigin) bool {
	return left.scheme == right.scheme && left.port == right.port && left.host == right.host
}

func sameOriginAllowedOrigins(r *http.Request) []networkOrigin {
	if r == nil {
		return nil
	}
	out := make([]networkOrigin, 0, 3)
	directScheme := "http"
	if r.TLS != nil {
		directScheme = "https"
	}
	if candidate, ok := parseNetworkOrigin(directScheme + "://" + strings.TrimSpace(r.Host)); ok {
		out = append(out, candidate)
	}

	if !isTrustedForwardedHostSource(r) {
		return out
	}
	forwardedScheme := strings.ToLower(strings.TrimSpace(firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))))
	if forwardedScheme == "" {
		forwardedScheme = strings.ToLower(strings.TrimSpace(forwardedHeaderProto(r.Header.Get("Forwarded"))))
	}
	if !isNetworkOriginScheme(forwardedScheme) {
		return out
	}
	for _, forwardedHost := range []string{
		firstForwardedValue(r.Header.Get("X-Forwarded-Host")),
		forwardedHeaderHost(r.Header.Get("Forwarded")),
	} {
		if candidate, ok := parseNetworkOrigin(forwardedScheme + "://" + strings.TrimSpace(forwardedHost)); ok {
			out = append(out, candidate)
		}
	}
	return out
}

func configuredCORSAllowedOrigins() []networkOrigin {
	var out []networkOrigin
	for _, raw := range strings.Split(os.Getenv("LABTETHER_CORS_ALLOWED_ORIGINS"), ",") {
		if candidate, ok := parseNetworkOrigin(raw); ok {
			out = append(out, candidate)
		}
	}
	return out
}

func firstForwardedValue(raw string) string {
	if raw == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(raw, ",")[0])
}

func forwardedHeaderHost(raw string) string {
	for _, entry := range strings.Split(raw, ",") {
		for _, part := range strings.Split(entry, ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "host") {
				continue
			}
			return strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return ""
}

func forwardedHeaderProto(raw string) string {
	for _, entry := range strings.Split(raw, ",") {
		for _, part := range strings.Split(entry, ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "proto") {
				continue
			}
			return strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return ""
}

func isTrustedForwardedHostSource(r *http.Request) bool {
	clientHost := requestClientKey(r)
	if clientHost == "" {
		return false
	}
	ip := net.ParseIP(strings.Trim(clientHost, "[]"))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, rawCIDR := range strings.Split(os.Getenv("LABTETHER_TRUST_PROXY_CIDRS"), ",") {
		_, network, err := net.ParseCIDR(strings.TrimSpace(rawCIDR))
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	for _, rawHost := range strings.Split(os.Getenv("LABTETHER_TRUST_PROXY_HOSTS"), ",") {
		host := strings.TrimSpace(rawHost)
		if host == "" {
			continue
		}
		lookupCtx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		resolved, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
		cancel()
		if err != nil {
			continue
		}
		for _, candidate := range resolved {
			if candidate.IP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

func isLoopbackRequestSource(r *http.Request) bool {
	clientHost := requestClientKey(r)
	if clientHost == "" {
		return false
	}
	ip := net.ParseIP(strings.Trim(clientHost, "[]"))
	return ip != nil && ip.IsLoopback()
}

func requestForwardedProtoHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))), "https") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(forwardedHeaderProto(r.Header.Get("Forwarded"))), "https")
}

func externalURLIsHTTPS(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

func isNativeAppStreamPath(path string) bool {
	return isSessionStreamPath(path, "/terminal/sessions/", "stream") ||
		isSessionStreamPath(path, "/desktop/sessions/", "stream", "audio")
}

func isSessionStreamPath(path, prefix string, allowedActions ...string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	remainder := strings.TrimPrefix(path, prefix)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return false
	}
	action := strings.TrimSpace(parts[1])
	for _, allowed := range allowedActions {
		if action == allowed {
			return true
		}
	}
	return false
}

func isNativeAppOriginScheme(rawScheme string) bool {
	switch strings.ToLower(strings.TrimSpace(rawScheme)) {
	case "file", "labtether-local":
		return true
	default:
		return false
	}
}

// streamTicketFallbackForbidden reports whether a stream request must not fall
// back to cookie, API-key, or owner-token authentication after ticket
// validation fails. Opaque/native browser origins are admitted by the
// WebSocket origin check only because a one-time ticket is mandatory. A
// supplied ticket is also authoritative: accepting another credential after
// an invalid or replayed ticket would defeat its one-time semantics.
func streamTicketFallbackForbidden(r *http.Request) bool {
	if r == nil || r.Method != http.MethodGet || !isNativeAppStreamPath(r.URL.Path) {
		return false
	}
	if r.URL.Query().Has("ticket") {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if strings.EqualFold(origin, "null") {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && isNativeAppOriginScheme(parsed.Scheme)
}

func isNetworkOriginScheme(rawScheme string) bool {
	switch strings.ToLower(strings.TrimSpace(rawScheme)) {
	case "http", "https":
		return true
	default:
		return false
	}
}

// corsMiddleware adds explicit CORS response headers for requests whose
// Origin passes the same-origin check (checkSameOrigin). Preflight OPTIONS
// requests receive a 204 No Content immediately — before any downstream auth
// middleware — so browsers can negotiate cross-origin access without
// credentials.
//
// Vary: Origin is always set when an origin is echoed to ensure intermediate
// caches do not serve a cached Access-Control-Allow-Origin for one origin to
// a different origin.
func (s *apiServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Browser mutations need an origin check at the hub as well as at the
		// console proxy. The API listener is reachable directly in common homelab
		// deployments. Applying this before authentication also protects initial
		// bootstrap and login endpoints from cross-origin state changes.
		if isMutatingHTTPMethod(r.Method) && !browserMutationOriginAllowed(r) {
			servicehttp.WriteError(w, http.StatusForbidden, "forbidden origin")
			return
		}

		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && checkSameOrigin(r)
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Labtether-Token, X-Labtether-Setup-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.Header().Add("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func isMutatingHTTPMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func browserMutationOriginAllowed(r *http.Request) bool {
	if r == nil || !isMutatingHTTPMethod(r.Method) {
		return true
	}
	if strings.TrimSpace(r.Header.Get("Origin")) != "" {
		return checkSameOrigin(r)
	}

	// Non-browser clients commonly omit both Origin and Fetch Metadata. Modern
	// browsers identify cross-origin requests through Sec-Fetch-Site even when
	// a privacy feature strips Origin, so fail closed for cross-site/same-site
	// browser mutations while preserving CLI compatibility.
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "", "none", "same-origin":
		return true
	default:
		return false
	}
}
