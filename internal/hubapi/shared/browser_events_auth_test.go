package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/auth"
)

func browserEventsWebSocketURL(httpURL, path string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + path
}

func TestHandleBrowserEventsRejectsInvalidSuppliedTicketWithoutAuthFallback(t *testing.T) {
	t.Parallel()

	consumeCalled := false
	sessionCalled := false
	ownerTokenCalled := false
	deps := &BrowserEventsDeps{
		ConsumeEventTicket: func(ticket string) bool {
			consumeCalled = true
			return ticket == "valid-ticket"
		},
		ValidateSession: func(string) (bool, error) {
			sessionCalled = true
			return true, nil
		},
		ValidateOwnerToken: func(*http.Request) bool {
			ownerTokenCalled = true
			return true
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/events?ticket=replayed-ticket", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "otherwise-valid-session"})
	rec := httptest.NewRecorder()

	deps.HandleBrowserEvents(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !consumeCalled {
		t.Fatal("supplied ticket was not checked")
	}
	if sessionCalled || ownerTokenCalled {
		t.Fatalf("invalid supplied ticket fell through to another credential: session=%t owner_token=%t", sessionCalled, ownerTokenCalled)
	}
}

func TestHandleBrowserEventsRejectsEmptySuppliedTicketWithoutAuthFallback(t *testing.T) {
	t.Parallel()

	sessionCalled := false
	deps := &BrowserEventsDeps{
		ConsumeEventTicket: func(string) bool {
			t.Fatal("empty ticket must not be consumed")
			return false
		},
		ValidateSession: func(string) (bool, error) {
			sessionCalled = true
			return true, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/events?ticket=", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "otherwise-valid-session"})
	rec := httptest.NewRecorder()

	deps.HandleBrowserEvents(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if sessionCalled {
		t.Fatal("empty supplied ticket fell through to the session cookie")
	}
}

func TestHandleBrowserEventsConsumesTicketOnceWithoutSessionFallback(t *testing.T) {
	var mu sync.Mutex
	used := false
	sessionCalled := false
	deps := &BrowserEventsDeps{
		ConsumeEventTicket: func(ticket string) bool {
			mu.Lock()
			defer mu.Unlock()
			if ticket != "one-use-ticket" || used {
				return false
			}
			used = true
			return true
		},
		ValidateSession: func(string) (bool, error) {
			sessionCalled = true
			return true, nil
		},
		CheckOrigin: func(*http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(deps.HandleBrowserEvents))
	t.Cleanup(server.Close)

	header := http.Header{}
	header.Set("Cookie", (&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: "otherwise-valid-session",
	}).String())
	url := browserEventsWebSocketURL(server.URL, "/ws/events?ticket=one-use-ticket")

	conn, response, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("first ticket use failed: %v (response=%v)", err, response)
	}
	_ = conn.Close()

	conn, response, err = websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("replayed ticket unexpectedly upgraded")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed ticket response = %v, want HTTP %d", response, http.StatusUnauthorized)
	}
	if sessionCalled {
		t.Fatal("ticket use or replay fell through to the otherwise-valid session cookie")
	}
}

func TestHandleBrowserEventsAllowsCookieFallbackWhenNoTicketWasSupplied(t *testing.T) {
	sessionCalled := false
	ownerTokenCalled := false
	deps := &BrowserEventsDeps{
		ValidateSession: func(string) (bool, error) {
			sessionCalled = true
			return true, nil
		},
		ValidateOwnerToken: func(*http.Request) bool {
			ownerTokenCalled = true
			return false
		},
		CheckOrigin: func(*http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(deps.HandleBrowserEvents))
	t.Cleanup(server.Close)

	header := http.Header{}
	header.Set("Cookie", (&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: "valid-session",
	}).String())
	conn, response, err := websocket.DefaultDialer.Dial(
		browserEventsWebSocketURL(server.URL, "/ws/events"),
		header,
	)
	if err != nil {
		t.Fatalf("cookie-authenticated connection failed: %v (response=%v)", err, response)
	}
	_ = conn.Close()

	if !sessionCalled {
		t.Fatal("request without a ticket did not use the session cookie fallback")
	}
	if ownerTokenCalled {
		t.Fatal("successful session-cookie authentication fell through to owner-token validation")
	}
}
