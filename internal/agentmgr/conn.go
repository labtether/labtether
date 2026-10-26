package agentmgr

import (
	"context"
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
	// writeMu serializes socket writes only. Metadata uses stateMu so a
	// stalled write never blocks the read loop or manager inventory reads.
	writeMu sync.Mutex
	stateMu sync.Mutex
	meta    map[string]string

	credentialMu        sync.Mutex
	credentialValidator func() error
	credentialLease     time.Duration
	credentialValidTill time.Time
	rejected            atomic.Bool
	doneOnce            sync.Once
	closeOnce           sync.Once
	done                chan struct{}
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
	return c.SendContext(context.Background(), msg)
}

// SendContext rejects canceled requests at each pre-write ownership gate.
// It cannot interrupt a current mutex/credential holder. Once a frame begins,
// the existing transport timeout owns it; cancellation does not close or alter
// the deadline of this shared socket, and delivery may remain unknown.
//
// The credential check (which may consult the database) runs before the write
// mutex is taken, so a slow check never holds up other writers, and the write
// mutex is never held while waiting on anything but the socket itself.
func (c *AgentConn) SendContext(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.checkCredentialForWrite(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(AgentWriteDeadline)); err != nil {
		return err
	}
	return c.conn.WriteJSON(msg)
}

func (c *AgentConn) checkCredentialForWrite() error {
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.ValidateCredential(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentCredentialRejected, err)
	}
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	return nil
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
	if err := c.checkCredentialForWrite(); err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.rejected.Load() {
		return ErrAgentCredentialRejected
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(AgentWriteDeadline)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

// WriteClose sends a close control frame to the agent. Gorilla allows
// WriteControl concurrently with other writers, so it does not wait behind a
// stalled data frame.
func (c *AgentConn) WriteClose(msg []byte) error {
	return c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(5*time.Second))
}

// Done closes as soon as this connection is retired. It is safe on a zero-value
// AgentConn so in-process owners and tests share the same lifecycle contract.
func (c *AgentConn) Done() <-chan struct{} {
	c.doneOnce.Do(func() { c.done = make(chan struct{}) })
	return c.done
}

// Close retires this connection and interrupts its underlying socket. Gorilla
// permits Close concurrently with every other method; taking the write mutex
// here would delay disconnect waiters behind a stalled write or credential read.
func (c *AgentConn) Close() {
	c.rejected.Store(true)
	c.closeOnce.Do(func() {
		c.Done()
		close(c.done)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

// TouchLastMessage updates the last message timestamp.
func (c *AgentConn) TouchLastMessage() {
	c.stateMu.Lock()
	c.LastMessageAt = time.Now().UTC()
	c.stateMu.Unlock()
}

// GetLastMessageAt returns the last message timestamp under the lock.
func (c *AgentConn) GetLastMessageAt() time.Time {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.LastMessageAt
}

// GetConnectedAt returns the connection timestamp under the lock.
func (c *AgentConn) GetConnectedAt() time.Time {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.ConnectedAt
}

// SetMeta stores a lightweight runtime metadata key/value on the connection.
func (c *AgentConn) SetMeta(key, value string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.meta == nil {
		c.meta = make(map[string]string)
	}
	c.meta[key] = value
}

// Meta reads a connection metadata key.
func (c *AgentConn) Meta(key string) string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.meta == nil {
		return ""
	}
	return c.meta[key]
}

// SetCredentialValidator installs an authoritative credential check for a
// connection. The callback must not retain raw bearer material.
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

// ValidateCredential revalidates the server-side credential and bound asset.
// A nil validator preserves compatibility for in-process tests that do not
// represent an authenticated agent session.
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
