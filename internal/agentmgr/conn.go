package agentmgr

import (
	"sync"
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

// Send writes a message to the agent, protected by a mutex with a write deadline.
func (c *AgentConn) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(AgentWriteDeadline)); err != nil {
		return err
	}
	return c.conn.WriteJSON(msg)
}

// ReadJSON reads a JSON message from the underlying connection.
func (c *AgentConn) ReadJSON(v interface{}) error {
	return c.conn.ReadJSON(v)
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
