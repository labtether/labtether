package main

import (
	"context"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	sharedpkg "github.com/labtether/labtether/internal/hubapi/shared"
)

func streamRoleRank(role string) int {
	switch auth.NormalizeRole(role) {
	case auth.RoleOwner:
		return 3
	case auth.RoleAdmin:
		return 2
	case auth.RoleOperator:
		return 1
	default:
		return 0
	}
}

// streamTicketPrincipalActive re-checks, when a stream ticket is redeemed,
// that the sign-in session, user or API key it was issued to still exists and
// still holds at least the ticket's role. A ticket issued just before logout,
// deletion, demotion or key removal must not open a stream afterwards.
func (s *apiServer) streamTicketPrincipalActive(ctx context.Context, actorID, role, apiKeyID, authSessionID string) bool {
	actorID = strings.TrimSpace(actorID)
	switch {
	case actorID == "owner":
		// The bootstrap owner token is not a revocable credential.
		return true
	case apiKeyID != "":
		if s.apiKeyStore == nil {
			return false
		}
		key, found, err := s.apiKeyStore.GetAPIKey(ctx, apiKeyID)
		if err != nil || !found || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())) {
			return false
		}
		return streamRoleRank(key.Role) >= streamRoleRank(role)
	case authSessionID != "":
		if s.authStore == nil {
			return false
		}
		user, found, err := s.authStore.GetUserByID(actorID)
		if err != nil || !found || streamRoleRank(user.Role) < streamRoleRank(role) {
			return false
		}
		sessions, err := s.authStore.ListSessionsByUserID(actorID)
		if err != nil {
			return false
		}
		for _, session := range sessions {
			if session.ID == authSessionID && session.ExpiresAt.After(time.Now()) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// streamTicketRequestActive applies streamTicketPrincipalActive to a request
// authenticated by a redeemed ticket.
func (s *apiServer) streamTicketRequestActive(ctx context.Context) bool {
	return s.streamTicketPrincipalActive(ctx, userIDFromContext(ctx), userRoleFromContext(ctx), apiKeyIDFromContext(ctx), sharedpkg.AuthSessionIDFromContext(ctx))
}

// dropStreamTickets removes unredeemed tickets issued to any of keys, so a
// revoked credential cannot open a stream with a ticket it already holds.
func (s *apiServer) dropStreamTickets(keys ...string) {
	match := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		match[key] = struct{}{}
	}
	s.streamTicketStore.Mu.Lock()
	defer s.streamTicketStore.Mu.Unlock()
	for ticket, entry := range s.streamTicketStore.Tickets {
		ctx := contextWithPrincipal(context.Background(), entry.ActorID, entry.Role)
		if entry.AuthSessionID != "" {
			ctx = sharedpkg.ContextWithAuthSessionID(ctx, entry.AuthSessionID)
		}
		for _, key := range sharedpkg.StreamPrincipalKeys(ctx) {
			if _, ok := match[key]; ok {
				delete(s.streamTicketStore.Tickets, ticket)
				break
			}
		}
	}
}
