package main

import (
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
	now := time.Now().UTC()

	s.streamTicketStore.Mu.Lock()
	defer s.streamTicketStore.Mu.Unlock()

	entry, ok := s.streamTicketStore.Tickets[ticket]
	if !ok {
		return false
	}
	if entry.ExpiresAt.Before(now) {
		delete(s.streamTicketStore.Tickets, ticket)
		return false
	}
	if entry.SessionID != "__browser_events__" {
		return false
	}

	delete(s.streamTicketStore.Tickets, ticket)
	return true
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
		ConsumeEventTicket: s.consumeEventTicket,
		IssueStreamTicket:  s.issueStreamTicket,
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
	if s.browserEventsDeps != nil {
		return s.browserEventsDeps
	}
	d := s.buildBrowserEventsDeps()
	s.browserEventsDeps = d
	return d
}
