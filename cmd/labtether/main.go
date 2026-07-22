package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

// version is set at build time via ldflags: -X main.version=...
var version string

const (
	maxActorIDLength          = 64
	maxTargetLength           = 255
	maxCommandLength          = 4096
	maxModeLength             = 32
	maxConnectorIDLength      = 64
	maxActionIDLength         = 64
	maxActionParamCount       = 24
	maxActionParamKeyLength   = 64
	maxActionParamValLength   = 512
	maxPlanNameLength         = 120
	maxPlanTargetCount        = 100
	maxPlanScopeCount         = 24
	maxCredentialNameLength   = 120
	maxCredentialKindLength   = 32
	maxCredentialSecretLen    = 16384
	maxHostKeyLength          = 2048
	maxAlertRuleNameLength    = 120
	maxAlertDescriptionLen    = 2048
	maxAlertTargetCount       = 200
	maxIncidentTitleLength    = 160
	maxIncidentSummaryLen     = 4096
	maxIncidentLinkIDLength   = 255
	maxAssetTagCount          = 32
	maxAssetTagLength         = 64
	streamTicketTTL           = 60 * time.Second
	maxBrowserEventsReadBytes = 64 * 1024
	maxTerminalInputReadBytes = 256 * 1024
	maxDesktopInputReadBytes  = 256 * 1024
)

var terminalWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin:  checkSameOrigin,
	Subprotocols: []string{"binary"},
}

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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runHub(ctx); err != nil {
		logStartupFailure(err)
		os.Exit(1)
	}
}

// handlePolicyCheck evaluates a policy decision request.
func (s *apiServer) handlePolicyCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req policy.CheckRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid policy request payload")
		return
	}

	cfg := s.policyState.Current()
	res := policy.Evaluate(req, cfg)
	servicehttp.WriteJSON(w, http.StatusOK, res)
}

func (s *apiServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuthMethodPolicy(next, methodAllowedForRole)
}

// withReadCapabilityAuth authenticates every supported principal while
// treating the protected POST as a read capability rather than a fleet
// mutation. Use this only for endpoints whose handler encodes a read-only
// operation as POST, such as minting a one-use subscription ticket.
func (s *apiServer) withReadCapabilityAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuthMethodPolicy(next, func(_, _ string) bool { return true })
}

