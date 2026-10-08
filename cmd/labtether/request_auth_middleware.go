package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"time"
)

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
