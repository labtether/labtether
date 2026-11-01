package shared

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

type authSessionContextKey struct{}

// ContextWithAuthSessionID records the sign-in session behind a request, so a
// stream opened with it can be closed when that session ends.
func ContextWithAuthSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, authSessionContextKey{}, strings.TrimSpace(sessionID))
}

// AuthSessionIDFromContext returns the sign-in session ID, or "".
func AuthSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(authSessionContextKey{}).(string)
	return id
}

// StreamActorKey names every stream opened by one user or API key
// ("apikey:<id>").
func StreamActorKey(actorID string) string { return "actor:" + strings.TrimSpace(actorID) }

// StreamSessionKey names every stream opened through one sign-in session.
func StreamSessionKey(sessionID string) string { return "session:" + strings.TrimSpace(sessionID) }

// StreamPrincipalKeys names the credentials behind a stream request: its actor
// and, for browser sign-ins, its session. The bootstrap owner token is not a
// revocable credential and yields no actor key.
func StreamPrincipalKeys(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	keys := make([]string, 0, 2)
	if actor := strings.TrimSpace(apiv2.UserIDFromContext(ctx)); actor != "" && actor != "owner" {
		keys = append(keys, StreamActorKey(actor))
	}
	if sessionID := AuthSessionIDFromContext(ctx); sessionID != "" {
		keys = append(keys, StreamSessionKey(sessionID))
	}
	return keys
}

func (r *SessionStreamRegistry) indexPrincipals(owner *sessionStreamRequest) {
	if len(owner.principals) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.principals == nil {
		r.principals = make(map[string]map[*sessionStreamRequest]struct{})
	}
	for _, key := range owner.principals {
		if r.principals[key] == nil {
			r.principals[key] = make(map[*sessionStreamRequest]struct{})
		}
		r.principals[key][owner] = struct{}{}
	}
}

func (r *SessionStreamRegistry) unindexPrincipals(owner *sessionStreamRequest) {
	if len(owner.principals) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range owner.principals {
		delete(r.principals[key], owner)
		if len(r.principals[key]) == 0 {
			delete(r.principals, key)
		}
	}
}

// RevokePrincipals closes every open or opening stream whose credentials match
// one of keys. Callers use it after logout, user deletion or demotion, session
// revocation and API key removal, so access ends with the credential.
func (r *SessionStreamRegistry) RevokePrincipals(keys ...string) int {
	r.mu.Lock()
	var owners []*sessionStreamRequest
	for _, key := range keys {
		for owner := range r.principals[key] {
			owners = append(owners, owner)
		}
	}
	r.mu.Unlock()
	closed := 0
	for _, owner := range owners {
		owner.entry.mu.Lock()
		wasOpen := !owner.closed
		owner.closed = true
		conn := owner.conn
		owner.entry.mu.Unlock()
		owner.cancel()
		if conn != nil {
			_ = conn.Close()
		}
		if wasOpen {
			closed++
		}
	}
	return closed
}
