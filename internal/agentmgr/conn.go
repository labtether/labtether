package agentmgr

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// AgentConn wraps a WebSocket connection to an agent with metadata.
type AgentConn struct {
	AssetID       string
	Platform      string
	ConnectedAt   time.Time
	LastMessageAt time.Time

	conn *websocket.Conn
	mu   sync.Mutex
	meta map[string]string

	credentialMu        sync.Mutex
	credentialValidator func() error
	credentialLease     time.Duration
	credentialValidTill time.Time
	rejected            atomic.Bool
}

// NewAgentConn creates an AgentConn wrapping the given WebSocket connection.
func NewAgentConn(conn *websocket.Conn, assetID, platform string) *AgentConn {
	now := time.Now().UTC()
	return &AgentConn{
		AssetID:       assetID,
		Platform:      platform,
		ConnectedAt:   now,
		LastMessageAt: now,
		conn:          conn,
		meta:          make(map[string]string),
	}
}

// AgentWriteDeadline is the write deadline applied to all agent WebSocket writes.
const AgentWriteDeadline = 10 * time.Second

var ErrAgentCredentialRejected = errors.New("agent connection credential rejected")

// Send writes a message to the agent, protected by a mutex with a write deadline.
func (c *AgentConn) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.ValidateCredential(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentCredentialRejected, err)
	}
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(AgentWriteDeadline)); err != nil {
		return err
	}
	return c.conn.WriteJSON(msg)
}

// ReadJSON reads a JSON message from the underlying connection.
func (c *AgentConn) ReadJSON(v interface{}) error {
	return c.conn.ReadJSON(v)
}

// ReadMessage reads one complete frame so callers can account for the exact
// inbound byte cost before unmarshalling attacker-controlled JSON.
func (c *AgentConn) ReadMessage() (int, []byte, error) {
	return c.conn.ReadMessage()
}

// SetReadDeadline sets the read deadline on the underlying connection.
func (c *AgentConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetPongHandler sets the pong handler on the underlying connection.
func (c *AgentConn) SetPongHandler(h func(string) error) {
	c.conn.SetPongHandler(h)
}

// WritePing sends a WebSocket ping control frame.
func (c *AgentConn) WritePing() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.ValidateCredential(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentCredentialRejected, err)
	}
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(AgentWriteDeadline)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

// WriteClose sends a close control frame to the agent.
func (c *AgentConn) WriteClose(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(5*time.Second))
}

// Close closes the underlying WebSocket connection.
func (c *AgentConn) Close() {
	// Reject queued writes before waiting for an in-progress frame to release
	// the socket mutex. One frame already inside WriteJSON may complete; writers
	// queued behind it cannot drain after revocation/unregistration.
	c.rejected.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.Close()
}

// TouchLastMessage updates the last message timestamp.
func (c *AgentConn) TouchLastMessage() {
	c.mu.Lock()
	c.LastMessageAt = time.Now().UTC()
	c.mu.Unlock()
}

// GetLastMessageAt returns the last message timestamp under the lock.
func (c *AgentConn) GetLastMessageAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.LastMessageAt
}

// GetConnectedAt returns the connection timestamp under the lock.
func (c *AgentConn) GetConnectedAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ConnectedAt
}

// SetMeta stores a lightweight runtime metadata key/value on the connection.
func (c *AgentConn) SetMeta(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.meta == nil {
		c.meta = make(map[string]string)
	}
	c.meta[key] = value
}

// Meta reads a connection metadata key.
func (c *AgentConn) Meta(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.meta == nil {
		return ""
	}
	return c.meta[key]
}

// SetCredentialValidator installs a bounded authoritative credential check for
// a per-agent connection. Owner/bootstrap connections intentionally leave it
// unset. The callback must not retain raw bearer material.
func (c *AgentConn) SetCredentialValidator(validator func() error) {
	c.SetCredentialValidatorWithLease(validator, 0)
}

// SetCredentialValidatorWithLease installs an authoritative validator with a
// short positive cache lease. Local revocation still rejects immediately via
// Close; the lease only bounds cross-process database revalidation latency.
func (c *AgentConn) SetCredentialValidatorWithLease(validator func() error, lease time.Duration) {
	c.credentialMu.Lock()
	c.credentialValidator = validator
	c.credentialLease = lease
	c.credentialValidTill = time.Time{}
	c.credentialMu.Unlock()
}

// ValidateCredential revalidates the server-side token ID and bound asset.
// A nil validator preserves compatibility for owner/bootstrap connections and
// in-process tests that do not represent a per-agent bearer session.
func (c *AgentConn) ValidateCredential() error {
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	c.credentialMu.Lock()
	defer c.credentialMu.Unlock()
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	validator := c.credentialValidator
	if validator == nil {
		return nil
	}
	now := time.Now()
	if c.credentialLease > 0 && now.Before(c.credentialValidTill) {
		return nil
	}
	if err := validator(); err != nil {
		c.rejected.Store(true)
		return err
	}
	if c.credentialLease > 0 {
		c.credentialValidTill = now.Add(c.credentialLease)
	}
	return nil
}