func (s *apiServer) withAuthMethodPolicy(next http.HandlerFunc, methodPolicy func(role, method string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authzError := func() {
			servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		}
		if ticketedReq, ok := s.consumeStreamTicketAuth(r); ok {
			next(w, ticketedReq)
			return
		}
		if streamTicketFallbackForbidden(r) {
			servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Check cookie session first. An authentication-store failure is a
		// temporary service outage, not evidence that the caller's credential is
		// invalid. Returning 401 here would make well-behaved clients destroy a
		// still-valid session and force an unnecessary login.
		if authenticated, role, ok, err := s.authenticatedSessionRequest(r); err != nil {
			writeSessionAuthenticationUnavailable(w, err)
			return
		} else if ok {
			if methodPolicy == nil || !methodPolicy(role, r.Method) {
				authzError()
				return
			}
			next(w, authenticated)
			return
		}

		// Check API key (lt_ prefixed Bearer token)
		if bearer := auth.ExtractBearerToken(r); apikeys.IsAPIKeyFormat(bearer) {
			if s.apiKeyStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "api key authentication unavailable")
				return
			}
			// Reject API keys over plain HTTP. Forwarded proto is trusted only
			// from loopback reverse proxies; LAN clients can spoof it directly.
			if !s.apiKeyRequestIsSecure(r) {
				servicehttp.WriteError(w, http.StatusForbidden, "api keys require HTTPS")
				return
			}
			hash := apikeys.HashKey(bearer)
			key, found, err := s.apiKeyStore.LookupAPIKeyByHash(r.Context(), hash)
			if err != nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "api key lookup failed")
				return
			}
			if !found {
				servicehttp.WriteError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			// Check expiry.
			if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
				servicehttp.WriteError(w, http.StatusUnauthorized, "api key expired")
				return
			}
			// Persisted keys are untrusted input too. Legacy/corrupt rows with nil
			// scopes must not inherit the nil-context convention used by owner and
			// cookie sessions to mean unrestricted access.
			if err := apikeys.ValidateScopes(key.Scopes); err != nil {
				securityruntime.Logf("api key authentication rejected invalid persisted scopes for key %s: %v", key.ID, err)
				servicehttp.WriteError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			role := auth.NormalizeRole(key.Role)
			if role == auth.RoleOwner {
				securityruntime.Logf("api key authentication rejected reserved owner role for key %s", key.ID)
				servicehttp.WriteError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			if methodPolicy == nil || !methodPolicy(role, r.Method) {
				authzError()
				return
			}
			// Per-key-per-IP rate limiting.
			rateLimitKey := "apikey:" + key.ID
			if !s.enforceRateLimit(w, r, rateLimitKey, 600, time.Minute) {
				return
			}
			// Global per-key rate limit (all IPs combined, higher ceiling).
			globalRateLimitKey := "apikey-global:" + key.ID
			if !s.enforceRateLimitGlobal(w, globalRateLimitKey, 3000, time.Minute) {
				return
			}
			// Debounce last_used_at updates — only touch if >1 minute since last use.
			if key.LastUsedAt == nil || time.Since(*key.LastUsedAt) > time.Minute {
				select {
				case s.apiKeyTouchCh <- key.ID:
				default:
					// Channel full — skip this touch.
				}
			}

			ctx := contextWithPrincipal(r.Context(), "apikey:"+key.ID, role)
			ctx = contextWithScopes(ctx, key.Scopes)
			ctx = contextWithAllowedAssets(ctx, key.AllowedAssets)
			ctx = contextWithAPIKeyID(ctx, key.ID)
			next(w, r.WithContext(ctx))
			return
		}

		// Fall back to bearer token
		if !s.validateOwnerTokenRequest(r) {
			servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		role := auth.RoleOwner
		if methodPolicy == nil || !methodPolicy(role, r.Method) {
			authzError()
			return
		}
		ctx := contextWithPrincipal(r.Context(), "owner", role)
		next(w, r.WithContext(ctx))
	}
}

// withSelfServiceAuth authenticates an interactive user session without
// applying the fleet-wide read-only role gate to mutations that affect only
// that same identity (password, 2FA, and account deletion). API keys and the
// bootstrap owner bearer are intentionally excluded: neither represents a
// persisted user account whose security settings can be changed.
func (s *apiServer) withSelfServiceAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticated, _, ok, err := s.authenticatedSessionRequest(r)
		if err != nil {
			writeSessionAuthenticationUnavailable(w, err)
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, authenticated)
	}
}

func writeSessionAuthenticationUnavailable(w http.ResponseWriter, err error) {
	// Explicitly prevent intermediaries from caching a transient auth outage,
	// and give idempotent clients a bounded retry hint.
	securityruntime.Logf("session authentication temporarily unavailable: %v", err)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "1")
	servicehttp.WriteError(w, http.StatusServiceUnavailable, "session authentication unavailable")
}

func (s *apiServer) authenticatedSessionRequest(r *http.Request) (*http.Request, string, bool, error) {
	if s == nil || r == nil {
		return nil, "", false, nil
	}
	token := auth.ExtractSessionToken(r)
	if token == "" {
		return nil, "", false, nil
	}
	if s.authStore == nil {
		return nil, "", false, errors.New("authentication store is unavailable")
	}
	session, ok, err := s.authStore.ValidateSession(auth.HashToken(token))
	if err != nil {
		return nil, "", false, fmt.Errorf("validate session: %w", err)
	}
	if !ok {
		return nil, "", false, nil
	}
	user, ok, err := s.authStore.GetUserByID(session.UserID)
	if err != nil {
		return nil, "", false, fmt.Errorf("load session user: %w", err)
	}
	if !ok {
		return nil, "", false, nil
	}
	role := auth.NormalizeRole(user.Role)
	ctx := contextWithPrincipal(r.Context(), session.UserID, role)
	return r.WithContext(ctx), role, true, nil
}

func (s *apiServer) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if !auth.HasAdminPrivileges(userRoleFromContext(r.Context())) {
			servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
}

func methodAllowedForRole(role, method string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return true
	}
	return auth.HasWritePrivileges(role)
}

