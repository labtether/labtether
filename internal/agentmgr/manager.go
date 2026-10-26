package agentmgr

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// AgentManager tracks active WebSocket connections from agents.
type AgentManager struct {
	lifecycleMu        sync.Mutex
	lifecycles         map[string]*assetLifecycle
	mu                 sync.RWMutex
	conns              map[string]*AgentConn
	disconnectCleanups map[string]func(*AgentConn)
	admissions         int
	admissionsByKey    map[string]int
}

// NewManager creates an empty AgentManager.
func NewManager() *AgentManager {
	return &AgentManager{
		conns:              make(map[string]*AgentConn),
		disconnectCleanups: make(map[string]func(*AgentConn)),
		admissionsByKey:    make(map[string]int),
	}
}

type AdmissionRejection int

const (
	AdmissionAllowed AdmissionRejection = iota
	AdmissionGlobalLimit
	AdmissionSourceLimit
)

// TryReserveAdmission atomically reserves capacity for an authenticated or
// in-flight agent WebSocket. The returned release function is idempotent and
// must remain held for the full lifetime of the socket.
func (m *AgentManager) TryReserveAdmission(sourceKey string, globalLimit, sourceLimit int) (func(), AdmissionRejection) {
	if m == nil || globalLimit <= 0 || sourceLimit <= 0 {
		return func() {}, AdmissionGlobalLimit
	}
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		sourceKey = "unknown"
	}
	m.mu.Lock()
	if m.admissions >= globalLimit {
		m.mu.Unlock()
		return func() {}, AdmissionGlobalLimit
	}
	if m.admissionsByKey[sourceKey] >= sourceLimit {
		m.mu.Unlock()
		return func() {}, AdmissionSourceLimit
	}
	m.admissions++
	m.admissionsByKey[sourceKey]++
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			if m.admissions > 0 {
				m.admissions--
			}
			if count := m.admissionsByKey[sourceKey]; count <= 1 {
				delete(m.admissionsByKey, sourceKey)
			} else {
				m.admissionsByKey[sourceKey] = count - 1
			}
			m.mu.Unlock()
		})
	}, AdmissionAllowed
}

// AdmissionCount returns the number of reserved live/in-flight sockets.
func (m *AgentManager) AdmissionCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.admissions
}

// Register adds an agent connection, closing any previous socket for its asset.
func (m *AgentManager) Register(conn *AgentConn) {
	m.RegisterWithLifecycle(conn, nil, nil)
}

// RegisterWithLifecycle publishes a connection and retains its cleanup until
// any unregister or replacement. Both callbacks run in the asset lifecycle
// transition. They may use manager reads, but must not recursively register or
// unregister this same asset. No manager-wide lock is held during callbacks.
func (m *AgentManager) RegisterWithLifecycle(conn *AgentConn, cleanup func(*AgentConn), publish func()) {
	if m == nil || conn == nil {
		return
	}
	release := m.lockAssetLifecycle(conn.AssetID)
	defer release()
	if current, ok := m.Get(conn.AssetID); ok && current == conn {
		return
	}
	if m.unregisterInLifecycle(conn.AssetID, nil) {
		log.Printf("agentmgr: replaced existing connection for %s", conn.AssetID)
	}
	m.mu.Lock()
	m.conns[conn.AssetID] = conn
	m.disconnectCleanups[conn.AssetID] = cleanup
	m.mu.Unlock()
	if publish != nil {
		publish()
	}
	log.Printf("agentmgr: agent connected: %s (platform=%s)", conn.AssetID, conn.Platform)
}

// Unregister removes the connection for the given asset ID.
func (m *AgentManager) Unregister(assetID string) {
	_ = m.UnregisterIfMatch(assetID, nil)
}

// UnregisterIfMatch removes the connection for the given asset ID only when
// the active connection matches expectedConn (or expectedConn is nil). Its
// registered cleanup completes before a replacement can be published.
// It returns true when a connection was removed.
func (m *AgentManager) UnregisterIfMatch(assetID string, expectedConn *AgentConn) bool {
	if m == nil {
		return false
	}
	release := m.lockAssetLifecycle(assetID)
	defer release()
	return m.unregisterInLifecycle(assetID, expectedConn)
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
