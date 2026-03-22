package agentmgr

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// AgentManager tracks active WebSocket connections from agents.
type AgentManager struct {
	mu    sync.RWMutex
	conns map[string]*AgentConn
}

// NewManager creates an empty AgentManager.
func NewManager() *AgentManager {
	return &AgentManager{
		conns: make(map[string]*AgentConn),
	}
}

// Register adds an agent connection. If an existing connection for the same
// asset ID exists, it is closed and replaced.
func (m *AgentManager) Register(conn *AgentConn) {
	if m == nil || conn == nil {
		return
	}
	m.mu.Lock()
	if old, ok := m.conns[conn.AssetID]; ok {
		old.Close()
		log.Printf("agentmgr: replaced existing connection for %s", conn.AssetID)
	}
	m.conns[conn.AssetID] = conn
	m.mu.Unlock()
	log.Printf("agentmgr: agent connected: %s (platform=%s)", conn.AssetID, conn.Platform)
}

// Unregister removes the connection for the given asset ID.
func (m *AgentManager) Unregister(assetID string) {
	_ = m.UnregisterIfMatch(assetID, nil)
}

// UnregisterIfMatch removes the connection for the given asset ID only when
// the active connection matches expectedConn (or expectedConn is nil).
// It returns true when a connection was removed.
func (m *AgentManager) UnregisterIfMatch(assetID string, expectedConn *AgentConn) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.conns[assetID]; ok {
		if expectedConn != nil && conn != expectedConn {
			return false
		}
		conn.Close()
		delete(m.conns, assetID)
		log.Printf("agentmgr: agent disconnected: %s", assetID)
		return true
	}
	return false
}

// Get returns the connection for the given asset ID, if any.
func (m *AgentManager) Get(assetID string) (*AgentConn, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.conns[assetID]
	return conn, ok
}

// IsConnected returns true if the given asset has an active WebSocket connection.
func (m *AgentManager) IsConnected(assetID string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.conns[assetID]
	return ok
}

// ConnectedAssets returns the asset IDs of all connected agents.
func (m *AgentManager) ConnectedAssets() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.conns))
	for id := range m.conns {
		ids = append(ids, id)
	}
	return ids
}

// ConnectedAssetInfo holds per-agent capability metadata for the connected agents endpoint.
type ConnectedAssetInfo struct {
	ID       string `json:"id"`
	HasTmux  bool   `json:"has_tmux"`
	Platform string `json:"platform,omitempty"`
}

// ConnectedAssetsInfo returns asset IDs with capability metadata for all connected agents.
func (m *AgentManager) ConnectedAssetsInfo() []ConnectedAssetInfo {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ConnectedAssetInfo, 0, len(m.conns))
	for id, conn := range m.conns {
		hasTmux := strings.TrimSpace(conn.Meta("terminal.tmux.has")) == "true"
		out = append(out, ConnectedAssetInfo{
			ID:       id,
			HasTmux:  hasTmux,
			Platform: conn.Platform,
		})
	}
	return out
}

// Count returns the number of connected agents.
func (m *AgentManager) Count() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.conns)
}

// SendToAgent sends a message to the agent with the given asset ID.
// It satisfies the docker.AgentCommander interface.
func (m *AgentManager) SendToAgent(assetID string, msg Message) error {
	if m == nil {
		return fmt.Errorf("agent manager unavailable")
	}
	conn, ok := m.Get(assetID)
	if !ok {
		return fmt.Errorf("agent %s not connected", assetID)
	}
	return conn.Send(msg)
}