func (s *apiServer) apiKeyRequestIsSecure(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if s != nil && s.tlsState.Enabled {
		return true
	}
	return isLoopbackRequestSource(r) && requestForwardedProtoHTTPS(r)
}

func (s *apiServer) validateOwnerTokenRequest(r *http.Request) bool {
	if s == nil || s.authValidator == nil {
		return false
	}
	return s.authValidator.ValidateRequest(r)
}

func (s *apiServer) consumeStreamTicketAuth(r *http.Request) (*http.Request, bool) {
	if s == nil || r == nil {
		return nil, false
	}
	if r.Method != http.MethodGet {
		return nil, false
	}

	actionSet := map[string]struct{}{"stream": {}}
	path := strings.TrimPrefix(r.URL.Path, "/terminal/sessions/")
	if path == r.URL.Path || path == "" {
		path = strings.TrimPrefix(r.URL.Path, "/desktop/sessions/")
		if path == r.URL.Path || path == "" {
			return nil, false
		}
		actionSet["audio"] = struct{}{}
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return nil, false
	}

	sessionID := strings.TrimSpace(parts[0])
	if sessionID == "" {
		return nil, false
	}
	action := strings.TrimSpace(parts[1])
	if _, ok := actionSet[action]; !ok {
		return nil, false
	}

	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		return nil, false
	}

	now := time.Now().UTC()

	s.streamTicketStore.Mu.Lock()
	defer s.streamTicketStore.Mu.Unlock()

	if len(s.streamTicketStore.Tickets) > 0 {
		for key, entry := range s.streamTicketStore.Tickets {
			if entry.ExpiresAt.Before(now) {
				delete(s.streamTicketStore.Tickets, key)
			}
		}
	}

	entry, ok := s.streamTicketStore.Tickets[ticket]
	if !ok {
		return nil, false
	}
	if entry.ExpiresAt.Before(now) {
		delete(s.streamTicketStore.Tickets, ticket)
		return nil, false
	}
	if entry.SessionID != sessionID {
		return nil, false
	}

	delete(s.streamTicketStore.Tickets, ticket)
	ctx := contextWithPrincipal(r.Context(), entry.ActorID, entry.Role)
	if entry.Scopes != nil {
		ctx = contextWithScopes(ctx, append([]string(nil), entry.Scopes...))
	}
	if entry.AllowedAssets != nil {
		ctx = contextWithAllowedAssets(ctx, append([]string(nil), entry.AllowedAssets...))
	}
	if entry.APIKeyID != "" {
		ctx = contextWithAPIKeyID(ctx, entry.APIKeyID)
	}
	return r.WithContext(ctx), true
}

func (s *apiServer) issueStreamTicket(ctx context.Context, sessionID string) (string, time.Time, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", time.Time{}, errors.New("session id is required")
	}
	actorID := strings.TrimSpace(userIDFromContext(ctx))
	role := auth.NormalizeRole(userRoleFromContext(ctx))
	if actorID == "" || role == "" {
		return "", time.Time{}, errors.New("authenticated principal is required")
	}

	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate stream ticket: %w", err)
	}

	ticket := base64.RawURLEncoding.EncodeToString(payload)
	expiresAt := time.Now().UTC().Add(streamTicketTTL)

	s.streamTicketStore.Mu.Lock()
	defer s.streamTicketStore.Mu.Unlock()

	if s.streamTicketStore.Tickets == nil {
		s.streamTicketStore.Tickets = make(map[string]streamTicket, 128)
	}
	for key, entry := range s.streamTicketStore.Tickets {
		if entry.ExpiresAt.Before(time.Now().UTC()) {
			delete(s.streamTicketStore.Tickets, key)
		}
	}
	s.streamTicketStore.Tickets[ticket] = streamTicket{
		SessionID:     sessionID,
		ActorID:       actorID,
		Role:          role,
		Scopes:        append([]string(nil), scopesFromContext(ctx)...),
		AllowedAssets: append([]string(nil), allowedAssetsFromContext(ctx)...),
		APIKeyID:      apiKeyIDFromContext(ctx),
		ExpiresAt:     expiresAt,
	}
	return ticket, expiresAt, nil
}
