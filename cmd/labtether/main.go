package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
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
	"github.com/labtether/labtether/internal/servicehttp"
)

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

// checkSameOrigin validates that the Origin header hostname matches the public host seen by
// the browser. Port is intentionally ignored because the frontend proxy may terminate on one
// port while forwarding websocket upgrades to the backend on another.
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

	originHost := u.Hostname() // strip port
	for _, requestHost := range sameOriginAllowedHosts(r) {
		if sameOriginHostsMatch(originHost, requestHost) {
			return true
		}
	}

	// A loopback Origin (localhost, 127.0.0.1, ::1) means the browser page was
	// genuinely loaded from localhost — browsers always set Origin truthfully
	// and remote attackers cannot forge a loopback Origin header.
	//
	// This covers the dev-mode Next.js proxy where the browser connects to
	// localhost:3000 and the proxy forwards to the backend on a different
	// hostname (e.g. a Tailscale cert hostname or LAN IP). Restricting
	// loopback origins behind DEV_MODE would break this common local
	// development workflow with no security benefit — browsers enforce the
	// Origin header at the protocol level, so spoofing is not possible.
	if isLoopbackHostname(originHost) {
		return true
	}

	return false
}

func sameOriginAllowedHosts(r *http.Request) []string {
	if r == nil {
		return nil
	}
	hosts := []string{requestHostname(r.Host)}
	if isTrustedForwardedHostSource(r) {
		if forwardedHost := requestHostname(firstForwardedValue(r.Header.Get("X-Forwarded-Host"))); forwardedHost != "" {
			hosts = append(hosts, forwardedHost)
		}
		if forwardedHost := requestHostname(forwardedHeaderHost(r.Header.Get("Forwarded"))); forwardedHost != "" {
			hosts = append(hosts, forwardedHost)
		}
	}
	return dedupeNonEmptyHosts(hosts)
}

func requestHostname(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(value); err == nil {
		return h
	}
	return strings.Trim(value, "[]")
}

func sameOriginHostsMatch(originHost, requestHost string) bool {
	if strings.EqualFold(originHost, requestHost) {
		return true
	}
	return isLoopbackHostname(originHost) && isLoopbackHostname(requestHost)
}

func isLoopbackHostname(raw string) bool {
	trimmed := strings.Trim(strings.TrimSpace(raw), "[]")
	switch strings.ToLower(trimmed) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
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

func isTrustedForwardedHostSource(r *http.Request) bool {
	clientHost := requestClientKey(r)
	if clientHost == "" {
		return false
	}
	ip := net.ParseIP(strings.Trim(clientHost, "[]"))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func dedupeNonEmptyHosts(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		trimmed := strings.TrimSpace(host)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
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
		origin := r.Header.Get("Origin")
		if origin != "" && checkSameOrigin(r) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Labtether-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runHub(ctx); err != nil {
		log.Fatalf("labtether exited with error: %v", err)
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
	return func(w http.ResponseWriter, r *http.Request) {
		authzError := func() {
			servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		}
		if ticketedReq, ok := s.consumeStreamTicketAuth(r); ok {
			next(w, ticketedReq)
			return
		}

		// Check cookie session first
		if token := auth.ExtractSessionToken(r); token != "" && s.authStore != nil {
			hashed := auth.HashToken(token)
			session, ok, err := s.authStore.ValidateSession(hashed)
			if err == nil && ok {
				user, userOK, userErr := s.authStore.GetUserByID(session.UserID)
				if userErr != nil || !userOK {
					servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
				role := auth.NormalizeRole(user.Role)
				if !methodAllowedForRole(role, r.Method) {
					authzError()
					return
				}
				ctx := contextWithPrincipal(r.Context(), session.UserID, role)
				next(w, r.WithContext(ctx))
				return
			}
		}

		// Check API key (lt_ prefixed Bearer token)
		if bearer := auth.ExtractBearerToken(r); apikeys.IsAPIKeyFormat(bearer) {
			if s.apiKeyStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "api key authentication unavailable")
				return
			}
			// Reject API keys over plain HTTP.
			isTLS := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
			if !s.tlsState.Enabled && !isTLS {
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
			role := auth.NormalizeRole(key.Role)
			if !methodAllowedForRole(role, r.Method) {
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
				touchCtx := context.WithoutCancel(r.Context())
				go func() {
					updateCtx, cancel := context.WithTimeout(touchCtx, 5*time.Second)
					defer cancel()
					if err := s.apiKeyStore.TouchAPIKeyLastUsed(updateCtx, key.ID); err != nil {
						log.Printf("apikey: touch last-used for %s: %v", key.ID, err) // #nosec G706 -- API key IDs are store-generated identifiers and the error is local runtime state.
					}
				}()
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
		if !methodAllowedForRole(role, r.Method) {
			authzError()
			return
		}
		ctx := contextWithPrincipal(r.Context(), "owner", role)
		next(w, r.WithContext(ctx))
	}
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
		SessionID: sessionID,
		ActorID:   actorID,
		Role:      role,
		ExpiresAt: expiresAt,
	}
	return ticket, expiresAt, nil
}
