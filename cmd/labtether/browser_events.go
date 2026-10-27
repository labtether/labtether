package main

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// EventBroadcaster and related browser WebSocket types now live in
// internal/hubapi/shared. The aliases below keep all call sites in this
// package compiling without change.

type EventBroadcaster = shared.EventBroadcaster

func newEventBroadcaster() *EventBroadcaster {
	return shared.NewEventBroadcaster()
}

// consumeEventTicket validates and consumes a one-time ticket for browser events.
func (s *apiServer) consumeEventTicket(ticket string) bool {
	_, ok := s.consumeEventTicketPrincipals(ticket)
	return ok
}

// consumeEventTicketPrincipals consumes a browser events ticket and returns the
// credentials it was issued to, so the socket can be closed when they end.
func (s *apiServer) consumeEventTicketPrincipals(ticket string) ([]string, bool) {
	now := time.Now().UTC()

	s.streamTicketStore.Mu.Lock()
	defer s.streamTicketStore.Mu.Unlock()

	entry, ok := s.streamTicketStore.Tickets[ticket]
	if !ok {
		return nil, false
	}
	if entry.ExpiresAt.Before(now) {
		delete(s.streamTicketStore.Tickets, ticket)
		return nil, false
	}
	if entry.SessionID != "__browser_events__" {
		return nil, false
	}
	if entry.APIKeyID != "" && len(entry.AllowedAssets) > 0 {
		delete(s.streamTicketStore.Tickets, ticket)
		return nil, false
	}

	delete(s.streamTicketStore.Tickets, ticket)
	ctx := contextWithPrincipal(context.Background(), entry.ActorID, entry.Role)
	if entry.AuthSessionID != "" {
		ctx = shared.ContextWithAuthSessionID(ctx, entry.AuthSessionID)
	}
	return shared.StreamPrincipalKeys(ctx), true
}

// handleEventTicket issues a one-time ticket for WebSocket event streaming.
// POST /ws/events/ticket — returns { "ticket": "...", "expires_at": "..." }
func (s *apiServer) handleEventTicket(w http.ResponseWriter, r *http.Request) {
	s.ensureBrowserEventsDeps().HandleEventTicket(w, r)
}

// handleBrowserEvents upgrades an HTTP connection to WebSocket for live event streaming.
func (s *apiServer) handleBrowserEvents(w http.ResponseWriter, r *http.Request) {
	s.ensureBrowserEventsDeps().HandleBrowserEvents(w, r)
}

// buildBrowserEventsDeps constructs the BrowserEventsDeps from apiServer fields.
func (s *apiServer) buildBrowserEventsDeps() *shared.BrowserEventsDeps {
	return &shared.BrowserEventsDeps{
		ConsumeEventTicket:           s.consumeEventTicket,
		ConsumeEventTicketPrincipals: s.consumeEventTicketPrincipals,
		SessionPrincipals: func(hashedToken string) ([]string, bool, error) {
			if s.authStore == nil {
				return nil, false, nil
			}
			session, ok, err := s.authStore.ValidateSession(hashedToken)
			if err != nil || !ok {
				return nil, ok, err
			}
			return []string{shared.StreamActorKey(session.UserID), shared.StreamSessionKey(session.ID)}, true, nil
		},
		IssueStreamTicket: s.issueStreamTicket,
		ValidateSession: func(hashedToken string) (bool, error) {
			if s.authStore == nil {
				return false, nil
			}
			_, ok, err := s.authStore.ValidateSession(hashedToken)
			return ok, err
		},
		ValidateOwnerToken: s.validateOwnerTokenRequest,
		Broadcaster:        s.broadcaster,
		CheckOrigin:        checkSameOrigin,
		MaxReadBytes:       maxBrowserEventsReadBytes,
	}
}

// ensureBrowserEventsDeps returns the browser events deps, creating and
// caching on first call.
func (s *apiServer) ensureBrowserEventsDeps() *shared.BrowserEventsDeps {
	s.browserEventsDepsOnce.Do(func() {
		s.browserEventsDeps = s.buildBrowserEventsDeps()
	})
	return s.browserEventsDeps
}

// revokeStreamPrincipals closes terminal, remote view, container shell and
// event streams opened with any of the given credentials.
func (s *apiServer) revokeStreamPrincipals(keys ...string) {
	if s == nil || len(keys) == 0 {
		return
	}
	s.sessionStreams.RevokePrincipals(keys...)
	if s.broadcaster != nil {
		s.broadcaster.DisconnectPrincipals(keys...)
	}
}
