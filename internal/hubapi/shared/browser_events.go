package shared

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

// BrowserClient wraps a WebSocket connection with a per-connection write mutex
// to prevent concurrent writes which corrupt frames and can panic.
type BrowserClient struct {
	conn     *websocket.Conn
	Outgoing chan []byte
	closeMu  sync.Mutex
	closed   bool
}

// EventBroadcaster manages browser WebSocket connections for live event streaming.
type EventBroadcaster struct {
	mu          sync.RWMutex
	clients     map[*BrowserClient]bool
	onBroadcast func() // optional callback invoked on each broadcast (e.g. to bump cache generation)
	onEvent     func(eventType string, data any, at time.Time)
}

// NewEventBroadcaster creates an EventBroadcaster ready for use.
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		clients: make(map[*BrowserClient]bool),
	}
}

// NewBrowserClientForTesting constructs a BrowserClient with the given
// WebSocket connection and outgoing channel. Only use this in tests that need
// white-box access to backpressure or eviction behaviour.
func NewBrowserClientForTesting(conn *websocket.Conn, outgoing chan []byte) *BrowserClient {
	return &BrowserClient{conn: conn, Outgoing: outgoing}
}

// InjectClientForTesting inserts a pre-built BrowserClient directly into the
// broadcaster's client set without starting a write loop. Only use this in
// tests that need to control channel state before broadcasting.
func (eb *EventBroadcaster) InjectClientForTesting(client *BrowserClient) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.clients[client] = true
}

// SetOnBroadcast registers an optional callback that is invoked on every
// Broadcast call (e.g. to bump a cache-generation counter).
func (eb *EventBroadcaster) SetOnBroadcast(fn func()) {
	eb.onBroadcast = fn
}

// SetOnEvent registers an optional callback invoked with the event envelope
// details for each broadcast. Callers must keep handlers non-blocking.
func (eb *EventBroadcaster) SetOnEvent(fn func(eventType string, data any, at time.Time)) {
	eb.onEvent = fn
}

// Register adds a WebSocket connection to the broadcast list.
func (eb *EventBroadcaster) Register(conn *websocket.Conn) *BrowserClient {
	client := &BrowserClient{
		conn:     conn,
		Outgoing: make(chan []byte, 32),
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.clients[client] = true
	go client.writeLoop()
	return client
}

// Unregister removes a browser client from the broadcast list.
func (eb *EventBroadcaster) Unregister(client *BrowserClient) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if _, ok := eb.clients[client]; ok {
		delete(eb.clients, client)
		client.close()
	}
}

// Broadcast sends an event to all connected browser clients.
// Each write is serialized per-connection to prevent concurrent write panics.
func (eb *EventBroadcaster) Broadcast(eventType string, data any) {
	now := time.Now().UTC()
	if eb.onBroadcast != nil {
		eb.onBroadcast()
	}
	if eb.onEvent != nil {
		eb.onEvent(eventType, data, now)
	}
	msg, err := json.Marshal(map[string]any{
		"type": eventType,
		"data": data,
		"ts":   now.Format(time.RFC3339),
	})
	if err != nil {
		log.Printf("browser events: marshal error: %v", err)
		return
	}

	eb.mu.RLock()
	clients := make([]*BrowserClient, 0, len(eb.clients))
	for client := range eb.clients {
		clients = append(clients, client)
	}
	eb.mu.RUnlock()

	for _, client := range clients {
		queued := client.enqueue(msg)
		if !queued {
			eb.Unregister(client)
		}
	}
}

// Count returns the number of connected browser clients.
func (eb *EventBroadcaster) Count() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.clients)
}

func (c *BrowserClient) enqueue(msg []byte) bool {
	if c == nil {
		return false
	}
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.Outgoing <- append([]byte(nil), msg...):
		return true
	default:
		return false
	}
}

func (c *BrowserClient) writeLoop() {
	for msg := range c.Outgoing {
		if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			c.close()
			return
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			c.close()
			return
		}
	}
}

func (c *BrowserClient) close() {
	if c == nil {
		return
	}
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return
	}
	c.closed = true
	close(c.Outgoing)
	c.closeMu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

// BrowserEventsDeps holds the external dependencies for the browser event
// WebSocket endpoint handlers. All function fields are required unless
// documented otherwise.
type BrowserEventsDeps struct {
	// ConsumeEventTicket validates and consumes a one-time ticket issued for
	// the browser events WebSocket (__browser_events__ session). Returns true
	// if the ticket is valid and has not been used.
	ConsumeEventTicket func(ticket string) bool

	// IssueStreamTicket issues a new one-time stream ticket for the given
	// sessionID, returning the ticket string and its expiry time.
	IssueStreamTicket func(ctx context.Context, sessionID string) (string, time.Time, error)

	// ValidateSession checks a hashed session token and reports whether it
	// corresponds to an active authenticated session.
	ValidateSession func(hashedToken string) (bool, error)

	// ValidateOwnerToken reports whether the request carries a valid owner
	// bearer token.
	ValidateOwnerToken func(r *http.Request) bool

	// Broadcaster is the live event broadcaster shared with the rest of the hub.
	Broadcaster *EventBroadcaster

	// CheckOrigin validates the WebSocket upgrade Origin header. When nil, all
	// origins are permitted (not recommended for production).
	CheckOrigin func(r *http.Request) bool

	// MaxReadBytes is the read limit applied to each browser WebSocket
	// connection. Zero means no limit.
	MaxReadBytes int64
}

// HandleEventTicket issues a one-time ticket for WebSocket event streaming.
// POST /ws/events/ticket — returns { "ticket": "...", "expires_at": "..." }
func (d *BrowserEventsDeps) HandleEventTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ticket, expiresAt, err := d.IssueStreamTicket(r.Context(), "__browser_events__")
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue ticket")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_at": expiresAt,
	})
}

// HandleBrowserEvents upgrades an HTTP connection to WebSocket for live event streaming.
func (d *BrowserEventsDeps) HandleBrowserEvents(w http.ResponseWriter, r *http.Request) {
	authenticated := false

	// Check one-time ticket (preferred for browser WS — avoids token in URL).
	if ticket := strings.TrimSpace(r.URL.Query().Get("ticket")); ticket != "" {
		if d.ConsumeEventTicket != nil {
			authenticated = d.ConsumeEventTicket(ticket)
		}
	}

	// Fallback: cookie session auth.
	if !authenticated && d.ValidateSession != nil {
		if token := auth.ExtractSessionToken(r); token != "" {
			hashed := auth.HashToken(token)
			ok, err := d.ValidateSession(hashed)
			if err == nil && ok {
				authenticated = true
			}
		}
	}

	// Fallback: bearer token.
	if !authenticated {
		if d.ValidateOwnerToken == nil || !d.ValidateOwnerToken(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	upgrader := websocket.Upgrader{}
	if d.CheckOrigin != nil {
		upgrader.CheckOrigin = d.CheckOrigin
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("browser events: upgrade failed: %v", err)
		return
	}
	if d.MaxReadBytes > 0 {
		wsConn.SetReadLimit(d.MaxReadBytes)
	}

	if d.Broadcaster == nil {
		_ = wsConn.Close()
		return
	}

	client := d.Broadcaster.Register(wsConn)
	defer func() {
		d.Broadcaster.Unregister(client)
		_ = wsConn.Close()
	}()

	// Read loop — only for keepalive/close detection.
	for {
		_, _, err := wsConn.ReadMessage()
		if err != nil {
			return
		}
	}
}
