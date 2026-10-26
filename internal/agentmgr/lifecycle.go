package agentmgr

import "sync"

// assetLifecycle serializes registration with disconnect side effects for one
// asset. Waiters retain the lock entry, so it can be removed when truly idle.
type assetLifecycle struct {
	mu    sync.Mutex
	users int
}

func (m *AgentManager) lockAssetLifecycle(assetID string) func() {
	m.lifecycleMu.Lock()
	if m.lifecycles == nil {
		m.lifecycles = make(map[string]*assetLifecycle)
	}
	entry := m.lifecycles[assetID]
	if entry == nil {
		entry = &assetLifecycle{}
		m.lifecycles[assetID] = entry
	}
	entry.users++
	m.lifecycleMu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.lifecycleMu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(m.lifecycles, assetID)
		}
		m.lifecycleMu.Unlock()
	}
}

// unregisterInLifecycle requires the asset lifecycle lock, but drops the
// manager mutex before socket close and cleanup. Removing the callback with
// its owner makes ping, revocation, read-loop exit and replacement idempotent.
func (m *AgentManager) unregisterInLifecycle(assetID string, expectedConn *AgentConn) bool {
	m.mu.Lock()
	conn, ok := m.conns[assetID]
	if !ok || (expectedConn != nil && conn != expectedConn) {
		m.mu.Unlock()
		return false
	}
	cleanup := m.disconnectCleanups[assetID]
	delete(m.conns, assetID)
	delete(m.disconnectCleanups, assetID)
	m.mu.Unlock()
	conn.Close()
	if cleanup != nil {
		cleanup(conn)
	}
	return true
}

// DispatchIfCurrent runs connection-owned work in its asset lifecycle.
// Replacement waits for the callback before disconnect cleanup, so an old
// inbound message or browser cannot publish state after a new connection.
// Keep callbacks bounded; perform probes and browser writes outside this gate.
// Handlers may read the manager and close their connection, but must not call
// Register or Unregister for this asset. A closed owner is removed on return.
func (m *AgentManager) DispatchIfCurrent(conn *AgentConn, handle func()) bool {
	if m == nil || conn == nil || handle == nil {
		return false
	}
	release := m.lockAssetLifecycle(conn.AssetID)
	defer release()
	current, ok := m.Get(conn.AssetID)
	if !ok || current != conn {
		return false
	}
	select {
	case <-conn.Done():
		m.unregisterInLifecycle(conn.AssetID, conn)
		return false
	default:
	}
	defer func() {
		select {
		case <-conn.Done():
			m.unregisterInLifecycle(conn.AssetID, conn)
		default:
		}
	}()
	handle()
	return true
}
