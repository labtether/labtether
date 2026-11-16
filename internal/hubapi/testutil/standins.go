package testutil

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Stand-ins for the Deps fields that cmd/labtether fills from its own
// middleware: route auth, the admin check, the rate limiter, stream tickets,
// the WebSocket upgrader and agent command dispatch. None of them allows
// everything. Each one either refuses what the real one refuses, or fails
// the test when it is reached. Who may call a route is tested in cmd, through
// the real route table (the wiring pins in cmd/labtether/testdata/wiring_pins
// and the route tests there); a package test cannot prove it.

// RouteAuthNotTested returns a WrapAuth/WrapAdmin stand-in. The wrapped
// handler fails the test if a request reaches it: a package test calls the
// handler methods directly, and route auth belongs to cmd's route tests.
func RouteAuthNotTested(t *testing.T) func(http.HandlerFunc) http.HandlerFunc {
	return func(http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			t.Errorf("a request reached a route auth stand-in; route auth is tested in cmd/labtether")
			servicehttp.WriteError(w, http.StatusInternalServerError, "route auth is not wired in package tests")
		}
	}
}

// RequireAdminRole makes the same check as cmd's requireAdminAuth: the
// request's principal must have an admin role, or it gets 403 "forbidden".
// Its wiring to the real middleware is tested in cmd.
func RequireAdminRole(w http.ResponseWriter, r *http.Request) bool {
	if !auth.HasAdminPrivileges(apiv2.UserRoleFromContext(r.Context())) {
		servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// RateLimiter counts requests per bucket and client and refuses with 429
// "rate limit exceeded" once a window's limit is used, as cmd's
// enforceRateLimit does. It does not copy cmd's pruning and eviction; those
// are tested in cmd.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
	Calls   []string // "bucket" for every call, in order
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

// Enforce has the signature of the Deps EnforceRateLimit fields.
func (l *RateLimiter) Enforce(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Calls = append(l.Calls, bucket)
	if limit <= 0 || window <= 0 {
		return true
	}
	if l.windows == nil {
		l.windows = make(map[string]rateWindow)
	}
	key := bucket + ":" + shared.RequestClientKey(r)
	now := time.Now().UTC()
	counter := l.windows[key]
	if counter.resetAt.IsZero() || now.After(counter.resetAt) {
		counter = rateWindow{resetAt: now.Add(window)}
	}
	if counter.count >= limit {
		l.windows[key] = counter
		servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}
	counter.count++
	l.windows[key] = counter
	return true
}

// StreamTickets issues one-use stream tickets with the same input checks as
// cmd's issueStreamTicket: a session ID and a signed-in principal are
// required. Tickets are kept so a test can read what was issued. Consuming
// tickets is cmd middleware and is tested there.
type StreamTickets struct {
	mu     sync.Mutex
	Issued map[string]string // ticket -> session ID
}

// Issue has the signature of the Deps IssueStreamTicket fields.
func (s *StreamTickets) Issue(ctx context.Context, sessionID string) (string, time.Time, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", time.Time{}, errors.New("session id is required")
	}
	actorID := strings.TrimSpace(apiv2.UserIDFromContext(ctx))
	role := auth.NormalizeRole(apiv2.UserRoleFromContext(ctx))
	if actorID == "" || role == "" {
		return "", time.Time{}, errors.New("authenticated principal is required")
	}
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", time.Time{}, err
	}
	ticket := base64.RawURLEncoding.EncodeToString(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Issued == nil {
		s.Issued = make(map[string]string)
	}
	s.Issued[ticket] = sessionID
	return ticket, time.Now().UTC().Add(60 * time.Second), nil
}

// NewStreamUpgrader returns a WebSocket upgrader with the subprotocol of
// cmd's terminalWebSocketUpgrader. Its origin check accepts only requests
// with no Origin header (non-browser clients, which cmd also accepts) and
// refuses every browser origin; cmd's same-origin policy is tested in cmd.
func NewStreamUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return strings.TrimSpace(r.Header.Get("Origin")) == "" },
		Subprotocols: []string{"binary"},
	}
}

// CommandDispatchNotTested returns stand-ins for the agent command
// executors. In cmd these send the command to a connected agent through the
// worker package. Here they fail the test and return a failed result, so a
// test that reaches agent dispatch cannot pass by accident.
func CommandDispatchNotTested(t *testing.T) (func(actions.Job) actions.Result, func(context.Context, actions.Job) actions.Result) {
	fail := func(job actions.Job) actions.Result {
		t.Errorf("job %q reached agent command dispatch; it is not wired in package tests", job.JobID)
		return actions.Result{JobID: job.JobID, RunID: job.RunID, Status: actions.StatusFailed,
			Error: "agent command dispatch is not wired in package tests", CompletedAt: time.Now().UTC()}
	}
	return fail, func(_ context.Context, job actions.Job) actions.Result { return fail(job) }
}
